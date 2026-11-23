// Package hub runs the rooms: who is connected, their rights, chat, presence
// and the remote. Every browser tab is a Client; all tabs of one person form
// a member. Each Client has its own neko member, so neko enforces the
// member's rights on the desktop.
package hub

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"
	"time"

	"cozycast/internal/anilist"
	"cozycast/internal/diag"
	"cozycast/internal/neko"
	"cozycast/internal/store"
)

const chatSweepInterval = time.Minute

type Hub struct {
	// HibernateAfter is how long a room stays empty before the programs on
	// its desktop are paused; 0 = never. Set before Start.
	HibernateAfter time.Duration
	// AniList finds the shows of the episodes played in rooms, for the
	// watchlog; nil = none are recognised. Set before Start.
	AniList *anilist.Client
	// Watchlog is the watchlog's database; nil = there is no watchlog, and
	// SetWatchlog turns nothing on. Set before Start.
	Watchlog *store.Watchlog
	// Diagnostics keeps the requests to the rooms' helpers; nil = nothing
	// does. Set before Start.
	Diagnostics *diag.Store

	watchlog atomic.Bool // the watchlog is on: rooms follow what is played

	store    *store.Store
	mediaDir string       // chat images and videos
	mu       diag.RWMutex // room map and lifetimes
	ctx      context.Context
	rooms    map[string]*Room
	log      *slog.Logger
	// The rooms when Locks last looked, for when it cannot look.
	lockRooms atomic.Pointer[[]*Room]
}

type RoomConfig struct {
	Source        string // configured (default) | registered
	Name          string
	Neko          *neko.Client
	DefaultScreen string
	// Container hooks are nil when the operator has not enabled control.
	// Running reports whether the room's container is running.
	Restart     func(ctx context.Context) error
	Start, Stop func(ctx context.Context) error
	Running     func(ctx context.Context) (bool, error)
	// TitleURL answers with the title of the window in front on the room's
	// desktop (worker/window-title.py); "" if the room has no such helper.
	TitleURL string
	// PlayURL plays a file of the room desktop's Downloads folder on the
	// desktop (worker/play.py), for requests that carry PlayToken; "" if
	// the room has no such helper.
	PlayURL   string
	PlayToken string
	// StatusURL says what the VLC the play helper started plays, or Firefox,
	// for requests that carry PlayToken; "" if the room has no such helper.
	StatusURL string
	// OpenURL opens a web page in the desktop's Firefox, for requests that
	// carry PlayToken; "" if the room has no such helper.
	OpenURL string
	// HibernateURL pauses and continues the programs on the room's desktop
	// (worker/hibernate.py), for requests that carry PlayToken; "" if the
	// room has no such helper.
	HibernateURL string
}

func New(s *store.Store, mediaDir string, rooms []RoomConfig) *Hub {
	h := &Hub{store: s, mediaDir: mediaDir, rooms: make(map[string]*Room), log: slog.Default()}
	for _, rc := range rooms {
		h.rooms[rc.Name] = h.makeRoom(rc)
	}
	return h
}

// Locks is how long the hub's lock and each room's have been held, found
// without taking them (diag.Watchdog).
func (h *Hub) Locks() []diag.Lock {
	if h.mu.TryRLock() {
		rooms := make([]*Room, 0, len(h.rooms))
		for _, r := range h.rooms {
			rooms = append(rooms, r)
		}
		h.mu.RUnlock()
		h.lockRooms.Store(&rooms)
	}
	locks := []diag.Lock{{Name: "hub", Held: h.mu.Held()}}
	if rooms := h.lockRooms.Load(); rooms != nil {
		for _, r := range *rooms {
			locks = append(locks, diag.Lock{Name: "room " + r.Name, Held: r.mu.Held()})
		}
	}
	return locks
}

// SetWatchlog turns following what is played in the rooms on or off (the
// watchlog setting). Turned off, rooms forget their episode and suggestion
// at their next poll.
func (h *Hub) SetWatchlog(on bool) { h.watchlog.Store(on && h.Watchlog != nil) }

// Room returns the named room, or nil.
func (h *Hub) Room(name string) *Room {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.rooms[name]
}

// Rooms returns all rooms sorted by name.
func (h *Hub) Rooms() []*Room {
	h.mu.RLock()
	defer h.mu.RUnlock()
	list := make([]*Room, 0, len(h.rooms))
	for _, r := range h.rooms {
		list = append(list, r)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list
}

// Start loads every room's settings, then runs background work for the
// rooms until ctx ends. Call it before serving: a room whose settings have
// not been loaded lets everyone in.
func (h *Hub) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.rooms {
		s, err := h.store.RoomSettings(ctx, r.Name)
		if err != nil {
			return fmt.Errorf("load settings of room %s: %w", r.Name, err)
		}
		r.mu.Lock()
		r.settings = s
		r.mu.Unlock()
	}
	h.ctx = ctx
	for _, r := range h.rooms {
		h.startRoom(r)
	}
	go h.sweepChat(ctx)
	go h.refreshAiring(ctx)
	return nil
}

// UserChanged re-reads an account (profile, flags) and applies it to its
// live connections in every room: rights are recomputed, members who may no
// longer be there are disconnected.
func (h *Hub) UserChanged(ctx context.Context, userID int64) {
	for _, r := range h.Rooms() {
		r.refreshUser(ctx, userID)
	}
}

