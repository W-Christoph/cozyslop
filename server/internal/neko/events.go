package neko

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

// ObserverID is the neko member CozyCast uses to follow room events. neko's
// API-token session cannot open a WebSocket, so the server keeps one admin
// member that only listens.
const ObserverID = "cozycast-observer"

// RelayID is the neko member the media relay watches with, for all of a
// room's viewers (internal/relay).
const RelayID = "cozycast-relay"

// ResetControl takes the remote away from whoever holds it.
func (c *Client) ResetControl(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/api/room/control/reset", nil, nil, true)
}

// SetImplicitHosting controls whether anyone allowed to host can take an
// occupied remote (true) or must wait for it to be released (false).
func (c *Client) SetImplicitHosting(ctx context.Context, implicit bool) error {
	return c.do(ctx, http.MethodPost, "/api/room/settings", map[string]bool{"implicit_hosting": implicit}, nil, true)
}

// The observer's patience with neko: observerTimeout for the WebSocket
// handshake, for system/init after it and for the answer to a ping, sent
// every observerPingEvery. Variables so tests can shorten them.
var (
	observerTimeout   = 10 * time.Second
	observerPingEvery = 20 * time.Second
)

// Init is what neko reports when the event stream (re)connects.
type Init struct {
	Videos []string // capture pipeline ids, the default first
}

// WatchHost calls onHost with the neko session id of the remote holder
// ("" = nobody) whenever it changes, until ctx ends. It reconnects on its own
// and calls onConnect after every successful (re)connect, which is also the
// first sign that neko may have restarted and lost its runtime settings, and
// onDisconnect when a connected stream ends. Retries back off to 10 s at
// most, so a desktop that comes back is noticed soon.
func (c *Client) WatchHost(ctx context.Context, onConnect func(Init), onDisconnect func(), onHost func(hostID string)) {
	log := slog.With("neko", c.base.Host)
	backoff := time.Second
	for ctx.Err() == nil {
		start := time.Now()
		up := false
		err := c.watchHostOnce(ctx, func(init Init) { up = true; onConnect(init) }, onHost)
		if ctx.Err() != nil {
			return
		}
		if up {
			onDisconnect()
		}
		// Kept-alive connections may be as dead as this one (the other
		// end of a tunnel restarted).
		c.closeIdle()
		log.Warn("neko event stream ended, reconnecting", "err", err)
		if time.Since(start) > time.Minute {
			backoff = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		case <-c.reconnect:
			backoff = time.Second
			continue
		}
		backoff = min(backoff*2, 10*time.Second)
	}
}

func (c *Client) closeIdle() {
	if c.own != nil {
		c.own.CloseIdleConnections()
	}
}

// Reconnect drops the observer's connection and connects again at once:
// for when neko is known to be back but the old connection may not have
// noticed yet that it is dead (a paired room's computer restarted).
// Requests in flight end too, and kept-alive connections are dropped.
func (c *Client) Reconnect() {
	select {
	case c.reconnect <- struct{}{}:
	default:
	}
	c.mu.Lock()
	drop, cancelGen := c.dropWatch, c.cancelGen
	c.gen, c.cancelGen = context.WithCancel(context.Background())
	c.mu.Unlock()
	cancelGen()
	c.closeIdle()
	if drop != nil {
		drop()
	}
}

func (c *Client) watchHostOnce(ctx context.Context, onConnect func(Init), onHost func(string)) error {
	defer c.connected.Store(false)
	token, err := c.observerToken(ctx)
	if err != nil {
		return err
	}

	dialCtx, dialCancel := context.WithTimeout(ctx, observerTimeout)
	conn, _, err := websocket.Dial(dialCtx, c.SocketURL(token), &websocket.DialOptions{HTTPClient: c.stream})
	dialCancel()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.dropWatch = cancel
	c.mu.Unlock()
	// A Reconnect from before this connection was meant for this one.
	select {
	case <-c.reconnect:
	default:
	}
	pingDone := make(chan struct{})
	go func() {
		defer close(pingDone)
		defer cancel()
		ping := time.NewTicker(observerPingEvery)
		defer ping.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ping.C:
				pingCtx, pingCancel := context.WithTimeout(ctx, observerTimeout)
				err := conn.Ping(pingCtx)
				pingCancel()
				if err != nil {
					return
				}
			}
		}
	}()
	defer func() {
		cancel()
		conn.CloseNow()
		<-pingDone
	}()
	conn.SetReadLimit(1 << 20)
	initCtx, initCancel := context.WithTimeout(ctx, observerTimeout)
	defer initCancel()
	readCtx := initCtx

	type controlHost struct {
		HasHost bool   `json:"has_host"`
		HostID  string `json:"host_id"`
	}
	for {
		_, data, err := conn.Read(readCtx)
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
				WebRTC      struct {
					Videos []string `json:"videos"`
				} `json:"webrtc"`
			}
			if json.Unmarshal(msg.Payload, &init) != nil {
				continue
			}
			initCancel()
			readCtx = ctx
			c.connected.Store(true)
			onConnect(Init{Videos: init.WebRTC.Videos})
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
