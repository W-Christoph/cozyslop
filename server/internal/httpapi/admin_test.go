package httpapi_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"testing"
	"time"

	"cozycast/internal/store"

	"github.com/coder/websocket/wsjson"
)

func TestAdminAuthorization(t *testing.T) {
	a := newAPITest(t)
	a.user("alice", false)
	anon, user := a.client(), a.login("alice")
	for _, route := range []struct{ method, path string }{
		{"GET", "/api/admin/users"},
		{"PATCH", "/api/admin/users/alice"},
		{"DELETE", "/api/admin/users/alice"},
		{"POST", "/api/admin/users/alice/password"},
		{"GET", "/api/admin/permissions"},
		{"PUT", "/api/admin/permissions/default/alice"},
		{"DELETE", "/api/admin/permissions/default/alice"},
		{"GET", "/api/admin/rooms/default/settings"},
		{"PUT", "/api/admin/rooms/default/settings"},
		{"POST", "/api/admin/rooms/default/bans"},
		{"GET", "/api/admin/bans"},
		{"DELETE", "/api/admin/bans/1"},
		{"PUT", "/api/admin/settings"},
		{"POST", "/api/admin/invites"},
		{"GET", "/api/admin/invites"},
		{"DELETE", "/api/admin/invites/missing"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			a.error(anon, route.method, route.path, map[string]any{}, 401, "Please log in.")
			a.error(user, route.method, route.path, map[string]any{}, 403, "Admins only.")
		})
	}
}

func TestAdminUsers(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()
	a.user("root", true)
	alice := a.user("alice", false)
	a.user("second", true)
	if err := a.st.UpdateAvatar(ctx, alice.ID, "alice.png"); err != nil {
		t.Fatal(err)
	}
	admin, user, other := a.login("root"), a.login("alice"), a.login("alice")
	var list []struct {
		Username  string `json:"username"`
		Nickname  string `json:"nickname"`
		NameColor string `json:"nameColor"`
		AvatarURL string `json:"avatarUrl"`
		Admin     bool   `json:"admin"`
		Verified  bool   `json:"verified"`
		Disabled  bool   `json:"disabled"`
		CreatedAt int64  `json:"createdAt"`
	}
	data := a.call(admin, "GET", "/api/admin/users", nil, 200, &list)
	if len(list) != 3 || list[0].Username != "alice" || list[1].Username != "root" || list[2].Username != "second" || list[0].AvatarURL != "/media/avatars/alice.png" || list[0].CreatedAt == 0 || list[0].Nickname != "alice" || list[0].NameColor != "#fff" || list[0].Admin || list[0].Verified || list[0].Disabled || !list[1].Admin {
		t.Fatalf("user list: %s", data)
	}
	if strings.Contains(string(data), "password") || strings.Contains(string(data), `"id"`) {
		t.Fatalf("private user fields: %s", data)
	}
	for _, req := range []map[string]bool{{"admin": false}, {"disabled": true}} {
		a.error(admin, "PATCH", "/api/admin/users/root", req, 403, "You can't remove your own admin rights or disable yourself.")
	}
	a.call(admin, "PATCH", "/api/admin/users/root", map[string]bool{"verified": true, "admin": true, "disabled": false}, 200, nil)
	a.call(admin, "DELETE", "/api/admin/users/root", nil, 403, nil)
	a.error(admin, "DELETE", "/api/admin/users/second", nil, 409, "Remove admin rights first.")
	var updated store.User
	a.call(admin, "PATCH", "/api/admin/users/alice", map[string]bool{"verified": true}, 200, &updated)
	if !updated.Verified || updated.Admin || updated.Disabled || updated.Username != "alice" {
		t.Fatalf("partial flags: %+v", updated)
	}
	a.call(admin, "PATCH", "/api/admin/users/alice", map[string]bool{"disabled": true}, 200, &updated)
	if !updated.Disabled || !updated.Verified {
		t.Fatalf("disable: %+v", updated)
	}
	// Re-enabling before either client makes a request proves the sessions
	// were deleted, rather than merely blocked while the account was disabled.
	a.call(admin, "PATCH", "/api/admin/users/alice", map[string]bool{"disabled": false}, 200, nil)
	a.me(user, "")
	a.me(other, "")
	a.call(admin, "PATCH", "/api/admin/users/second", map[string]bool{"admin": false}, 200, nil)
	a.call(admin, "DELETE", "/api/admin/users/second", nil, 204, nil)
	a.call(admin, "DELETE", "/api/admin/users/alice", nil, 204, nil)
	for _, route := range []struct{ method, suffix string }{{"PATCH", ""}, {"DELETE", ""}, {"POST", "/password"}} {
		a.error(admin, route.method, "/api/admin/users/missing"+route.suffix, map[string]any{}, 404, "Unknown user.")
	}
}

