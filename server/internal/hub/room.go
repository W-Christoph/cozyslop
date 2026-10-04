package hub

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"math"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"cozycast/internal/auth"
	"cozycast/internal/neko"
	"cozycast/internal/ratelimit"
	"cozycast/internal/rights"
	"cozycast/internal/store"
)

var (
	ErrNotPresent  = errors.New("hub: user is not in the room")
	ErrNotAllowed  = errors.New("hub: not allowed")
	ErrRateLimited = errors.New("hub: sending too fast")
)

// DeniedError is returned by Join when the identity may not enter.
type DeniedError struct{ Denial rights.Denial }

func (e *DeniedError) Error() string { return "hub: join denied: " + e.Denial.Reason }

type Room struct {
	Name     string
	NekoPath string // public path the browser uses to reach this room's neko

	hub           *Hub
	neko          *neko.Client
	log           *slog.Logger
	ready         chan struct{} // closed once neko is up and cleaned
	defaultScreen string

	inbound   *ratelimit.Limiter // per person: every message type
	chatUser  *ratelimit.Limiter // per person: new chat messages
	chatAnon  *ratelimit.Limiter
	whisperID atomic.Int64
	restart   func(ctx context.Context) error // nil = container control disabled

	mu       sync.Mutex
	settings store.RoomSettings
	members  map[string]*member // by identity key
	clients  map[string]*Client // by client id
	tokens   map[string]*Client // by the neko session token issued to the tab
	hostID   string             // neko session id (= client id) holding the remote

	lastRestart time.Time // for the trusted-user cooldown
	streams     []string  // capture pipelines neko offers, the default first

	titleURL string // set before run
	title    string // of the window in front on the desktop
}

// member is one person in the room, with all their tabs.
type member struct {
	key      string
	user     *store.User // nil for anonymous
	anonID   string
	perm     store.Permission
	grant    *rights.Grant // from a temporary access invite
	rights   rights.Rights
	joinedAt int64 // unix ms
	lastSeen int64 // unix ms; when the member was last active
	clients  map[string]*Client
}

// Client is one browser tab.
type Client struct {
	ID   string
	send func(any) // must not block
	kill func()    // closes the connection; Leave follows

	sessionHash []byte
	ip          string
	joinedAt    time.Time

	// guarded by Room.mu
	m      *member
	active bool
	muted  bool
	typing time.Time // last typing broadcast, for throttling

	nekoToken string                 // the tab's current neko session token
	nekoConns map[*NekoConn]struct{} // its proxied connections to neko

	nekoMu       sync.Mutex // serializes neko member creation, updates and deletion
	nekoPassword string
	gone         bool // left or kicked: no more neko members for this tab
}

func newRoom(h *Hub, name string, nc *neko.Client) *Room {
	return &Room{
		Name:     name,
		NekoPath: "/neko/" + name,
		hub:      h,
		neko:     nc,
		log:      slog.With("room", name),
		ready:    make(chan struct{}),
		inbound:  ratelimit.New(40, 200*time.Millisecond),
		chatUser: ratelimit.New(10, 500*time.Millisecond),
		chatAnon: ratelimit.New(5, time.Second),
		settings: store.RoomSettings{Name: name, Access: "public"},
		members:  make(map[string]*member),
		clients:  make(map[string]*Client),
		tokens:   make(map[string]*Client),
	}
}

func (r *Room) Neko() *neko.Client { return r.neko }

// UserCount is the number of people (not tabs) in the room.
func (r *Room) UserCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.members)
}

