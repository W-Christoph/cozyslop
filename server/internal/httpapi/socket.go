package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"cozycast/internal/room"
)

const (
	sendBuffer   = 64
	writeTimeout = 10 * time.Second
	pingInterval = 20 * time.Second
	maxNameRunes = 32
)

// Messages from server to browser.
type welcomeMsg struct {
	Type        string           `json:"type"` // "welcome"
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Permissions room.Permissions `json:"permissions"`
}

type nekoMsg struct {
	Type  string `json:"type"` // "neko"
	Token string `json:"token"`
	Path  string `json:"path"`
}

type errorMsg struct {
	Type    string `json:"type"` // "error"
	Message string `json:"message"`
}

// Messages from browser to server.
type clientMsg struct {
	Type string `json:"type"`
}

func (s *Server) roomSocket(w http.ResponseWriter, r *http.Request) {
	rm := s.room(r)
	if rm == nil {
		http.NotFound(w, r)
		return
	}

	conn, err := websocket.Accept(w, r, nil) // same-origin only
	if err != nil {
		return
	}
	conn.SetReadLimit(64 << 10)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	out := make(chan any, sendBuffer)
	send := func(msg any) {
		select {
		case out <- msg:
		default:
			// A client that cannot keep up is dropped rather than
			// allowed to stall the room.
			cancel()
		}
	}
	go writeLoop(ctx, cancel, conn, out)

	client := rm.Join(displayName(r.URL.Query().Get("name")), send)
	defer rm.Leave(context.WithoutCancel(ctx), client)

	send(welcomeMsg{Type: "welcome", ID: client.ID, Name: client.Name, Permissions: client.Permissions()})
	s.sendNekoToken(ctx, rm, client)

	for {
		var msg clientMsg
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			if !errors.Is(err, context.Canceled) && websocket.CloseStatus(err) == -1 {
				s.log.Debug("room socket read", "client", client.ID, "err", err)
			}
			conn.Close(websocket.StatusNormalClosure, "")
			return
		}
		switch msg.Type {
		case "neko/token":
			// Sent by the browser when its neko connection was rejected,
			// e.g. after the neko container restarted.
			s.sendNekoToken(ctx, rm, client)
		}
	}
}

func (s *Server) sendNekoToken(ctx context.Context, rm *room.Room, c *room.Client) {
	token, err := rm.NekoToken(ctx, c)
	if err != nil {
		s.log.Error("issue neko token", "room", rm.Name, "client", c.ID, "err", err)
		c.Send(errorMsg{Type: "error", Message: "The room's desktop is not reachable right now."})
		return
	}
	c.Send(nekoMsg{Type: "neko", Token: token, Path: rm.NekoPath})
}

func writeLoop(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, out <-chan any) {
	defer cancel()
	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-out:
			wctx, wcancel := context.WithTimeout(ctx, writeTimeout)
			err := wsjson.Write(wctx, conn, msg)
			wcancel()
			if err != nil {
				return
			}
		case <-ping.C:
			pctx, pcancel := context.WithTimeout(ctx, writeTimeout)
			err := conn.Ping(pctx)
			pcancel()
			if err != nil {
				return
			}
		}
	}
}

func displayName(raw string) string {
	name := strings.TrimSpace(raw)
	if utf8.RuneCountInString(name) > maxNameRunes {
		name = string([]rune(name)[:maxNameRunes])
	}
	if name == "" {
		name = "Anonymous"
	}
	return name
}