func TestAdminLastAdminProtection(t *testing.T) {
	for _, flags := range []string{`{"admin":false}`, `{"disabled":true}`} {
		t.Run(flags, func(t *testing.T) {
			a := newAPITest(t)
			a.user("first", true)
			a.user("second", true)
			first, second := a.login("first"), a.login("second")
			reader, writer := io.Pipe()
			defer reader.Close()
			defer writer.Close()
			req, err := http.NewRequest("PATCH", a.srv.URL+"/api/admin/users/second", reader)
			if err != nil {
				t.Fatal(err)
			}
			// The server sends 100 Continue when readJSON first reads the body,
			// after requireAdmin has authenticated first. Hold that body until
			// second demotes first, leaving second as the only enabled admin.
			authenticated := make(chan struct{})
			req.Header.Set("Expect", "100-continue")
			req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
				Got100Continue: func() { close(authenticated) },
			}))
			type result struct {
				response *http.Response
				err      error
			}
			finished := make(chan result, 1)
			go func() {
				res, err := first.Do(req)
				finished <- result{res, err}
			}()
			select {
			case <-authenticated:
			case <-time.After(5 * time.Second):
				t.Fatal("request did not reach body decoding")
			}
			a.call(second, "PATCH", "/api/admin/users/first", map[string]bool{"admin": false}, 200, nil)
			if _, err := io.WriteString(writer, flags); err != nil {
				t.Fatal(err)
			}
			writer.Close()
			got := <-finished
			if got.err != nil {
				t.Fatal(got.err)
			}
			defer got.response.Body.Close()
			body, err := io.ReadAll(got.response.Body)
			if err != nil || got.response.StatusCode != 409 || string(body) != "{\"error\":\"There must be at least one admin.\"}\n" {
				t.Fatalf("last admin: status=%d body=%s err=%v", got.response.StatusCode, body, err)
			}
			n, err := a.st.CountAdmins(context.Background())
			if err != nil || n != 1 {
				t.Fatalf("enabled admins: %d err=%v", n, err)
			}
		})
	}
}

func TestAdminPasswordReset(t *testing.T) {
	a := newAPITest(t)
	a.user("root", true)
	a.user("alice", false)
	admin, user, other := a.login("root"), a.login("alice"), a.login("alice")
	for _, password := range []string{"short", strings.Repeat("a", 73), strings.Repeat("é", 37)} {
		a.error(admin, "POST", "/api/admin/users/alice/password", map[string]string{"password": password}, 400, passwordError)
	}
	a.call(admin, "POST", "/api/admin/users/alice/password", map[string]string{"password": "newpassword"}, 204, nil)
	a.me(user, "")
	a.me(other, "")
	a.call(user, "POST", "/api/auth/login", map[string]string{"username": "alice", "password": testPassword}, 401, nil)
	a.call(user, "POST", "/api/auth/login", map[string]string{"username": "alice", "password": "newpassword"}, 200, nil)
}

func TestAdminPermissions(t *testing.T) {
	a := newAPITest(t)
	a.user("root", true)
	u := a.user("alice", false)
	admin := a.login("root")
	var list []store.Permission
	data := a.call(admin, "GET", "/api/admin/permissions", nil, 200, &list)
	if string(data) != "[]\n" {
		t.Fatalf("empty permissions: %s", data)
	}
	req := map[string]any{"remote": true, "image": true, "upload": true, "trusted": true, "invited": true, "banned": true, "inviteName": strings.Repeat("é", 64), "bannedUntil": int64(2000000000)}
	var p store.Permission
	a.call(admin, "PUT", "/api/admin/permissions/default/alice", req, 200, &p)
	if p.Room != "default" || p.Username != "alice" || !p.Remote || !p.Image || !p.Upload || !p.Trusted || !p.Invited || !p.Banned || p.InviteName != req["inviteName"] || p.BannedUntil == nil || *p.BannedUntil != 2000000000 {
		t.Fatalf("saved permission: %+v", p)
	}
	for _, path := range []string{"/api/admin/permissions", "/api/admin/permissions?room=default"} {
		a.call(admin, "GET", path, nil, 200, &list)
		if len(list) != 1 || list[0].Username != "alice" {
			t.Fatalf("permissions: %+v", list)
		}
	}
	a.call(admin, "GET", "/api/admin/permissions?room=other", nil, 200, &list)
	if len(list) != 0 {
		t.Fatalf("filter: %+v", list)
	}
	a.call(admin, "PUT", "/api/admin/permissions/default/alice", map[string]any{"image": true, "bannedUntil": nil}, 200, &p)
	if p.Remote || p.Upload || p.Trusted || p.Invited || p.Banned || p.BannedUntil != nil || !p.Image {
		t.Fatalf("full replace: %+v", p)
	}
	a.error(admin, "PUT", "/api/admin/permissions/missing/alice", req, 404, "Unknown room.")
	a.error(admin, "PUT", "/api/admin/permissions/default/missing", req, 404, "Unknown user.")
	req["inviteName"] = strings.Repeat("é", 65)
	a.call(admin, "PUT", "/api/admin/permissions/default/alice", req, 400, nil)
	a.call(admin, "DELETE", "/api/admin/permissions/default/alice", nil, 204, nil)
	stored, err := a.st.Permission(context.Background(), "default", u.ID)
	if err != nil || stored.Image {
		t.Fatalf("deleted: %v %+v", err, stored)
	}
	a.call(admin, "DELETE", "/api/admin/permissions/default/alice", nil, 404, nil)
	a.call(admin, "DELETE", "/api/admin/permissions/missing/alice", nil, 404, nil)
	a.call(admin, "DELETE", "/api/admin/permissions/default/missing", nil, 404, nil)
}