// run prepares neko and follows the remote until ctx ends.
func (r *Room) run(ctx context.Context) {
	for {
		if r.neko.Healthy(ctx) {
			err := r.prepareNeko(ctx)
			if err == nil {
				break
			}
			r.log.Warn("prepare neko", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
	close(r.ready)
	r.log.Info("neko ready")
	go r.watchMembers(ctx)
	if r.titleURL != "" {
		go r.watchTitle(ctx)
	}
	r.neko.WatchHost(ctx, func(init neko.Init) {
		r.mu.Lock()
		stream := r.streamLocked()
		r.streams = init.Videos
		// Tabs that joined before neko reported its streams may have been
		// told about a stream it does not offer.
		r.broadcastLocked(settingsMsg{Type: "room_settings", Settings: r.publicSettingsLocked()}, nil)
		r.pinStreamLocked(stream)
		r.mu.Unlock()
		r.reapplyNekoSettings(ctx)
	}, r.setHost)
}

// titleInterval is how often the desktop is asked which window is in front.
const (
	titleInterval = 2 * time.Second
	maxTitleRunes = 200
)

var titleClient = &http.Client{Timeout: titleInterval}

// watchTitle keeps the room's viewers told which window is in front on the
// desktop; their browser tab is named after it.
func (r *Room) watchTitle(ctx context.Context) {
	t := time.NewTicker(titleInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.checkTitle(ctx)
		}
	}
}

// checkTitle asks the desktop for its window title, if anyone is there to
// see it, and broadcasts a change. A desktop that does not answer (it is
// restarting, or has no helper) has no title.
func (r *Room) checkTitle(ctx context.Context) {
	r.mu.Lock()
	watched := len(r.clients) > 0
	r.mu.Unlock()
	if !watched {
		return
	}
	title := fetchTitle(ctx, r.titleURL)
	r.mu.Lock()
	defer r.mu.Unlock()
	if title != r.title {
		r.title = title
		r.broadcastLocked(windowTitleMsg{Type: "window_title", Title: title}, nil)
	}
}

func fetchTitle(ctx context.Context, url string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	res, err := titleClient.Do(req)
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ""
	}
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	return cleanTitle(string(body))
}

// cleanTitle makes a window title fit to show: it is whatever the person
// holding the remote named a window. One line of printable text, not long.
func cleanTitle(s string) string {
	s = strings.Map(func(c rune) rune {
		if unicode.IsControl(c) || c == utf8.RuneError {
			return ' '
		}
		return c
	}, strings.ToValidUTF8(s, ""))
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > maxTitleRunes {
		s = strings.TrimSpace(string([]rune(s)[:maxTitleRunes-1])) + "…"
	}
	return s
}

// memberCheckInterval is how often neko's members are compared with the
// tabs in the room.
const memberCheckInterval = 30 * time.Second

func (r *Room) watchMembers(ctx context.Context) {
	t := time.NewTicker(memberCheckInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.checkMembers(ctx)
		}
	}
}

// checkMembers makes neko's members match the tabs in the room: members
// that belong to no tab are deleted, and profiles that differ from the
// tab's rights are set again. Both happen when a call to neko failed
// earlier, and when someone used neko's admin token (which the room's
// desktop user can read) to add a member or raise their own rights.
func (r *Room) checkMembers(ctx context.Context) {
	members, err := r.neko.ListMembers(ctx)
	if err != nil {
		return // neko is down or restarting; run notices and logs that
	}
	for _, m := range members {
		if m.ID == neko.ObserverID {
			continue
		}
		r.mu.Lock()
		c := r.clients[m.ID]
		r.mu.Unlock()
		if c == nil {
			r.log.Warn("deleting a neko member that belongs to no tab", "member", m.ID, "name", m.Profile.Name)
			if err := r.neko.DeleteMember(ctx, m.ID); err != nil && !errors.Is(err, neko.ErrNotFound) {
				r.log.Warn("delete neko member", "member", m.ID, "err", err)
			}
			continue
		}
		c.nekoMu.Lock()
		if want := r.nekoProfile(c); !c.gone && !sameProfile(m.Profile, want) {
			r.log.Warn("resetting a neko member's profile", "client", c.ID)
			if err := r.neko.UpdateProfile(ctx, c.ID, want); err != nil && !errors.Is(err, neko.ErrNotFound) {
				r.log.Warn("update neko member", "client", c.ID, "err", err)
			}
		}
		c.nekoMu.Unlock()
	}
}

