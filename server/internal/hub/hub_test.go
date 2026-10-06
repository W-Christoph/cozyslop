package hub

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"cozycast/internal/auth"
	"cozycast/internal/neko"
	"cozycast/internal/neko/nekotest"
	"cozycast/internal/rights"
	"cozycast/internal/store"
)

const testTimeout = 5 * time.Second

type recording struct {
	mu       sync.Mutex
	messages []any
	kills    int
	consumed map[int]bool
	changed  chan struct{}
}

func newRecording() *recording {
	return &recording{changed: make(chan struct{}, 1), consumed: make(map[int]bool)}
}
func (r *recording) send(m any) {
	r.mu.Lock()
	r.messages = append(r.messages, m)
	r.mu.Unlock()
	select {
	case r.changed <- struct{}{}:
	default:
	}
}
func (r *recording) kill()     { r.mu.Lock(); defer r.mu.Unlock(); r.kills++ }
func messageType(m any) string { return reflect.ValueOf(m).FieldByName("Type").String() }
func (r *recording) snapshot() []any {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]any(nil), r.messages...)
}
func (r *recording) count(typ string) int {
	n := 0
	for _, m := range r.snapshot() {
		if messageType(m) == typ {
			n++
		}
	}
	return n
}
func (r *recording) wait(t *testing.T, typ string) any {
	t.Helper()
	timer := time.NewTimer(testTimeout)
	defer timer.Stop()
	for {
		r.mu.Lock()
		for i, m := range r.messages {
			if !r.consumed[i] && messageType(m) == typ {
				r.consumed[i] = true
				r.mu.Unlock()
				return m
			}
		}
		r.mu.Unlock()
		select {
		case <-r.changed:
		case <-timer.C:
			t.Fatalf("timeout waiting for %s; messages: %#v", typ, r.snapshot())
			return nil
		}
	}
}
func (r *recording) assertKills(t *testing.T, n int) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.kills != n {
		t.Fatalf("kills=%d, want %d", r.kills, n)
	}
}
func requireOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func requireEqual[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

type fixture struct {
	t    *testing.T
	ctx  context.Context
	s    *store.Store
	h    *Hub
	r    *Room
	fake *nekotest.Server
}

func newFixture(t *testing.T, set store.RoomSettings, additional ...store.RoomSettings) *fixture {
	return newFixtureWithRestart(t, set, nil, additional...)
}

func newFixtureWithRestart(t *testing.T, set store.RoomSettings, restart func(context.Context) error, additional ...store.RoomSettings) *fixture {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s, err := store.Open(ctx, ":memory:")
	requireOK(t, err)
	t.Cleanup(func() { requireOK(t, s.Close()) })
	if set.Name == "" {
		set.Name = "main"
	}
	settings := append([]store.RoomSettings{set}, additional...)
	var configs []RoomConfig
	var fakes []*nekotest.Server
	for _, settings := range settings {
		if settings.Access == "" {
			settings.Access = "public"
		}
		requireOK(t, s.SaveRoomSettings(ctx, settings))
		fake := nekotest.New(t, "admin-secret")
		nc, err := neko.NewClient(fake.URL(), "admin-secret")
		requireOK(t, err)
		configs = append(configs, RoomConfig{Name: settings.Name, Neko: nc, Restart: restart})
		fakes = append(fakes, fake)
	}
	h := New(s, t.TempDir(), configs)
	requireOK(t, h.Start(ctx))
	t.Cleanup(func() {
		cancel()
		waitCtx, stop := observerDeadline()
		defer stop()
		for _, fake := range fakes {
			requireOK(t, fake.WaitObservers(waitCtx, 0))
		}
	})
	waitCtx, stop := context.WithTimeout(ctx, testTimeout)
	defer stop()
	for _, r := range h.Rooms() {
		select {
		case <-r.ready:
		case <-waitCtx.Done():
			t.Fatal("room not ready", r.Name)
		}
	}
	for _, fake := range fakes {
		requireOK(t, fake.WaitObservers(waitCtx, 1))
	}
	// The observer's first report broadcasts room_settings; let it pass so
	// tests only see the broadcasts they cause.
	for _, r := range h.Rooms() {
		for r.Streams() == nil {
			select {
			case <-waitCtx.Done():
				t.Fatal("no streams reported", r.Name)
			case <-time.After(time.Millisecond):
			}
		}
	}
	return &fixture{t, ctx, s, h, h.Room(set.Name), fakes[0]}
}
func anon(id string) auth.Identity {
	return auth.Identity{AnonID: id, IP: "192.0.2." + fmt.Sprint(len(id))}
}
func (f *fixture) user(name string) auth.Identity {
	f.t.Helper()
	u := &store.User{Username: name, Nickname: name + " nick", NameColor: "#123456", Avatar: name + ".png"}
	requireOK(f.t, f.s.CreateUser(f.ctx, u))
	return auth.Identity{User: u, AnonID: "browser-" + name, IP: "198.51.100.1"}
}
func (f *fixture) flags(id auth.Identity, admin, verified bool) auth.Identity {
	f.t.Helper()
	requireOK(f.t, f.s.UpdateFlags(f.ctx, id.User.ID, admin, verified, false))
	u, err := f.s.UserByID(f.ctx, id.User.ID)
	requireOK(f.t, err)
	id.User = u
	return id
}
func (f *fixture) permission(id auth.Identity, p store.Permission) {
	f.t.Helper()
	p.Room = f.r.Name
	p.UserID = id.User.ID
	requireOK(f.t, f.s.SavePermission(f.ctx, p))
}
func (f *fixture) join(id auth.Identity, code ...string) (*Client, *recording) {
	f.t.Helper()
	rec := newRecording()
	req := JoinRequest{Identity: id, Send: rec.send, Kill: rec.kill}
	if len(code) > 0 {
		req.AccessCode = code[0]
	}
	c, err := f.r.Join(f.ctx, req)
	requireOK(f.t, err)
	f.t.Cleanup(func() { f.r.Leave(context.Background(), c) })
	return c, rec
}
func assertDenied(t *testing.T, r *Room, ctx context.Context, id auth.Identity, code, reason string, until *int64) {
	t.Helper()
	rec := newRecording()
	c, err := r.Join(ctx, JoinRequest{Identity: id, AccessCode: code, Send: rec.send, Kill: rec.kill})
	var denied *DeniedError
	if c != nil || !errors.As(err, &denied) {
		t.Fatalf("Join=(%v,%v), want DeniedError", c, err)
	}
	requireEqual(t, denied.Denial.Reason, reason)
	if !reflect.DeepEqual(denied.Denial.BannedUntil, until) {
		t.Fatalf("until=%v, want %v", denied.Denial.BannedUntil, until)
	}
	requireEqual(t, len(rec.snapshot()), 0)
	rec.assertKills(t, 0)
}
func assertProfile(t *testing.T, fake *nekotest.Server, c *Client, name string, rt rights.Rights) {
	t.Helper()
	p, ok := fake.Member(c.ID)
	if !ok {
		t.Fatalf("missing neko member %s", c.ID)
	}
	want := neko.Profile{Name: name, CanLogin: true, CanConnect: true, CanWatch: true, CanHost: rt.Remote, CanAccessClipboard: rt.Remote, Plugins: map[string]any{"filetransfer.enabled": rt.Upload, "chat.can_send": false, "chat.can_receive": false}}
	if !reflect.DeepEqual(p, want) {
		t.Fatalf("profile=%#v, want %#v", p, want)
	}
}

func TestStartup(t *testing.T) {
	for _, ownership := range []bool{false, true} {
		t.Run(fmt.Sprint(ownership), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s, err := store.Open(ctx, ":memory:")
			requireOK(t, err)
			defer s.Close()
			requireOK(t, s.SaveRoomSettings(ctx, store.RoomSettings{Name: "main", Access: "public", RemoteOwnership: ownership}))
			fake := nekotest.New(t, "secret")
			nc, err := neko.NewClient(fake.URL(), "secret")
			requireOK(t, err)
			for _, id := range []string{"stale", neko.ObserverID} {
				requireOK(t, nc.CreateMember(ctx, id, "old", neko.Profile{CanLogin: true}))
			}
			h := New(s, t.TempDir(), []RoomConfig{{Name: "main", Neko: nc}})
			rec := newRecording()
			c, err := h.Room("main").Join(ctx, JoinRequest{Identity: anon("new"), Send: rec.send, Kill: rec.kill})
			requireOK(t, err)
			done := make(chan struct{})
			go func() { defer close(done); h.Room("main").SendNekoToken(ctx, c) }()
			requireEqual(t, rec.count("neko"), 0)
			requireOK(t, h.Start(ctx))
			token := rec.wait(t, "neko").(nekoMsg)
			if token.Token == "" {
				t.Fatal("empty token")
			}
			requireEqual(t, token.Path, "/neko/main")
			<-done
			waitCtx, stop := context.WithTimeout(ctx, testTimeout)
			defer stop()
			requireOK(t, fake.WaitObservers(waitCtx, 1))
			requireEqual(t, fake.ImplicitHosting(), !ownership)
			if _, ok := fake.Member("stale"); ok {
				t.Fatal("stale member survived startup")
			}
			calls := fake.Calls()
			removed := map[string]bool{}
			for _, call := range calls {
				if call.Method == "DELETE" {
					removed[call.MemberID] = true
				}
				if call.Path == "/api/login" && (!removed["stale"] || !removed[neko.ObserverID]) {
					t.Fatal("login issued before stale cleanup", calls)
				}
			}
			p, ok := fake.Member(neko.ObserverID)
			if !ok || !p.IsAdmin || !p.CanLogin || !p.CanConnect || p.CanHost || p.CanWatch {
				t.Fatalf("observer profile=%#v", p)
			}
			h.Room("main").Leave(ctx, c)
			cancel()
			closeCtx, closeCancel := observerDeadline()
			defer closeCancel()
			requireOK(t, fake.WaitObservers(closeCtx, 0))
		})
	}
}

