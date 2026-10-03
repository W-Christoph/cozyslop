package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// socketPair runs fill against a server-side socket before its write loop
// starts, and returns the browser's end.
func socketPair(t *testing.T, fill func(*socket)) (*websocket.Conn, context.Context) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		sock := newSocket(conn)
		fill(sock)
		ctx, cancel := context.WithCancel(r.Context())
		go sock.writeLoop(ctx, cancel)
		conn.Read(ctx) // the browser's close ends the handler
	}))
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	return conn, ctx
}

func TestSocketKillSendsTheReasonThenCloses(t *testing.T) {
	conn, ctx := socketPair(t, func(s *socket) {
		s.send("reason")
		s.kill()
		s.send("too late")
	})
	var got string
	if err := wsjson.Read(ctx, conn, &got); err != nil || got != "reason" {
		t.Fatalf("read %q, %v", got, err)
	}
	err := wsjson.Read(ctx, conn, &got)
	if websocket.CloseStatus(err) != statusKicked {
		t.Fatalf("read %q, %v; want close %d", got, err, statusKicked)
	}
}

func TestSocketOverflowClosesForReconnect(t *testing.T) {
	conn, ctx := socketPair(t, func(s *socket) {
		for i := 0; i <= sendBuffer; i++ {
			s.send(i)
		}
	})
	for {
		var got int
		err := wsjson.Read(ctx, conn, &got)
		if err == nil {
			continue
		}
		if websocket.CloseStatus(err) != statusOverflow || statusOverflow == statusKicked {
			t.Fatalf("closed with %v; want %d", err, statusOverflow)
		}
		return
	}
}