// sameProfile reports whether neko's profile got grants what want does.
// neko may list plugin settings we never set; those are ignored.
func sameProfile(got, want neko.Profile) bool {
	for k, v := range want.Plugins {
		if got.Plugins[k] != v {
			return false
		}
	}
	got.Plugins, want.Plugins = nil, nil
	return reflect.DeepEqual(got, want)
}

// prepareNeko removes neko members left from a previous run (CozyCast owns
// all of them) and applies room settings neko enforces.
func (r *Room) prepareNeko(ctx context.Context) error {
	members, err := r.neko.ListMembers(ctx)
	if err != nil {
		return err
	}
	for _, m := range members {
		if err := r.neko.DeleteMember(ctx, m.ID); err != nil && !errors.Is(err, neko.ErrNotFound) {
			return err
		}
	}
	r.mu.Lock()
	set := r.settings
	r.mu.Unlock()
	if err := r.applyScreen(ctx, set.Screen); err != nil {
		return err
	}
	return r.neko.SetImplicitHosting(ctx, !set.RemoteOwnership)
}

// applyScreen sets the desktop resolution; "" restores the configured default.
// Setting the size it already has is skipped: it would restart the stream.
func (r *Room) applyScreen(ctx context.Context, screen string) error {
	if screen == "" {
		screen = r.defaultScreen
	}
	if screen == "" {
		return nil
	}
	size, err := neko.ParseScreen(screen)
	if err != nil {
		return err
	}
	if current, err := r.neko.Screen(ctx); err == nil && current == size {
		return nil
	}
	return r.neko.SetScreen(ctx, size)
}

// reapplyNekoSettings restores the settings neko holds only at runtime. It
// runs whenever the event stream reconnects, because neko may have
// restarted (container restart, crash) and come back with its defaults.
func (r *Room) reapplyNekoSettings(ctx context.Context) {
	r.mu.Lock()
	set := r.settings
	r.mu.Unlock()
	if err := r.applyScreen(ctx, set.Screen); err != nil {
		r.log.Warn("reapply neko screen", "err", err)
	}
	if err := r.neko.SetImplicitHosting(ctx, !set.RemoteOwnership); err != nil {
		r.log.Warn("reapply neko implicit hosting", "err", err)
	}
}

// ---- joining and leaving --------------------------------------------------

type JoinRequest struct {
	Identity   auth.Identity
	AccessCode string // temporary access invite, optional
	Send       func(any)
	Kill       func()
}