func TestJoinWelcomeAndTabs(t *testing.T) {
	set := store.RoomSettings{Name: "main", Access: "public", RemoteOwnership: true, DefaultRemote: true}
	f := newFixture(t, set)
	id := f.user("alice")
	f.permission(id, store.Permission{Image: true, Upload: true})
	host, watch := f.join(anon("holder"))
	watch.wait(t, "welcome")
	for _, body := range []string{"oldest", "newest"} {
		f.r.Handle(f.ctx, host, ClientMsg{Type: "chat_send", Body: body})
		watch.wait(t, "chat")
	}
	f.fake.SetHost(host.ID)
	requireEqual(t, *watch.wait(t, "remote").(remoteMsg).Holder, "a:holder")
	c, rec := f.join(id)
	w := rec.wait(t, "welcome").(welcomeMsg)
	requireEqual(t, w.ClientID, c.ID)
	requireEqual(t, w.Self.Key, id.Key())
	requireEqual(t, w.Self.Username, "alice")
	requireEqual(t, w.Self.Nickname, "alice nick")
	requireEqual(t, w.Self.NameColor, "#123456")
	requireEqual(t, w.Self.AvatarURL, "/media/avatars/alice.png")
	requireEqual(t, w.Self.Anonymous, false)
	requireEqual(t, w.Self.Active, true)
	requireEqual(t, w.Self.Muted, false)
	if w.Self.JoinedAt <= 0 {
		t.Fatal("missing join time")
	}
	requireEqual(t, w.Rights, rights.Rights{Remote: true, Image: true, Upload: true})
	requireEqual(t, w.Settings, set)
	requireEqual(t, len(w.Users), 2)
	if !slices.ContainsFunc(w.Users, func(u User) bool { return u == w.Self }) {
		t.Fatal("self missing from users")
	}
	requireEqual(t, len(w.History), 2)
	requireEqual(t, w.History[0].Body, "oldest")
	requireEqual(t, w.History[1].Body, "newest")
	if w.History[0].ID >= w.History[1].ID {
		t.Fatal("history not oldest first")
	}
	requireEqual(t, *w.Remote, "a:holder")
	requireEqual(t, watch.wait(t, "user_joined").(userMsg).User.Key, id.Key())
	tab, tabRec := f.join(id)
	requireEqual(t, tabRec.wait(t, "welcome").(welcomeMsg).Self.Key, id.Key())
	requireEqual(t, f.r.UserCount(), 2)
	requireEqual(t, watch.count("user_joined"), 1)
	f.r.Leave(f.ctx, c)
	requireEqual(t, watch.count("user_left"), 0)
	requireEqual(t, f.r.UserCount(), 2)
	f.r.Leave(f.ctx, tab)
	requireEqual(t, watch.wait(t, "user_left").(userLeftMsg).Key, id.Key())
	requireEqual(t, f.r.UserCount(), 1)
	_, a := f.join(anon("one"))
	_, b := f.join(anon("two"))
	wa := a.wait(t, "welcome").(welcomeMsg)
	wb := b.wait(t, "welcome").(welcomeMsg)
	if wa.Self.Key == wb.Self.Key {
		t.Fatal("anonymous identities merged")
	}
	requireEqual(t, f.r.UserCount(), 3)
}

