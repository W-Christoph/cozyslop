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
	"sync"
	"time"

	"cozycast/internal/neko"
	"cozycast/internal/store"
)

const chatSweepInterval = time.Minute

type Hub struct {
	store    *store.Store
	mediaDir string       // chat images and videos
	mu       sync.RWMutex // room map and lifetimes
	ctx      context.Context
	rooms    map[string]*Room
	log      *slog.Logger
}

type RoomConfig struct {
	Source        string // configured (default) | registered
	Name          string
	Neko          *neko.Client
	DefaultScreen string
	// Restart restarts the room's container; nil when the operator has not
	// enabled container control.
	Restart func(ctx context.Context) error
	// TitleURL answers with the title of the window in front on the room's
	// desktop (worker/window-title.py); "" if the room has no such helper.
	TitleURL string
	// PlayURL plays a file of the room desktop's Downloads folder on the
	// desktop (worker/play.py), for requests that carry PlayToken; "" if
	// the room has no such helper.
	PlayURL   string
	PlayToken string
}

func New(s *store.Store, mediaDir string, rooms []RoomConfig) *Hub {
	h := &Hub{store: s, mediaDir: mediaDir, rooms: make(map[string]*Room), log: slog.Default()}
	for _, rc := range rooms {
		h.rooms[rc.Name] = h.makeRoom(rc)
	}
	return h
}

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
			h.log.Warn("remove chat media", "file", name, "err", err)
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
	r.playURL, r.playToken = rc.PlayURL, rc.PlayToken
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