// Join admits a new tab, sends it the welcome message and announces it.
func (r *Room) Join(ctx context.Context, req JoinRequest) (*Client, error) {
	id := req.Identity
	in := rights.Input{User: id.User, Now: time.Now().Unix()}
	var err error
	if id.User != nil {
		in.Perm, err = r.hub.store.Permission(ctx, r.Name, id.User.ID)
	} else {
		in.AnonBan, err = r.hub.store.AnonBan(ctx, r.Name, id.AnonID, id.IP)
		if errors.Is(err, store.ErrNotFound) {
			in.AnonBan, err = nil, nil
		}
	}
	if err != nil {
		return nil, err
	}
	if req.AccessCode != "" {
		inv, err := r.hub.store.Invite(ctx, req.AccessCode)
		if err == nil && inv.Temporary && inv.Room == r.Name && inv.Valid(in.Now) {
			in.Grant = &rights.Grant{Remote: inv.Remote, Image: inv.Image, Upload: inv.Upload}
		}
	}

	r.mu.Lock()
	in.Room = r.settings
	r.mu.Unlock()
	if d := rights.Admit(in); d != nil {
		return nil, &DeniedError{*d}
	}
	// Count the access invite only once it actually let someone in.
	if in.Grant != nil {
		_, err := r.hub.store.UseAccessInvite(ctx, req.AccessCode, r.Name)
		if errors.Is(err, store.ErrInvalidInvite) {
			in.Grant = nil
			if d := rights.Admit(in); d != nil {
				return nil, &DeniedError{*d}
			}
		} else if err != nil {
			return nil, err
		}
	}

	c := &Client{ID: "c-" + randomID(), send: req.Send, kill: req.Kill, active: true,
		sessionHash: slices.Clone(id.SessionHash), ip: id.IP}
	now := time.Now().UnixMilli()

	r.mu.Lock()
	c.joinedAt = time.Now()
	key := id.Key()
	m := r.members[key]
	isNew := m == nil
	if isNew {
		m = &member{key: key, anonID: id.AnonID, joinedAt: now, lastSeen: now, clients: make(map[string]*Client)}
		r.members[key] = m
	}
	m.user, m.perm = id.User, in.Perm
	if in.Grant != nil {
		m.grant = mergeGrant(m.grant, in.Grant)
	}
	c.m = m
	m.clients[c.ID] = c
	r.clients[c.ID] = c
	changed := r.recomputeLocked(m)

	// History is read under the lock: a chat message is either in it or
	// broadcast to this client afterwards, never lost in between.
	history, err := r.hub.store.ChatHistory(ctx, r.Name)
	if err != nil {
		r.removeClientLocked(c)
		r.mu.Unlock()
		return nil, err
	}
	c.send(welcomeMsg{
		Type:     "welcome",
		ClientID: c.ID,
		Self:     r.userLocked(m),
		Rights:   m.rights,
		Settings: r.publicSettingsLocked(),
		Users:    r.usersLocked(),
		History:  r.toChat(history),
		Remote:   r.holderLocked(),
		Restart:  r.restart != nil,

		WindowTitle: r.title,
	})
	var resync []*Client
	if isNew {
		r.broadcastLocked(userMsg{Type: "user_joined", User: r.userLocked(m)}, m)
	} else if changed {
		resync = r.pushRightsLocked(m)
	}
	r.mu.Unlock()

	r.syncNeko(ctx, resync)
	r.log.Info("client joined", "client", c.ID, "identity", key)
	return c, nil
}

// Leave removes a tab; the person leaves once their last tab is gone.
func (r *Room) Leave(ctx context.Context, c *Client) {
	r.mu.Lock()
	r.removeClientLocked(c)
	r.mu.Unlock()

	// Under nekoMu, so a token request still in flight cannot create the
	// member after it was deleted.
	c.nekoMu.Lock()
	c.gone = true
	if err := r.neko.DeleteMember(ctx, c.ID); err != nil && !errors.Is(err, neko.ErrNotFound) {
		r.log.Warn("delete neko member", "client", c.ID, "err", err)
	}
	c.nekoMu.Unlock()
	r.log.Info("client left", "client", c.ID)
}

func (r *Room) removeClientLocked(c *Client) {
	if _, ok := r.clients[c.ID]; !ok {
		return
	}
	delete(r.clients, c.ID)
	delete(r.tokens, c.nekoToken)
	c.nekoToken = ""
	for conn := range c.nekoConns {
		conn.Close()
	}
	c.nekoConns = nil
	m := c.m
	before := r.userLocked(m)
	delete(m.clients, c.ID)
	if len(m.clients) == 0 {
		delete(r.members, m.key)
		r.broadcastLocked(userLeftMsg{Type: "user_left", Key: m.key}, nil)
		return
	}
	if after := r.userLocked(m); after != before {
		r.broadcastLocked(userMsg{Type: "user_updated", User: after}, nil)
	}
}

// ---- rights ---------------------------------------------------------------

// recomputeLocked updates m.rights from its inputs and reports a change.
func (r *Room) recomputeLocked(m *member) bool {
	next := rights.Compute(rights.Input{Room: r.settings, User: m.user, Perm: m.perm, Grant: m.grant})
	if next == m.rights {
		return false
	}
	m.rights = next
	return true
}