func TestNekoTokens(t *testing.T) {
	cases := []struct {
		name string
		anon bool
		p    store.Permission
		want rights.Rights
	}{
		{name: "none"}, {name: "remote", p: store.Permission{Remote: true}, want: rights.Rights{Remote: true}},
		{name: "upload", p: store.Permission{Upload: true}, want: rights.Rights{Upload: true}},
		{name: "trusted", p: store.Permission{Trusted: true}, want: rights.Rights{Trusted: true, Remote: true, Image: true, Upload: true}},
		{name: "anonymous", anon: true, want: rights.Rights{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, store.RoomSettings{})
			id := anon("anon")
			name := "Anonymous"
			if !tc.anon {
				id = f.user("alice")
				f.permission(id, tc.p)
				name = id.User.Nickname
			}
			c, rec := f.join(id)
			requireEqual(t, rec.wait(t, "welcome").(welcomeMsg).Rights, tc.want)
			f.r.SendNekoToken(f.ctx, c)
			first := rec.wait(t, "neko").(nekoMsg)
			if first.Token == "" {
				t.Fatal("empty token")
			}
			assertProfile(t, f.fake, c, name, tc.want)
			tab, tabRec := f.join(id)
			f.r.Handle(f.ctx, tab, ClientMsg{Type: "neko_token"})
			tabRec.wait(t, "neko")
			if tab.ID == c.ID {
				t.Fatal("tabs share neko id")
			}
			assertProfile(t, f.fake, tab, name, tc.want)
			f.r.SendNekoToken(f.ctx, c)
			second := rec.wait(t, "neko").(nekoMsg)
			if first.Token == second.Token {
				t.Fatal("token was not refreshed")
			}
			f.fake.Restart()
			f.r.SendNekoToken(f.ctx, c)
			third := rec.wait(t, "neko").(nekoMsg)
			if third.Token == first.Token || third.Token == second.Token {
				t.Fatal("restart reused token")
			}
			assertProfile(t, f.fake, c, name, tc.want)
			f.r.Leave(f.ctx, c)
			if _, ok := f.fake.Member(c.ID); ok {
				t.Fatal("Leave retained neko member")
			}
		})
	}
}

