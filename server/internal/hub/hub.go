// Package hub runs the rooms: who is connected, their rights, chat, presence
// and the remote. Every browser tab is a Client; all tabs of one person form
// a member. Each Client has its own neko member, so neko enforces the
// member's rights on the desktop.
package hub

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"cozycast/internal/neko"
	"cozycast/internal/store"
)

const chatSweepInterval = time.Minute

type Hub struct {
	store    *store.Store
	mediaDir string // chat images and videos
	rooms    map[string]*Room
	log      *slog.Logger
}

type RoomConfig struct {
	Name string
	Neko *neko.Client
}

func New(s *store.Store, mediaDir string, rooms []RoomConfig) *Hub {
	h := &Hub{store: s, mediaDir: mediaDir, rooms: make(map[string]*Room), log: slog.Default()}
	for _, rc := range rooms {
		h.rooms[rc.Name] = newRoom(h, rc.Name, rc.Neko)
	}
	return h
}

// Room returns the named room, or nil.
func (h *Hub) Room(name string) *Room { return h.rooms[name] }

// Rooms returns all rooms sorted by name.
func (h *Hub) Rooms() []*Room {
	list := make([]*Room, 0, len(h.rooms))
	for _, r := range h.rooms {
		list = append(list, r)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list
}

// Start runs background work for every room until ctx ends.
func (h *Hub) Start(ctx context.Context) {
	for _, r := range h.rooms {
		go r.run(ctx)
	}
	go h.sweepChat(ctx)
}

// UserChanged re-reads an account (profile, flags) and applies it to its
// live connections in every room: rights are recomputed, members who may no
// longer be there are disconnected.
func (h *Hub) UserChanged(ctx context.Context, userID int64) {
	for _, r := range h.rooms {
		r.refreshUser(ctx, userID)
	}
}

// PermissionsChanged applies a changed room permission to live connections.
func (h *Hub) PermissionsChanged(ctx context.Context, room string, userID int64) {
	if r := h.rooms[room]; r != nil {
		r.refreshUser(ctx, userID)
	}
}

// RoomSettingsChanged reloads a room's settings and re-checks everyone in it.
func (h *Hub) RoomSettingsChanged(ctx context.Context, room string) {
	if r := h.rooms[room]; r != nil {
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