// pushRightsLocked tells m's tabs about their new rights and returns the
// tabs whose neko member must be updated (outside the lock).
func (r *Room) pushRightsLocked(m *member) []*Client {
	clients := make([]*Client, 0, len(m.clients))
	for _, c := range m.clients {
		c.send(rightsMsg{Type: "rights", Rights: m.rights})
		clients = append(clients, c)
	}
	return clients
}

// refreshUser re-reads an account and its permission in this room.
func (r *Room) refreshUser(ctx context.Context, userID int64) {
	key := "u:" + strconv.FormatInt(userID, 10)
	r.mu.Lock()
	_, present := r.members[key]
	r.mu.Unlock()
	if !present {
		return
	}

	u, err := r.hub.store.UserByID(ctx, userID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && u.Disabled) {
		r.kick(key, kickedMsg{Type: "kicked", Reason: "deleted"})
		return
	}
	if err != nil {
		r.log.Warn("refresh user", "user", userID, "err", err)
		return
	}
	perm, err := r.hub.store.Permission(ctx, r.Name, userID)
	if err != nil {
		r.log.Warn("refresh permission", "user", userID, "err", err)
		return
	}

	r.mu.Lock()
	m := r.members[key]
	if m == nil {
		r.mu.Unlock()
		return
	}
	before := r.userLocked(m)
	m.user, m.perm = u, perm
	in := rights.Input{Room: r.settings, User: u, Perm: perm, Grant: m.grant, Now: time.Now().Unix()}
	if d := rights.Admit(in); d != nil {
		r.kickLocked(m, kickedMsg{Type: "kicked", Reason: d.Reason, BannedUntil: d.BannedUntil})
		r.mu.Unlock()
		return
	}
	var resync []*Client
	if r.recomputeLocked(m) {
		resync = r.pushRightsLocked(m)
	}
	if after := r.userLocked(m); after != before {
		r.broadcastLocked(userMsg{Type: "user_updated", User: after}, nil)
		resync = m.clientList() // the neko display name may have changed
	}
	r.mu.Unlock()
	r.syncNeko(ctx, resync)
}

// reloadSettings applies changed room settings to everyone present.
func (r *Room) reloadSettings(ctx context.Context) {
	s, err := r.hub.store.RoomSettings(ctx, r.Name)
	if err != nil {
		r.log.Warn("reload room settings", "err", err)
		return
	}

	r.mu.Lock()
	ownershipChanged := s.RemoteOwnership != r.settings.RemoteOwnership
	screenChanged := s.Screen != r.settings.Screen
	stream := r.streamLocked()
	r.settings = s
	r.broadcastLocked(settingsMsg{Type: "room_settings", Settings: r.publicSettingsLocked()}, nil)
	r.pinStreamLocked(stream)
	var resync []*Client
	now := time.Now().Unix()
	for _, m := range r.members {
		// Anonymous bans do not depend on settings, so they are not re-checked.
		in := rights.Input{Room: s, User: m.user, Perm: m.perm, Grant: m.grant, Now: now}
		if d := rights.Admit(in); d != nil {
			r.kickLocked(m, kickedMsg{Type: "kicked", Reason: d.Reason, BannedUntil: d.BannedUntil})
			continue
		}
		if r.recomputeLocked(m) {
			resync = append(resync, r.pushRightsLocked(m)...)
		}
	}
	r.mu.Unlock()

	if ownershipChanged {
		if err := r.neko.SetImplicitHosting(ctx, !s.RemoteOwnership); err != nil {
			r.log.Warn("set neko implicit hosting", "err", err)
		}
	}
	if screenChanged {
		if err := r.applyScreen(ctx, s.Screen); err != nil {
			r.log.Warn("set neko screen", "screen", s.Screen, "err", err)
		}
	}
	r.syncNeko(ctx, resync)
}

// ---- moderation -----------------------------------------------------------