func TestNekoUnavailable(t *testing.T) {
	for _, entry := range []string{"join", "request"} {
		t.Run(entry, func(t *testing.T) {
			ctx := context.Background()
			s, err := store.Open(ctx, ":memory:")
			requireOK(t, err)
			t.Cleanup(func() { requireOK(t, s.Close()) })
			fake := nekotest.New(t, "admin-secret")
			// An incorrect admin token makes member creation fail at neko.
			nc, err := neko.NewClient(fake.URL(), "wrong-secret")
			requireOK(t, err)
			h := New(s, t.TempDir(), []RoomConfig{{Name: "main", Neko: nc}})
			r := h.Room("main")
			close(r.ready)
			rec := newRecording()
			c, err := r.Join(ctx, JoinRequest{Identity: anon("guest"), Send: rec.send, Kill: rec.kill})
			requireOK(t, err)
			if entry == "join" {
				r.SendNekoToken(ctx, c)
			} else {
				r.Handle(ctx, c, ClientMsg{Type: "neko_token"})
			}
			msg := rec.wait(t, "neko_unavailable").(nekoUnavailableMsg)
			requireEqual(t, msg.Message, "The room's desktop is not reachable right now.")
			requireEqual(t, rec.count("error"), 0)
			requireEqual(t, rec.count("neko"), 0)
			// Neither cancellation nor leaving is a desktop failure.
			canceled, cancel := context.WithCancel(ctx)
			cancel()
			r.SendNekoToken(canceled, c)
			r.Leave(ctx, c)
			r.SendNekoToken(ctx, c)
			requireEqual(t, rec.count("neko_unavailable"), 1)
		})
	}
}