func TestAdminRoomSettings(t *testing.T) {
	a := newAPITest(t)
	a.user("root", true)
	admin := a.login("root")
	var set store.RoomSettings
	a.call(admin, "GET", "/api/admin/rooms/default/settings", nil, 200, &set)
	if set.Name != "default" || set.Access != "public" {
		t.Fatalf("defaults: %+v", set)
	}
	req := map[string]any{"access": "invite", "hidden": true, "remoteOwnership": false, "centerRemote": true, "defaultRemote": true, "defaultImage": true, "defaultUpload": true}
	a.call(admin, "PUT", "/api/admin/rooms/default/settings", req, 200, &set)
	if set.Name != "default" || set.Access != "invite" || !set.Hidden || set.RemoteOwnership || !set.CenterRemote || !set.DefaultRemote || !set.DefaultImage || !set.DefaultUpload {
		t.Fatalf("saved: %+v", set)
	}
	var saved store.RoomSettings
	a.call(admin, "GET", "/api/admin/rooms/default/settings", nil, 200, &saved)
	if saved != set {
		t.Fatalf("settings not persisted: %+v %+v", set, saved)
	}
	// A fresh anonymous room listing uses the hub's reloaded settings.
	var rooms []any
	a.call(a.client(), "GET", "/api/rooms", nil, 200, &rooms)
	if len(rooms) != 0 {
		t.Fatalf("hidden invite room: %+v", rooms)
	}
	for _, access := range []string{"", "invalid"} {
		req["access"] = access
		a.call(admin, "PUT", "/api/admin/rooms/default/settings", req, 400, nil)
	}
	// The path decides the room; a name in the body (clients send back the
	// object they received) is ignored.
	req["access"] = "public"
	req["name"] = "other"
	a.call(admin, "PUT", "/api/admin/rooms/default/settings", req, 200, &saved)
	if saved.Name != "default" {
		t.Fatalf("name from body used: %+v", saved)
	}
	delete(req, "name")
	// Stream settings: omitted keeps the stored value, "" (neko's default) is
	// always allowed, and choosing a stream or screen needs the room's
	// desktop to confirm it (the test neko is unreachable: 503).
	req["stream"] = ""
	a.call(admin, "PUT", "/api/admin/rooms/default/settings", req, 200, &saved)
	delete(req, "stream")
	a.call(admin, "PUT", "/api/admin/rooms/default/settings", req, 200, &saved)
	if saved.Stream != "" || saved.Screen != "" {
		t.Fatalf("stream settings not kept: %+v", saved)
	}
	req["stream"] = "b2500-s100-veryfast"
	a.call(admin, "PUT", "/api/admin/rooms/default/settings", req, 503, nil)
	delete(req, "stream")
	req["screen"] = "1920x1080@30"
	a.call(admin, "PUT", "/api/admin/rooms/default/settings", req, 503, nil)
	req["screen"] = "big"
	a.call(admin, "PUT", "/api/admin/rooms/default/settings", req, 400, nil)
	delete(req, "screen")
	for _, method := range []string{"GET", "PUT"} {
		a.error(admin, method, "/api/admin/rooms/missing/settings", req, 404, "Unknown room.")
	}
}