// Ban bans the person with the given identity key from this room until
// `until` (unix seconds, nil = forever) and disconnects them. Accounts are
// banned by account; anonymous users by browser id and IP. All anonymous
// identities with a tab on that IP are disconnected too.
func (r *Room) Ban(ctx context.Context, key string, until *int64) error {
	r.mu.Lock()
	m := r.members[key]
	if m == nil {
		r.mu.Unlock()
		return ErrNotPresent
	}
	user, anonID := m.user, m.anonID
	// Use the most recently joined tab's IP for the stored anonymous ban.
	var latest *Client
	for _, c := range m.clients {
		if latest == nil || c.joinedAt.After(latest.joinedAt) {
			latest = c
		}
	}
	var ip string
	if latest != nil {
		ip = latest.ip
	}
	r.mu.Unlock()

	var err error
	if user != nil {
		err = r.hub.store.BanUser(ctx, r.Name, user.ID, until)
	} else {
		err = r.hub.store.AddAnonBan(ctx, r.Name, anonID, ip, until)
	}
	if err != nil {
		return err
	}
	msg := kickedMsg{Type: "kicked", Reason: "banned", BannedUntil: until}
	if user != nil {
		r.kick(key, msg)
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range r.members {
		if m.user != nil {
			continue
		}
		if m.key == key {
			r.kickLocked(m, msg)
			continue
		}
		for _, c := range m.clients {
			if ip != "" && c.ip == ip {
				r.kickLocked(m, msg)
				break
			}
		}
	}
	return nil
}

// Kick disconnects a person without banning them; they may rejoin.
func (r *Room) Kick(key string) error {
	if !r.kick(key, kickedMsg{Type: "kicked", Reason: "kicked"}) {
		return ErrNotPresent
	}
	return nil
}

func (r *Room) kick(key string, msg kickedMsg) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.members[key]
	if m == nil {
		return false
	}
	r.kickLocked(m, msg)
	return true
}

// kickLocked tells every tab of m why, removes them from the room and closes
// them. They are gone at once: whatever a tab still sends while its socket
// closes is ignored (see Handle). The sockets call Leave themselves, which
// deletes their neko members.
func (r *Room) kickLocked(m *member, msg kickedMsg) {
	for _, c := range m.clientList() {
		r.kickClientLocked(c, msg)
	}
	m.rights = rights.Rights{}
}

func (r *Room) kickClientLocked(c *Client, msg kickedMsg) {
	c.send(msg)
	r.removeClientLocked(c)
	c.kill()
}

// ResetRemote takes the remote from whoever holds it.
func (r *Room) ResetRemote(ctx context.Context) error {
	return r.neko.ResetControl(ctx)
}

// ---- neko -----------------------------------------------------------------

// NekoToken returns a fresh neko session token for the tab, creating its
// neko member first if needed (first call, or neko restarted).
func (r *Room) NekoToken(ctx context.Context, c *Client) (string, error) {
	select {
	case <-r.ready:
	case <-ctx.Done():
		return "", ctx.Err()
	}

	c.nekoMu.Lock()
	defer c.nekoMu.Unlock()
	if c.gone {
		return "", ErrNotPresent
	}

	if c.nekoPassword != "" {
		token, err := r.neko.Login(ctx, c.ID, c.nekoPassword)
		if err == nil {
			return r.issueToken(c, token)
		}
		r.log.Info("neko login failed, recreating member", "client", c.ID, "err", err)
		_ = r.neko.DeleteMember(ctx, c.ID)
	}

	password := randomID() + randomID()
	if err := r.neko.CreateMember(ctx, c.ID, password, r.nekoProfile(c)); err != nil {
		return "", fmt.Errorf("create neko member: %w", err)
	}
	c.nekoPassword = password
	token, err := r.neko.Login(ctx, c.ID, password)
	if err != nil {
		return "", err
	}
	return r.issueToken(c, token)
}

