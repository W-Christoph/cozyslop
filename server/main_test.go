package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cozycast/internal/auth"
	"cozycast/internal/httpapi"
	"cozycast/internal/hub"
	"cozycast/internal/neko"
	"cozycast/internal/store"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestDeleteExpired(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "cozycast.db")
	st, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expired, future := int64(0), time.Now().Add(time.Hour).Unix()
	for _, until := range []*int64{&expired, &future, nil} {
		if err := st.AddAnonBan(ctx, "main", "anon", "ip", until); err != nil {
			t.Fatal(err)
		}
	}
	u := &store.User{Username: "alice", Nickname: "alice", PasswordHash: "hash"}
	if err := st.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	for _, token := range [][]byte{[]byte("expired"), []byte("active")} {
		if err := st.CreateSession(ctx, token, u.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, "UPDATE sessions SET expires_at = 0 WHERE token_hash = ?", []byte("expired")); err != nil {
		t.Fatal(err)
	}
	deleteExpired(ctx, st)
	deleteExpired(ctx, st)
	for table, want := range map[string]int{"anon_bans": 2, "sessions": 1} {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != want {
			t.Fatalf("%s: count=%d want=%d err=%v", table, count, want, err)
		}
	}
}

func TestEnsureAdminPasswordValidation(t *testing.T) {
	for _, password := range []string{"short", strings.Repeat("a", 73), strings.Repeat("é", 37), strings.Repeat("é", 36)} {
		t.Run(password, func(t *testing.T) {
			ctx := context.Background()
			st, err := store.Open(ctx, ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			err = ensureAdmin(ctx, st, password)
			if validation := auth.ValidatePassword(password); validation != nil {
				if err == nil || err.Error() != validation.Error() {
					t.Fatalf("initial admin: %v want=%v", err, validation)
				}
				if _, err := st.UserByUsername(ctx, "admin"); !errors.Is(err, store.ErrNotFound) {
					t.Fatalf("invalid password created admin: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			u, err := st.UserByUsername(ctx, "admin")
			if err != nil || !u.Admin || !auth.CheckPassword(u, password) {
				t.Fatalf("created admin: %+v %v", u, err)
			}
			if err := ensureAdmin(ctx, st, "short"); err != nil {
				t.Fatalf("existing admin should be left alone: %v", err)
			}
		})
	}
}

func TestShutdownWaitsForRoomHandlers(t *testing.T) {
	for _, bounded := range []bool{false, true} {
		name := "drain"
		if bounded {
			name = "deadline"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			st, err := store.Open(ctx, ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			deleting, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			desktop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					close(deleting)
					<-release
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			defer func() { unblock(); desktop.Close() }()
			nc, err := neko.NewClient(desktop.URL, "x")
			if err != nil {
				t.Fatal(err)
			}
			h := hub.New(st, t.TempDir(), []hub.RoomConfig{{Name: "main", Neko: nc}})
			api := httpapi.New(httpapi.Deps{Store: st, Auth: auth.New(st, false), Hub: h})
			srv := httptest.NewServer(api.Handler())
			defer srv.Close()
			conn, _, err := websocket.Dial(ctx, srv.URL+"/api/rooms/main/ws", nil)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.CloseNow()
			var welcome struct{ Type string }
			if err := wsjson.Read(ctx, conn, &welcome); err != nil || welcome.Type != "welcome" {
				t.Fatalf("join: %+v %v", welcome, err)
			}
			finished := make(chan error, 1)
			shutdownCtx := ctx
			if bounded {
				var stop context.CancelFunc
				shutdownCtx, stop = context.WithTimeout(ctx, time.Second)
				defer stop()
			}
			go func() { finished <- shutdown(shutdownCtx, []*http.Server{srv.Config}, api) }()
			select {
			case <-deleting:
			case <-ctx.Done():
				t.Fatal("room handler did not leave")
			}
			if _, _, err := conn.Read(ctx); err == nil {
				t.Fatal("room socket still open during Leave")
			}
			select {
			case err := <-finished:
				t.Fatalf("shutdown returned before Leave finished: %v", err)
			default:
			}
			if _, err := st.Settings(ctx); err != nil {
				t.Fatalf("store unavailable during Leave: %v", err)
			}
			client := &http.Client{Timeout: time.Second}
			if res, err := client.Get(srv.URL + "/api/settings"); err == nil {
				res.Body.Close()
				t.Fatal("listener still accepts requests during socket shutdown")
			}
			if !bounded {
				unblock()
			}
			select {
			case err := <-finished:
				if bounded && !errors.Is(err, context.DeadlineExceeded) || !bounded && err != nil {
					t.Fatalf("shutdown: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("shutdown ignored its deadline")
			}
			unblock()
			waitCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			if err := api.Shutdown(waitCtx); err != nil {
				t.Fatal(err)
			}
			if h.Room("main").UserCount() != 0 {
				t.Fatal("shutdown retained room members")
			}
		})
	}
}