func TestAdminBans(t *testing.T) {
	for _, account := range []bool{false, true} {
		for _, minutes := range []any{nil, 0, 5} {
			t.Run(fmt.Sprintf("account=%v minutes=%v", account, minutes), func(t *testing.T) {
				a := newAPITest(t)
				a.user("root", true)
				admin, user := a.login("root"), a.client()
				var u *store.User
				if account {
					u = a.user("alice", false)
					user = a.login("alice")
				}
				conn, key := a.join(user)
				before := time.Now().Unix()
				a.call(admin, "POST", "/api/admin/rooms/default/bans", map[string]any{"key": key, "minutes": minutes}, 204, nil)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				var kicked struct {
					Type        string `json:"type"`
					Reason      string `json:"reason"`
					BannedUntil *int64 `json:"bannedUntil"`
				}
				if err := wsjson.Read(ctx, conn, &kicked); err != nil {
					t.Fatal(err)
				}
				expectBan := minutes != 0
				reason := "kicked"
				if expectBan {
					reason = "banned"
				}
				if kicked.Type != "kicked" || kicked.Reason != reason {
					t.Fatalf("moderation: %+v", kicked)
				}
				if minutes == 5 && (kicked.BannedUntil == nil || *kicked.BannedUntil < before+300 || *kicked.BannedUntil > time.Now().Unix()+300) {
					t.Fatalf("expiry: %+v", kicked)
				}
				if minutes == nil && kicked.BannedUntil != nil {
					t.Fatalf("forever: %+v", kicked)
				}
				if account {
					p, err := a.st.Permission(ctx, "default", u.ID)
					if err != nil || p.Banned != expectBan {
						t.Fatalf("account ban: %v %+v", err, p)
					}
				} else {
					var bans []store.AnonBan
					a.call(admin, "GET", "/api/admin/bans?room=default", nil, 200, &bans)
					want := 0
					if expectBan {
						want = 1
					}
					if len(bans) != want {
						t.Fatalf("anon bans: %+v", bans)
					}
					if expectBan {
						a.call(admin, "DELETE", fmt.Sprintf("/api/admin/bans/%d", bans[0].ID), nil, 204, nil)
					}
				}
			})
		}
	}
}

func TestAdminBanErrorsAndList(t *testing.T) {
	a := newAPITest(t)
	a.user("root", true)
	admin := a.login("root")
	for _, minutes := range []any{nil, 0, 5} {
		a.error(admin, "POST", "/api/admin/rooms/default/bans", map[string]any{"key": "a:absent", "minutes": minutes}, 404, "That user is not in the room.")
	}
	a.call(admin, "POST", "/api/admin/rooms/default/bans", map[string]any{"key": "u:absent", "minutes": -1}, 400, nil)
	a.error(admin, "POST", "/api/admin/rooms/missing/bans", map[string]any{"key": "absent"}, 404, "Unknown room.")
	ctx := context.Background()
	for _, room := range []string{"default", "other"} {
		if err := a.st.AddAnonBan(ctx, room, "anon", "127.0.0.1", nil); err != nil {
			t.Fatal(err)
		}
	}
	var bans []store.AnonBan
	a.call(admin, "GET", "/api/admin/bans", nil, 200, &bans)
	if len(bans) != 2 {
		t.Fatalf("all bans: %+v", bans)
	}
	a.call(admin, "GET", "/api/admin/bans?room=default", nil, 200, &bans)
	if len(bans) != 1 || bans[0].Room != "default" || bans[0].IP != "127.0.0.1" || bans[0].BannedUntil != nil {
		t.Fatalf("filtered bans: %+v", bans)
	}
	path := fmt.Sprintf("/api/admin/bans/%d", bans[0].ID)
	a.call(admin, "DELETE", path, nil, 204, nil)
	a.call(admin, "DELETE", path, nil, 404, nil)
	a.call(admin, "DELETE", "/api/admin/bans/invalid", nil, 400, nil)
}

func TestAdminGlobalSettings(t *testing.T) {
	a := newAPITest(t)
	a.user("root", true)
	admin := a.login("root")
	var set store.Settings
	message := strings.Repeat("é", 4096)
	a.call(admin, "PUT", "/api/admin/settings", map[string]string{"message": message, "registration": "open"}, 200, &set)
	if set.Message != message || set.Registration != "open" {
		t.Fatalf("saved settings: %+v", set)
	}
	a.call(a.client(), "GET", "/api/settings", nil, 200, &set)
	if set.Message != message || set.Registration != "open" {
		t.Fatalf("persisted settings: %+v", set)
	}
	a.call(admin, "PUT", "/api/admin/settings", map[string]string{"message": message + "é", "registration": "open"}, 400, nil)
	a.call(admin, "PUT", "/api/admin/settings", map[string]string{"registration": "invalid"}, 400, nil)
	a.call(admin, "PUT", "/api/admin/settings", map[string]string{"message": "welcome", "registration": "invite"}, 200, nil)
}