// issueToken makes token the one the tab reaches neko with (see AttachNeko).
func (r *Room) issueToken(c *Client, token string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, present := r.clients[c.ID]; !present {
		return "", ErrNotPresent // kicked meanwhile; Leave deletes the member
	}
	delete(r.tokens, c.nekoToken)
	c.nekoToken = token
	r.tokens[token] = c
	return token, nil
}

// NekoConn is one connection of a tab to neko's WebSocket, as the HTTP layer
// proxies it. Send passes a message on to neko as if the tab had sent it;
// Close ends the connection. Neither may block.
type NekoConn struct {
	Send  func(msg []byte)
	Close func()
}

// NekoTokenIssued reports whether the hub issued token to a tab that is
// still in the room. Only such tokens are let through to neko: neko accepts
// the token of any of its members, also of members that someone with access
// to the room's desktop created behind the hub's back.
func (r *Room) NekoTokenIssued(token string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tokens[token] != nil
}

// NekoFilesAllowed reports whether the tab that token was issued to may use
// the desktop's files: upload into its Downloads folder, see what is in it
// and download from it. That is the upload right.
func (r *Room) NekoFilesAllowed(token string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.tokens[token]
	return c != nil && c.m.rights.Upload
}

// AttachNeko registers a proxied neko connection opened with token, so the
// hub can keep it on the room's stream and close it when the tab leaves.
// ok is false if NekoTokenIssued is.
func (r *Room) AttachNeko(token string, conn *NekoConn) (detach func(), ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.tokens[token]
	if c == nil {
		return nil, false
	}
	if c.nekoConns == nil {
		c.nekoConns = make(map[*NekoConn]struct{})
	}
	c.nekoConns[conn] = struct{}{}
	return func() {
		r.mu.Lock()
		delete(c.nekoConns, conn)
		r.mu.Unlock()
	}, true
}

// Stream is the capture pipeline everyone in the room watches: the room's
// setting, or neko's default if neko does not offer that. It is "" for
// neko's default while neko has not reported its pipelines yet.
func (r *Room) Stream() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.streamLocked()
}

func (r *Room) streamLocked() string {
	if len(r.streams) == 0 || slices.Contains(r.streams, r.settings.Stream) {
		return r.settings.Stream
	}
	return r.streams[0]
}

// pinStreamLocked moves every connection to the room's stream if that is no
// longer before. Browsers switch on their own when the setting changes; this
// is for the ones that do not, which would keep a second encoder running.
func (r *Room) pinStreamLocked(before string) {
	stream := r.streamLocked()
	if stream == before || stream == "" {
		return
	}
	msg := neko.SelectStream(stream)
	for _, c := range r.clients {
		for conn := range c.nekoConns {
			conn.Send(msg)
		}
	}
}

// syncNeko pushes current rights and names to the tabs' neko members.
func (r *Room) syncNeko(ctx context.Context, clients []*Client) {
	for _, c := range clients {
		c.nekoMu.Lock()
		if c.nekoPassword != "" {
			if err := r.neko.UpdateProfile(ctx, c.ID, r.nekoProfile(c)); err != nil && !errors.Is(err, neko.ErrNotFound) {
				r.log.Warn("update neko member", "client", c.ID, "err", err)
			}
		}
		c.nekoMu.Unlock()
	}
}

func (r *Room) nekoProfile(c *Client) neko.Profile {
	r.mu.Lock()
	name := r.userLocked(c.m).Nickname
	rt := c.m.rights
	r.mu.Unlock()
	return neko.Profile{
		Name:               name,
		CanLogin:           true,
		CanConnect:         true,
		CanWatch:           true,
		CanHost:            rt.Remote,
		CanAccessClipboard: rt.Remote,
		Plugins: map[string]any{
			"filetransfer.enabled": rt.Upload,
			"chat.can_send":        false,
			"chat.can_receive":     false,
		},
	}
}

// setHost is called by the neko event stream when the remote changes hands.
func (r *Room) setHost(hostID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if hostID == r.hostID {
		return
	}
	r.hostID = hostID
	r.broadcastLocked(remoteMsg{Type: "remote", Holder: r.holderLocked()}, nil)
}

