package neko_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"cozycast/internal/neko"
	"cozycast/internal/neko/nekotest"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestWatchHostLiveness(t *testing.T) {
	neko.SetObserverTiming(t, 300*time.Millisecond, 500*time.Millisecond)
	for _, mode := range []string{"handshake stalls", "init missing", "pong missing", "quiet healthy"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			fake := nekotest.New(t, "secret")
			upstream, err := url.Parse(fake.URL())
			if err != nil {
				t.Fatal(err)
			}
			proxy := httputil.NewSingleHostReverseProxy(upstream)
			var sockets atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/ws" {
					proxy.ServeHTTP(w, r)
					return
				}
				first := sockets.Add(1) == 1
				if first && mode == "handshake stalls" {
					select {
					case <-r.Context().Done():
					case <-ctx.Done():
					}
					return
				}
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer conn.CloseNow()
				if first && mode == "init missing" {
					// Other traffic must not satisfy or extend the init deadline.
					_ = wsjson.Write(ctx, conn, map[string]any{"event": "system/init", "payload": "invalid"})
					_ = wsjson.Write(ctx, conn, map[string]any{"event": "control/host", "payload": map[string]any{"has_host": false}})
				} else {
					_ = wsjson.Write(ctx, conn, map[string]any{"event": "system/init", "payload": map[string]any{
						"control_host": map[string]any{"has_host": true, "host_id": "holder"},
						"webrtc":       map[string]any{"videos": nekotest.Streams},
					}})
				}
				if first && mode == "pong missing" {
					// Without a reader the peer never responds to pings.
					<-ctx.Done()
					return
				}
				<-conn.CloseRead(ctx).Done()
			}))
			t.Cleanup(srv.Close)
			client, err := neko.NewClient(srv.URL, "secret", nil)
			if err != nil {
				t.Fatal(err)
			}
			connected := make(chan neko.Init, 4)
			hosts := make(chan string, 4)
			done := make(chan struct{})
			go func() {
				defer close(done)
				client.WatchHost(ctx, func(init neko.Init) { connected <- init }, func() {}, func(host string) { hosts <- host })
			}()
			t.Cleanup(func() {
				cancel()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Error("observer did not stop after cancellation")
				}
			})
			waitConnect := func() {
				t.Helper()
				select {
				case init := <-connected:
					if len(init.Videos) != len(nekotest.Streams) {
						t.Fatalf("init videos: %v", init.Videos)
					}
				case <-ctx.Done():
					t.Fatal("observer did not connect or reconnect")
				}
			}
			waitConnect()
			if mode == "pong missing" {
				waitConnect()
			}
			if mode == "quiet healthy" {
				select {
				case <-connected:
					t.Fatal("healthy quiet connection reconnected")
				case <-done:
					t.Fatal("healthy quiet connection ended")
				case <-time.After(2 * time.Second): // several pings
				}
				if sockets.Load() != 1 {
					t.Fatalf("healthy connection dialed %d times", sockets.Load())
				}
			} else if sockets.Load() < 2 {
				t.Fatalf("failed connection did not reconnect: %d sockets", sockets.Load())
			}
			select {
			case host := <-hosts:
				// The init-missing peer first reports an empty host.
				if mode != "init missing" && host != "holder" {
					t.Fatalf("host = %q", host)
				}
			case <-ctx.Done():
				t.Fatal("host callback missing")
			}
		})
	}
}