func TestRuntimeRoomLifecycle(t *testing.T) {
	f := newFixture(t, store.RoomSettings{})
	fake := nekotest.New(t, "runtime-secret")
	nc, err := neko.NewClient(fake.URL(), "runtime-secret")
	requireOK(t, err)
	rc := RoomConfig{Name: "added", Source: "registered", Neko: nc}
	requireOK(t, f.h.Add(f.ctx, rc))
	requireEqual(t, f.h.Add(f.ctx, rc), ErrRoomExists)
	r := f.h.Room(rc.Name)
	rec := newRecording()
	c, err := r.Join(f.ctx, JoinRequest{Identity: anon("live"), Send: rec.send, Kill: rec.kill})
	requireOK(t, err)
	rec.wait(t, "welcome")
	token, err := r.NekoToken(f.ctx, c)
	requireOK(t, err)
	waitCtx, cancel := observerDeadline()
	defer cancel()
	requireOK(t, fake.WaitObservers(waitCtx, 1))
	closed := make(chan struct{})
	detach, ok := r.AttachNeko(token, &NekoConn{Send: func([]byte) {}, Close: func() { close(closed) }})
	requireEqual(t, ok, true)
	defer detach()
	r.Handle(f.ctx, c, ClientMsg{Type: "chat_send", Body: "kept across registration"})
	settings := r.Settings()
	settings.DefaultUpload = true
	requireOK(t, f.s.SaveRoomSettings(f.ctx, settings))
	alice := f.user("runtime-user")
	requireOK(t, f.s.SavePermission(f.ctx, store.Permission{Room: rc.Name, UserID: alice.User.ID, Image: true}))
	requireEqual(t, f.h.Remove(rc.Name, "not_found"), true)
	rec.assertKills(t, 1)
	requireEqual(t, rec.wait(t, "kicked").(kickedMsg).Reason, "not_found")
	requireEqual(t, r.UserCount(), 0)
	requireEqual(t, f.h.Room(rc.Name), nil)
	requireEqual(t, r.NekoTokenIssued(token), false)
	select {
	case <-closed:
	default:
		t.Fatal("proxy not closed")
	}
	select {
	case <-r.done:
	default:
		t.Fatal("room work survived removal")
	}
	requireOK(t, fake.WaitObservers(waitCtx, 0))
	requireEqual(t, r.Neko().Connected(), false)
	_, err = r.Join(f.ctx, JoinRequest{Identity: anon("late"), Send: rec.send, Kill: rec.kill})
	var denied *DeniedError
	if !errors.As(err, &denied) || denied.Denial.Reason != "not_found" {
		t.Fatalf("stale room admitted a client: %v", err)
	}
	r.Leave(f.ctx, c)
	requireOK(t, f.h.Add(f.ctx, rc))
	again := f.h.Room(rc.Name)
	requireEqual(t, again.Settings().DefaultUpload, true)
	next := newRecording()
	client, err := again.Join(f.ctx, JoinRequest{Identity: alice, Send: next.send, Kill: next.kill})
	requireOK(t, err)
	welcome := next.wait(t, "welcome").(welcomeMsg)
	requireEqual(t, welcome.Rights.Image, true)
	if len(welcome.History) != 1 || welcome.History[0].Body != "kept across registration" {
		t.Fatalf("history lost: %+v", welcome.History)
	}
	again.Leave(f.ctx, client)
	requireEqual(t, f.h.Remove(rc.Name, "not_found"), true)
	requireEqual(t, f.h.Remove(rc.Name, "not_found"), false)
}