func (r *Room) holderLocked() *string {
	if c := r.clients[r.hostID]; c != nil {
		return &c.m.key
	}
	return nil
}

// ---- helpers --------------------------------------------------------------

// broadcastLocked sends msg to every tab except those of `except`.
func (r *Room) broadcastLocked(msg any, except *member) {
	for _, c := range r.clients {
		if except != nil && c.m == except {
			continue
		}
		c.send(msg)
	}
}

func (m *member) clientList() []*Client {
	list := make([]*Client, 0, len(m.clients))
	for _, c := range m.clients {
		list = append(list, c)
	}
	return list
}

func (r *Room) usersLocked() []User {
	users := make([]User, 0, len(r.members))
	for _, m := range r.members {
		users = append(users, r.userLocked(m))
	}
	return users
}

func (r *Room) userLocked(m *member) User {
	u := User{
		Key:       m.key,
		Admin:     m.rights.Admin,
		JoinedAt:  m.joinedAt,
		LastSeen:  m.lastSeen,
		Anonymous: m.user == nil,
		Muted:     true,
	}
	for _, c := range m.clients {
		u.Active = u.Active || c.active
		u.Muted = u.Muted && c.muted
	}
	if m.user != nil {
		u.Username, u.Nickname, u.NameColor, u.AvatarURL = m.user.Username, m.user.Nickname, m.user.NameColor, avatarURL(m.user)
	} else {
		u.Nickname, u.NameColor, u.AvatarURL = "Anonymous", anonColor(m.anonID), "/png/default_avatar_on_alpha.png"
	}
	return u
}

func avatarURL(u *store.User) string {
	if u.Avatar == "" {
		return "/png/default_avatar.png"
	}
	return "/media/avatars/" + u.Avatar
}

// anonColor gives each anonymous browser a stable, readable name colour.
func anonColor(anonID string) string {
	h := fnv.New32a()
	h.Write([]byte(anonID))
	return hslHex(float64(h.Sum32()%360), 0.65, 0.65)
}

func hslHex(hue, sat, light float64) string {
	c := (1 - math.Abs(2*light-1)) * sat
	x := c * (1 - math.Abs(math.Mod(hue/60, 2)-1))
	m := light - c/2
	var rf, gf, bf float64
	switch {
	case hue < 60:
		rf, gf, bf = c, x, 0
	case hue < 120:
		rf, gf, bf = x, c, 0
	case hue < 180:
		rf, gf, bf = 0, c, x
	case hue < 240:
		rf, gf, bf = 0, x, c
	case hue < 300:
		rf, gf, bf = x, 0, c
	default:
		rf, gf, bf = c, 0, x
	}
	to := func(v float64) int { return int((v+m)*255 + 0.5) }
	return fmt.Sprintf("#%02x%02x%02x", to(rf), to(gf), to(bf))
}

func mergeGrant(a, b *rights.Grant) *rights.Grant {
	if a == nil {
		return b
	}
	return &rights.Grant{Remote: a.Remote || b.Remote, Image: a.Image || b.Image, Upload: a.Upload || b.Upload}
}

func randomID() string {
	b := make([]byte, 9)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// Settings returns the room's current settings.
func (r *Room) Settings() store.RoomSettings {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.settings
}

// Streams returns the capture pipelines the room's neko offers (default
// first), or nil while neko has not reported them yet.
func (r *Room) Streams() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.streams)
}

// publicSettingsLocked is what browsers get: a stream neko does not offer
// (e.g. after the operator changed the stream list) falls back to neko's
// default instead of leaving viewers without video.
func (r *Room) publicSettingsLocked() store.RoomSettings {
	set := r.settings
	if set.Stream != "" && r.streams != nil && !slices.Contains(r.streams, set.Stream) {
		set.Stream = ""
	}
	return set
}