// EndSessions removes a user's tabs in every room, keeping sessionHash if
// supplied. With userID zero, it removes only tabs of sessionHash instead.
// Call after the corresponding session rows have been deleted.
func (h *Hub) EndSessions(userID int64, sessionHash []byte) {
	for _, r := range h.Rooms() {
		r.mu.Lock()
		for _, c := range r.clients {
			if c.m.user == nil {
				continue
			}
			if userID != 0 {
				if c.m.user.ID != userID || (len(sessionHash) > 0 && bytes.Equal(c.sessionHash, sessionHash)) {
					continue
				}
			} else if len(sessionHash) == 0 || !bytes.Equal(c.sessionHash, sessionHash) {
				continue
			}
			r.kickClientLocked(c, kickedMsg{Type: "kicked", Reason: "session"})
		}
		r.mu.Unlock()
	}
}

// PermissionsChanged applies a changed room permission to live connections.
func (h *Hub) PermissionsChanged(ctx context.Context, room string, userID int64) {
	if r := h.Room(room); r != nil {
		r.refreshUser(ctx, userID)
	}
}

// RoomSettingsChanged reloads a room's settings and re-checks everyone in it.
func (h *Hub) RoomSettingsChanged(ctx context.Context, room string) {
	if r := h.Room(room); r != nil {
		r.reloadSettings(ctx)
	}
}

func (h *Hub) sweepChat(ctx context.Context) {
	t := time.NewTicker(chatSweepInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		media, err := h.store.PruneChat(ctx)
		if err != nil {
			h.log.Warn("prune chat", "err", err)
			continue
		}
		h.removeMedia(media)
	}
}

// removeMedia deletes chat media files whose messages are gone.
func (h *Hub) removeMedia(names []string) {
	for _, name := range names {
		if name == "" || filepath.Base(name) != name {
			continue
		}
		if err := os.Remove(filepath.Join(h.mediaDir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			h.log.Warn("remove chat media", "err", err)
		}
	}
}

var ErrRoomExists = errors.New("hub: room already exists")

func (h *Hub) makeRoom(rc RoomConfig) *Room {
	r := newRoom(h, rc.Name, rc.Neko)
	r.Source = rc.Source
	if r.Source == "" {
		r.Source = "configured"
	}
	r.restart, r.defaultScreen, r.titleURL = rc.Restart, rc.DefaultScreen, rc.TitleURL
	r.start, r.stop, r.running = rc.Start, rc.Stop, rc.Running
	r.playURL, r.playToken, r.statusURL, r.openURL = rc.PlayURL, rc.PlayToken, rc.StatusURL, rc.OpenURL
	r.hibernateURL = rc.HibernateURL
	return r
}

func (h *Hub) startRoom(r *Room) {
	ctx, cancel := context.WithCancel(h.ctx)
	r.cancel = cancel
	go func() { defer close(r.done); r.run(ctx) }()
}

// Add loads settings before publishing a room and starts the same work as Start.
func (h *Hub) Add(ctx context.Context, rc RoomConfig) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[rc.Name] != nil {
		return ErrRoomExists
	}
	settings, err := h.store.RoomSettings(ctx, rc.Name)
	if err != nil {
		return err
	}
	r := h.makeRoom(rc)
	r.settings = settings
	if h.ctx != nil {
		h.startRoom(r)
	}
	h.rooms[rc.Name] = r
	return nil
}

// Remove waits for the room's background work before allowing its name to be reused.
// It deliberately leaves all per-room data in the store.
func (h *Hub) Remove(name, reason string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	r := h.rooms[name]
	if r == nil {
		return false
	}
	delete(h.rooms, name)
	r.mu.Lock()
	r.removeCancel()
	for _, c := range r.clients {
		r.kickClientLocked(c, kickedMsg{Type: "kicked", Reason: reason})
	}
	r.mu.Unlock()
	if r.cancel != nil {
		r.cancel()
		<-r.done
	} else {
		close(r.done)
	}
	return true
}

// AniList is asked about the shows that still air this often, and soon
// after one of their episodes airs. Variables for tests.
var (
	airingEvery = 6 * time.Hour
	airingLate  = 15 * time.Minute // after an episode's time, for AniList to know it aired
)

// refreshAiring keeps what AniList says of the watchlog's shows that still
// air up to date, while the watchlog is on: their next episode and when it
// airs, and their length once it is known.
func (h *Hub) refreshAiring(ctx context.Context) {
	for {
		wait := airingEvery
		if h.watchlog.Load() && h.AniList != nil {
			if next := h.updateAiring(ctx); !next.IsZero() {
				wait = min(wait, max(time.Minute, time.Until(next.Add(airingLate))))
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// updateAiring asks AniList about the shows that still air, and answers
// when the next of their episodes airs; zero if none is announced or
// AniList did not answer.
func (h *Hub) updateAiring(ctx context.Context) (next time.Time) {
	ids, err := h.Watchlog.AiringAnime(ctx)
	if err != nil {
		h.log.Warn("watchlog airing", "err", err)
		return time.Time{}
	}
	for len(ids) > 0 {
		batch := ids[:min(len(ids), anilist.MaxIDs)]
		ids = ids[len(batch):]
		actx, cancel := context.WithTimeout(diag.WithCaller(ctx, "airing", ""), 20*time.Second)
		found, err := h.AniList.GetMany(actx, batch)
		cancel()
		if err != nil {
			h.log.Warn("anilist airing", "err", err)
			return time.Time{}
		}
		for _, m := range found {
			a := m.Anime()
			if err := h.Watchlog.SaveAnime(ctx, a); err != nil {
				h.log.Warn("watchlog airing", "err", err)
				return time.Time{}
			}
			if a.NextAiringAt == nil {
				continue
			}
			// One AniList has not moved past yet is not waited for again.
			if at := time.Unix(*a.NextAiringAt, 0); at.After(time.Now()) && (next.IsZero() || at.Before(next)) {
				next = at
			}
		}
	}
	return next
}
