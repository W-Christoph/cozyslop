// Package room tracks who is connected to a room and keeps a matching neko
// member for every connection, so neko enforces CozyCast's permissions.
package room

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"cozycast/internal/neko"
)

// Permissions are what a connection may do inside the remote desktop.
type Permissions struct {
	Remote bool `json:"remote"` // mouse and keyboard
	Upload bool `json:"upload"` // drop files onto the desktop
}

// Client is one browser tab. Its ID doubles as the neko member ID, so a user
// with two tabs open gets two independent neko sessions.
type Client struct {
	ID   string
	Name string

	// Send delivers a message to the browser. It must not block.
	Send func(msg any)

	mu           sync.Mutex
	perms        Permissions
	nekoPassword string
}

func (c *Client) Permissions() Permissions {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.perms
}

type Room struct {
	Name     string
	NekoPath string // public path the browser uses to reach this room's neko

	neko     *neko.Client
	defaults Permissions
	log      *slog.Logger

	ready chan struct{} // closed once stale neko members are cleared

	mu      sync.Mutex
	clients map[string]*Client
}

func New(name string, nc *neko.Client, defaults Permissions) *Room {
	return &Room{
		Name:     name,
		NekoPath: "/neko/" + name,
		neko:     nc,
		defaults: defaults,
		log:      slog.With("room", name),
		ready:    make(chan struct{}),
		clients:  make(map[string]*Client),
	}
}

func (r *Room) Neko() *neko.Client { return r.neko }

// Start waits for the room's neko to come up and removes members left over
// from a previous server run. CozyCast owns every member in its neko
// instances, so all of them are stale. Tokens are only issued afterwards.
func (r *Room) Start(ctx context.Context) {
	for {
		if r.neko.Healthy(ctx) {
			err := r.removeAllMembers(ctx)
			if err == nil {
				close(r.ready)
				r.log.Info("neko ready")
				return
			}
			r.log.Warn("clear stale neko members", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func (r *Room) removeAllMembers(ctx context.Context) error {
	members, err := r.neko.ListMembers(ctx)
	if err != nil {
		return err
	}
	for _, m := range members {
		if err := r.neko.DeleteMember(ctx, m.ID); err != nil && !errors.Is(err, neko.ErrNotFound) {
			return err
		}
	}
	if len(members) > 0 {
		r.log.Info("removed stale neko members", "count", len(members))
	}
	return nil
}

func (r *Room) Join(name string, send func(any)) *Client {
	c := &Client{
		ID:    "c-" + randomToken(9),
		Name:  name,
		Send:  send,
		perms: r.defaults,
	}
	r.mu.Lock()
	r.clients[c.ID] = c
	r.mu.Unlock()
	r.log.Info("client joined", "client", c.ID, "name", name)
	return c
}

// Leave forgets the client and destroys its neko member, which also closes
// its neko session.
func (r *Room) Leave(ctx context.Context, c *Client) {
	r.mu.Lock()
	delete(r.clients, c.ID)
	r.mu.Unlock()

	if err := r.neko.DeleteMember(ctx, c.ID); err != nil && !errors.Is(err, neko.ErrNotFound) {
		r.log.Warn("delete neko member", "client", c.ID, "err", err)
	}
	r.log.Info("client left", "client", c.ID)
}

// NekoToken returns a fresh neko session token for the client, creating its
// neko member first if it does not exist (first call, or neko restarted).
func (r *Room) NekoToken(ctx context.Context, c *Client) (string, error) {
	select {
	case <-r.ready:
	case <-ctx.Done():
		return "", ctx.Err()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.nekoPassword != "" {
		token, err := r.neko.Login(ctx, c.ID, c.nekoPassword)
		if err == nil {
			return token, nil
		}
		r.log.Info("neko login failed, recreating member", "client", c.ID, "err", err)
		_ = r.neko.DeleteMember(ctx, c.ID)
	}

	password := randomToken(24)
	if err := r.neko.CreateMember(ctx, c.ID, password, r.profile(c.Name, c.perms)); err != nil {
		return "", fmt.Errorf("create neko member: %w", err)
	}
	c.nekoPassword = password
	return r.neko.Login(ctx, c.ID, password)
}

// SetPermissions changes a client's permissions; neko applies them to the
// live session immediately.
func (r *Room) SetPermissions(ctx context.Context, c *Client, p Permissions) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.perms = p
	if c.nekoPassword == "" {
		return nil // no member yet; created with the new permissions later
	}
	return r.neko.UpdateProfile(ctx, c.ID, r.profile(c.Name, p))
}

func (r *Room) profile(name string, p Permissions) neko.Profile {
	return neko.Profile{
		Name:               name,
		CanLogin:           true,
		CanConnect:         true,
		CanWatch:           true,
		CanHost:            p.Remote,
		CanAccessClipboard: p.Remote,
		Plugins: map[string]any{
			"filetransfer.enabled": p.Upload,
			"chat.can_send":        false,
			"chat.can_receive":     false,
		},
	}
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
