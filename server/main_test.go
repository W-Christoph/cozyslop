package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cozycast/internal/auth"
	"cozycast/internal/config"
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

func TestResetAdminCommand(t *testing.T) {
	for _, existing := range []bool{true, false} {
		name := "create"
		if existing {
			name = "reset"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			t.Setenv("COZYCAST_DATA_DIR", dir)
			t.Setenv("COZYCAST_NEKO_SECRET", "test secret")
			t.Setenv("COZYCAST_INIT_ADMIN_PASSWORD", "new password")
			t.Setenv("COZYCAST_LISTEN", "invalid listen address")
			t.Setenv("COZYCAST_ROOMS", "main=invalid-neko-url")
			t.Setenv("COZYCAST_DOCKER", "true")
			archive := filepath.Join(dir, "invalid-import")
			if err := os.WriteFile(archive, []byte("not an archive"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("COZYCAST_IMPORT", archive)
			// Keep another connection open, as when the server is running.
			st, err := store.Open(ctx, filepath.Join(dir, "cozycast.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			var old *store.User
			if existing {
				hash, err := auth.HashPassword("old password")
				if err != nil {
					t.Fatal(err)
				}
				old = &store.User{Username: "admin", PasswordHash: hash, Nickname: "Custom", NameColor: "#f90", Avatar: "avatar.png", Disabled: true}
				if err := st.CreateUser(ctx, old); err != nil {
					t.Fatal(err)
				}
				for _, token := range []string{"first", "second"} {
					if err := st.CreateSession(ctx, []byte(token), old.ID); err != nil {
						t.Fatal(err)
					}
				}
			}
			other := &store.User{Username: "alice", PasswordHash: "hash", Nickname: "Alice"}
			if err := st.CreateUser(ctx, other); err != nil {
				t.Fatal(err)
			}
			if err := st.CreateSession(ctx, []byte("other"), other.ID); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err := run([]string{"reset-admin"}, &out); err != nil {
				t.Fatal(err)
			}
			u, err := st.UserByUsername(ctx, "admin")
			if err != nil || !u.Admin || u.Disabled || !auth.CheckPassword(u, "new password") || auth.CheckPassword(u, "old password") {
				t.Fatalf("restored admin: %+v %v", u, err)
			}
			if existing {
				if u.ID != old.ID || u.Nickname != old.Nickname || u.NameColor != old.NameColor || u.Avatar != old.Avatar || u.Verified != old.Verified || u.CreatedAt != old.CreatedAt {
					t.Fatalf("reset changed unrelated account data: %+v want %+v", u, old)
				}
				for _, token := range []string{"first", "second"} {
					if _, extended, err := st.SessionUser(ctx, []byte(token)); !errors.Is(err, store.ErrNotFound) || extended {
						t.Fatalf("admin session retained: %v extended=%v", err, extended)
					}
				}
			} else if u.Nickname != "admin" || !u.Verified {
				t.Fatalf("created admin defaults: %+v", u)
			}
			if got, _, err := st.SessionUser(ctx, []byte("other")); err != nil || got.ID != other.ID {
				t.Fatalf("other user's session changed: %+v %v", got, err)
			}
			want := "Created admin account; enabled admin rights and cleared sessions.\n"
			if existing {
				want = "Reset admin password; enabled admin rights and cleared sessions.\n"
			}
			if out.String() != want {
				t.Fatalf("output: %q want %q", out.String(), want)
			}
			if _, err := os.Stat(filepath.Join(dir, "media")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("reset initialized server media directories: %v", err)
			}
			db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "cozycast.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var count int
			if err := db.QueryRow("SELECT count(*) FROM rooms").Scan(&count); err != nil || count != 0 {
				t.Fatalf("reset initialized rooms: count=%d %v", count, err)
			}
		})
	}
}

func TestResetAdminRejectsInvalidPassword(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	t.Setenv("COZYCAST_DATA_DIR", dir)
	t.Setenv("COZYCAST_NEKO_SECRET", "test secret")
	st, err := store.Open(ctx, filepath.Join(dir, "cozycast.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	hash, err := auth.HashPassword("old password")
	if err != nil {
		t.Fatal(err)
	}
	u := &store.User{Username: "admin", PasswordHash: hash, Nickname: "Custom", Disabled: true}
	if err := st.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateSession(ctx, []byte("session"), u.ID); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "cozycast.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, password := range []string{"", "short", strings.Repeat("a", 73), strings.Repeat("é", 37)} {
		t.Setenv("COZYCAST_INIT_ADMIN_PASSWORD", password)
		var out bytes.Buffer
		if err := run([]string{"reset-admin"}, &out); err == nil || !strings.Contains(err.Error(), "COZYCAST_INIT_ADMIN_PASSWORD") {
			t.Fatalf("invalid password accepted: %v", err)
		}
		got, err := st.UserByUsername(ctx, "admin")
		if err != nil || *got != *u || !auth.CheckPassword(got, "old password") {
			t.Fatalf("invalid password changed admin: %+v %v", got, err)
		}
		var count int
		if err := db.QueryRow("SELECT count(*) FROM sessions WHERE user_id = ?", u.ID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("invalid password revoked sessions: %d %v", count, err)
		}
		if out.Len() != 0 {
			t.Fatalf("invalid password printed success: %q", out.String())
		}
	}
	if err := st.DeleteUser(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COZYCAST_INIT_ADMIN_PASSWORD", "")
	if err := run([]string{"reset-admin"}, &bytes.Buffer{}); err == nil {
		t.Fatal("empty password accepted for missing admin")
	}
	if _, err := st.UserByUsername(ctx, "admin"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("invalid password created admin: %v", err)
	}
}

func TestUnknownSubcommand(t *testing.T) {
	for _, args := range [][]string{{"unknown"}, {"reset-admin", "extra"}} {
		var out bytes.Buffer
		err := run(args, &out)
		if err == nil || !strings.Contains(err.Error(), "Usage: cozycast [reset-admin]") || out.Len() != 0 {
			t.Fatalf("args %v: err=%v output=%q", args, err, out.String())
		}
		if args[0] == "unknown" && !strings.Contains(err.Error(), "unknown subcommand") {
			t.Fatalf("unknown subcommand not identified: %v", err)
		}
	}
}

func TestLoadConfiguredAndRegisteredRooms(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for _, room := range []store.RegisteredRoom{
		{Name: "default", NekoURL: "http://shadowed:8080", NekoToken: "shadow-token"},
		{Name: "registered", NekoURL: "https://remote:8443/neko", NekoToken: "random-token"},
	} {
		if err := st.CreateRegisteredRoom(ctx, &room); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Config{NekoSecret: "master-secret", DefaultScreen: "1280x720@30", Rooms: []config.Room{{Name: "default", NekoURL: "http://configured:8080"}}}
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(previous)
	rooms, err := loadRooms(ctx, st, cfg, func(name, nekoURL, token string) (hub.RoomConfig, error) {
		return hub.BuildRoomConfig(name, nekoURL, token, cfg.DefaultScreen)
	})
	if err != nil || len(rooms) != 2 {
		t.Fatalf("rooms: %+v %v", rooms, err)
	}
	if rooms[0].Name != "default" || rooms[0].Source != "configured" || rooms[0].Neko.BaseURL().Host != "configured:8080" || rooms[0].PlayToken != cfg.NekoToken("default") {
		t.Fatal("configured room lost precedence")
	}
	if rooms[1].Name != "registered" || rooms[1].Source != "registered" || rooms[1].PlayToken != "random-token" || rooms[1].PlayURL != "http://remote:8082/play" || rooms[1].TitleURL != "http://remote:8081/title" || rooms[1].DefaultScreen != cfg.DefaultScreen || rooms[1].Restart != nil {
		t.Fatal("registered room configuration differs")
	}
	if !strings.Contains(logs.String(), "configured room overrides registered room") || strings.Contains(logs.String(), "shadow-token") || strings.Contains(logs.String(), "random-token") {
		t.Fatalf("warning: %s", logs.String())
	}
	if _, err := st.RegisteredRoom(ctx, "default"); err != nil {
		t.Fatal("shadowed registration was deleted")
	}
}
