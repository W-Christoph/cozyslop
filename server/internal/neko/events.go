package neko

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/coder/websocket"
)

// ObserverID is the neko member CozyCast uses to follow room events. neko's
// API-token session cannot open a WebSocket, so the server keeps one admin
// member that only listens.
const ObserverID = "cozycast-observer"

// ResetControl takes the remote away from whoever holds it.
func (c *Client) ResetControl(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/api/room/control/reset", nil, nil, true)
}

// SetImplicitHosting controls whether anyone allowed to host can take an
// occupied remote (true) or must wait for it to be released (false).
func (c *Client) SetImplicitHosting(ctx context.Context, implicit bool) error {
	return c.do(ctx, http.MethodPost, "/api/room/settings", map[string]bool{"implicit_hosting": implicit}, nil, true)
}

// WatchHost calls onHost with the neko session id of the remote holder
// ("" = nobody) whenever it changes, until ctx ends. It reconnects on its own.
func (c *Client) WatchHost(ctx context.Context, onHost func(hostID string)) {
	log := slog.With("neko", c.base.Host)
	backoff := time.Second
	for ctx.Err() == nil {
		start := time.Now()
		err := c.watchHostOnce(ctx, onHost)
		if ctx.Err() != nil {
			return
		}
		log.Warn("neko event stream ended, reconnecting", "err", err)
		if time.Since(start) > time.Minute {
			backoff = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

func (c *Client) watchHostOnce(ctx context.Context, onHost func(string)) error {
	token, err := c.observerToken(ctx)
	if err != nil {
		return err
	}

	u := *c.base
	u.Scheme = map[string]string{"https": "wss"}[u.Scheme]
	if u.Scheme == "" {
		u.Scheme = "ws"
	}
	u.Path = u.JoinPath("/api/ws").Path
	u.RawQuery = url.Values{"token": {token}}.Encode()

	conn, _, err := websocket.Dial(ctx, u.String(), nil)
	if err != nil {
		return err
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1 << 20)

	type controlHost struct {
		HasHost bool   `json:"has_host"`
		HostID  string `json:"host_id"`
	}
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		var msg struct {
			Event   string          `json:"event"`
			Payload json.RawMessage `json:"payload"`
		}
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		var host controlHost
		switch msg.Event {
		case "system/init":
			var init struct {
				ControlHost controlHost `json:"control_host"`
			}
			if json.Unmarshal(msg.Payload, &init) != nil {
				continue
			}
			host = init.ControlHost
		case "control/host":
			if json.Unmarshal(msg.Payload, &host) != nil {
				continue
			}
		default:
			continue
		}
		if !host.HasHost {
			host.HostID = ""
		}
		onHost(host.HostID)
	}
}

// observerToken (re)creates the observer member and logs it in.
func (c *Client) observerToken(ctx context.Context) (string, error) {
	password := randomPassword()
	_ = c.DeleteMember(ctx, ObserverID)
	err := c.CreateMember(ctx, ObserverID, password, Profile{
		Name:       "CozyCast",
		IsAdmin:    true, // receives every room event
		CanLogin:   true,
		CanConnect: true,
	})
	if err != nil {
		return "", err
	}
	return c.Login(ctx, ObserverID, password)
}