func TestRemoveUnreachableRoomStopsWork(t *testing.T) {
	f := newFixture(t, store.RoomSettings{})
	fake := nekotest.New(t, "secret")
	fake.SetHealthy(false)
	nc, err := neko.NewClient(fake.URL(), "secret")
	requireOK(t, err)
	requireOK(t, f.h.Add(f.ctx, RoomConfig{Name: "offline", Neko: nc}))
	r := f.h.Room("offline")
	rec := newRecording()
	c, err := r.Join(f.ctx, JoinRequest{Identity: anon("waiting"), Send: rec.send, Kill: rec.kill})
	requireOK(t, err)
	done := make(chan struct{})
	go func() { defer close(done); r.SendNekoToken(context.Background(), c) }()
	requireEqual(t, f.h.Remove("offline", "not_found"), true)
	select {
	case <-done:
	case <-time.After(testTimeout):
		t.Fatal("token waiter survived removal")
	}
	select {
	case <-r.done:
	default:
		t.Fatal("health retry survived removal")
	}
	rec.assertKills(t, 1)
	r.Leave(f.ctx, c)
}

func TestRuntimeRoomReaders(t *testing.T) {
	f := newFixture(t, store.RoomSettings{})
	fake := nekotest.New(t, "admin-secret")
	fake.SetHealthy(false)
	nc, err := neko.NewClient(fake.URL(), "admin-secret")
	requireOK(t, err)
	done := make(chan struct{})
	var readers sync.WaitGroup
	for i := 0; i < 4; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				f.h.Room("changing")
				f.h.Rooms()
				f.h.EndSessions(0, nil)
				f.h.UserChanged(f.ctx, 999)
				f.h.PermissionsChanged(f.ctx, "changing", 999)
				f.h.RoomSettingsChanged(f.ctx, "changing")
			}
		}()
	}
	for i := 0; i < 10; i++ {
		requireOK(t, f.h.Add(f.ctx, RoomConfig{Name: "changing", Neko: nc}))
		f.h.Remove("changing", "not_found")
	}
	close(done)
	readers.Wait()
}

func TestRuntimeRoomBecomesReachable(t *testing.T) {
	f := newFixture(t, store.RoomSettings{})
	fake := nekotest.New(t, "later-secret")
	fake.SetHealthy(false)
	nc, err := neko.NewClient(fake.URL(), "later-secret")
	requireOK(t, err)
	requireOK(t, f.h.Add(f.ctx, RoomConfig{Name: "later", Source: "registered", Neko: nc}))
	r := f.h.Room("later")
	t.Cleanup(func() { f.h.Remove("later", "not_found") })
	rec := newRecording()
	client, err := r.Join(f.ctx, JoinRequest{Identity: anon("early"), Send: rec.send, Kill: rec.kill})
	requireOK(t, err)
	done := make(chan struct{})
	go func() { defer close(done); r.SendNekoToken(f.ctx, client) }()
	// Wait until startup actually encountered the unavailable desktop.
	deadline := time.NewTimer(testTimeout)
	defer deadline.Stop()
	for len(fake.Calls()) == 0 {
		select {
		case <-deadline.C:
			t.Fatal("no health retry")
		case <-time.After(time.Millisecond):
		}
	}
	requireEqual(t, rec.count("neko"), 0)
	fake.SetHealthy(true)
	if rec.wait(t, "neko").(nekoMsg).Token == "" {
		t.Fatal("no token after desktop came up")
	}
	select {
	case <-done:
	case <-time.After(testTimeout):
		t.Fatal("token request did not finish")
	}
	r.Leave(f.ctx, client)
}
