package httpapi_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
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

const testPassword = "password123"
const passwordError = "Passwords must be at least 8 characters and at most 72 bytes."

type apiTest struct {
	t   *testing.T
	st  *store.Store
	srv *httptest.Server
}

func newAPITest(t *testing.T) *apiTest {
	t.Helper()
	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	nc, err := neko.NewClient("http://127.0.0.1:1", "x")
	if err != nil {
		t.Fatal(err)
	}
	h := hub.New(st, t.TempDir(), []hub.RoomConfig{{Name: "default", Neko: nc}})
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{Store: st, Auth: auth.New(st, false), Hub: h}).Handler())
	t.Cleanup(srv.Close)
	return &apiTest{t: t, st: st, srv: srv}
}

func (a *apiTest) client() *http.Client {
	a.t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		a.t.Fatal(err)
	}
	return &http.Client{Jar: jar, Timeout: 5 * time.Second}
}

func (a *apiTest) user(username string, admin bool) *store.User {
	a.t.Helper()
	hash, err := auth.HashPassword(testPassword)
	if err != nil {
		a.t.Fatal(err)
	}
	u := &store.User{Username: username, Nickname: username, PasswordHash: hash, Admin: admin}
	if err := a.st.CreateUser(context.Background(), u); err != nil {
		a.t.Fatal(err)
	}
	return u
}

func (a *apiTest) login(username string) *http.Client {
	a.t.Helper()
	c := a.client()
	a.call(c, "POST", "/api/auth/login", map[string]any{"username": username, "password": testPassword}, 200, nil)
	return c
}

func (a *apiTest) call(c *http.Client, method, path string, body any, status int, out any) []byte {
	a.t.Helper()
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			a.t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, a.srv.URL+path, bytes.NewReader(data))
	if err != nil {
		a.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	data, err = io.ReadAll(res.Body)
	if err != nil {
		a.t.Fatal(err)
	}
	if res.StatusCode != status {
		a.t.Fatalf("%s %s: status %d, want %d: %s", method, path, res.StatusCode, status, data)
	}
	if status == 204 && len(data) != 0 {
		a.t.Fatalf("204 has a body: %s", data)
	}
	if status >= 400 {
		var failure struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(data, &failure); err != nil || failure.Error == "" {
			a.t.Fatalf("missing JSON error: %s (%v)", data, err)
		}
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			a.t.Fatalf("decode %s: %v", data, err)
		}
	}
	return data
}

func (a *apiTest) error(c *http.Client, method, path string, body any, status int, message string) {
	a.t.Helper()
	var out struct {
		Error string `json:"error"`
	}
	a.call(c, method, path, body, status, &out)
	if out.Error != message {
		a.t.Fatalf("error = %q, want %q", out.Error, message)
	}
}

func (a *apiTest) me(c *http.Client, username string) {
	a.t.Helper()
	var out struct {
		User *store.User `json:"user"`
	}
	a.call(c, "GET", "/api/me", nil, 200, &out)
	if username == "" {
		if out.User != nil {
			a.t.Fatalf("expected anonymous, got %+v", out.User)
		}
	} else if out.User == nil || out.User.Username != username {
		a.t.Fatalf("expected %s, got %+v", username, out.User)
	}
}

// The welcome message confirms the join before moderation. Without Hub.Start,
// neko token generation waits for readiness and ends when the socket closes.
func (a *apiTest) join(c *http.Client) (*websocket.Conn, string) {
	a.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, a.srv.URL+"/api/rooms/default/ws", &websocket.DialOptions{HTTPClient: c})
	if err != nil {
		a.t.Fatal(err)
	}
	a.t.Cleanup(func() { conn.CloseNow() })
	var welcome struct {
		Type string `json:"type"`
		Self struct {
			Key string `json:"key"`
		} `json:"self"`
	}
	if err := wsjson.Read(ctx, conn, &welcome); err != nil {
		a.t.Fatal(err)
	}
	if welcome.Type != "welcome" || welcome.Self.Key == "" {
		a.t.Fatalf("welcome: %+v", welcome)
	}
	return conn, welcome.Self.Key
}

func TestAccountLoginLogout(t *testing.T) {
	a := newAPITest(t)
	a.user("alice", false)
	c := a.client()
	a.me(c, "")
	a.error(c, "POST", "/api/auth/login", map[string]string{"username": "alice", "password": "wrong"}, 401, "Wrong username or password.")
	a.error(c, "POST", "/api/auth/login", map[string]string{"username": "missing", "password": testPassword}, 401, "Wrong username or password.")
	a.call(c, "POST", "/api/auth/login", map[string]string{"username": "ALICE", "password": testPassword}, 200, nil)
	a.me(c, "alice")
	a.call(c, "POST", "/api/auth/logout", nil, 204, nil)
	a.me(c, "")
	a.call(c, "POST", "/api/auth/logout", nil, 204, nil)
	for _, path := range []string{"/api/me", "/api/me/password"} {
		method := "PATCH"
		if strings.HasSuffix(path, "password") {
			method = "POST"
		}
		a.call(c, method, path, map[string]any{}, 401, nil)
	}
}

// oldRefreshToken is a refresh token as the old CozyCast handed it to
// browsers: its stored key, signed as a compact JWS.
func oldRefreshToken(key string) string {
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"HS256"}`)) + "." + enc([]byte(key)) + "." + enc([]byte("signature"))
}

func TestLegacyLogin(t *testing.T) {
	a := newAPITest(t)
	hash := func(s string) []byte {
		h := sha256.Sum256([]byte(s))
		return h[:]
	}
	password, err := auth.HashPassword(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.st.ImportLegacy(context.Background(), store.ImportData{
		Users: []store.User{
			{Username: "alice", Nickname: "alice", PasswordHash: password},
			{Username: "bob", Nickname: "bob", PasswordHash: password, Disabled: true},
		},
		Logins: []store.LegacyLogin{
			{Username: "alice", TokenHash: hash("11111111-2222-3333-4444-555555555555")},
			{Username: "alice", TokenHash: hash("stored as presented")},
			{Username: "alice", TokenHash: hash("revoked by password change")},
			{Username: "bob", TokenHash: hash("disabled account")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	const invalid = "That login is no longer valid."
	post := func(c *http.Client, token string, status int) {
		t.Helper()
		if status == 200 {
			a.call(c, "POST", "/api/auth/legacy", map[string]string{"token": token}, 200, nil)
		} else {
			a.error(c, "POST", "/api/auth/legacy", map[string]string{"token": token}, status, invalid)
		}
	}
	c := a.client()
	// Few enough attempts to stay under the login rate limit.
	for _, token := range []string{"", oldRefreshToken("unknown"), strings.Repeat("a", 513), oldRefreshToken("disabled account")} {
		post(c, token, 401)
	}
	a.me(c, "")

	token := oldRefreshToken("11111111-2222-3333-4444-555555555555")
	post(c, token, 200)
	a.me(c, "alice")
	// The old token is spent; the session it started is an ordinary one.
	post(a.client(), token, 401)
	a.call(c, "POST", "/api/auth/logout", nil, 204, nil)
	a.me(c, "")

	other := a.client()
	post(other, "stored as presented", 200)
	a.me(other, "alice")

	// Changing the password ends the logins not yet used.
	a.call(other, "POST", "/api/me/password", map[string]string{"current": testPassword, "new": testPassword + "2"}, 204, nil)
	post(a.client(), oldRefreshToken("revoked by password change"), 401)
}

func TestPasswordByteBoundary(t *testing.T) {
	a := newAPITest(t)
	if err := a.st.SaveSettings(context.Background(), store.Settings{Registration: "open"}); err != nil {
		t.Fatal(err)
	}
	for i, password := range []string{strings.Repeat("a", 72), strings.Repeat("é", 36)} {
		username := []string{"alice", "bob"}[i]
		a.call(a.client(), "POST", "/api/auth/register", map[string]string{"username": username, "password": password}, 201, nil)
		c := a.client()
		a.call(c, "POST", "/api/auth/login", map[string]string{"username": username, "password": password}, 200, nil)
		for _, name := range []string{username, "missing"} {
			a.error(c, "POST", "/api/auth/login", map[string]string{"username": name, "password": password + "a"}, 401, "Wrong username or password.")
		}
		a.error(c, "POST", "/api/me/password", map[string]string{"current": password + "a", "new": testPassword}, 403, "Your current password is wrong.")
	}
}

func TestAccountProfile(t *testing.T) {
	a := newAPITest(t)
	u := a.user("alice", false)
	a.user("bob", false)
	c := a.login("alice")
	for _, nickname := range []string{"", " leading", "trailing ", "two  spaces", "longnickname1", "😀"} {
		a.call(c, "PATCH", "/api/me", map[string]string{"nickname": nickname, "nameColor": "#abc"}, 400, nil)
	}
	a.call(c, "PATCH", "/api/me", map[string]string{"nickname": "Alice", "nameColor": "orange"}, 400, nil)
	a.error(c, "PATCH", "/api/me", map[string]string{"nickname": "BOB", "nameColor": "#abc"}, 409, "That nickname is another account's username.")
	for _, nickname := range []string{"ALICE", "A Friend"} {
		var out struct {
			User struct {
				Nickname  string `json:"nickname"`
				NameColor string `json:"nameColor"`
				AvatarURL string `json:"avatarUrl"`
			} `json:"user"`
		}
		a.call(c, "PATCH", "/api/me", map[string]string{"nickname": nickname, "nameColor": "#aabbcc"}, 200, &out)
		if out.User.Nickname != nickname || out.User.NameColor != "#aabbcc" || out.User.AvatarURL != "/png/default_avatar.png" {
			t.Fatalf("profile: %+v", out)
		}
	}
	stored, err := a.st.UserByID(context.Background(), u.ID)
	if err != nil || stored.Nickname != "A Friend" || stored.NameColor != "#aabbcc" {
		t.Fatalf("persisted profile: %v %+v", err, stored)
	}
}

func TestAccountChangePassword(t *testing.T) {
	a := newAPITest(t)
	a.user("alice", false)
	current, other := a.login("alice"), a.login("alice")
	a.error(current, "POST", "/api/me/password", map[string]string{"current": "wrong", "new": "newpassword"}, 403, "Your current password is wrong.")
	a.call(current, "POST", "/api/me/password", map[string]string{"current": testPassword, "new": "short"}, 400, nil)
	for _, password := range []string{strings.Repeat("a", 73), strings.Repeat("é", 37)} {
		a.error(current, "POST", "/api/me/password", map[string]string{"current": testPassword, "new": password}, 400, passwordError)
	}
	a.call(current, "POST", "/api/me/password", map[string]string{"current": testPassword, "new": "newpassword"}, 204, nil)
	a.me(current, "alice")
	a.me(other, "")
	a.call(other, "POST", "/api/auth/login", map[string]string{"username": "alice", "password": testPassword}, 401, nil)
	a.call(other, "POST", "/api/auth/login", map[string]string{"username": "alice", "password": "newpassword"}, 200, nil)
}

func TestRegistration(t *testing.T) {
	for _, tt := range []struct {
		name, mode, code string
		invite           bool
		status           int
		message          string
	}{
		{"open", "open", "", false, 201, ""},
		{"requires invite", "invite", "", false, 403, "Registration requires an invite."},
		{"bad invite", "invite", "missing", false, 400, "That invite is invalid or has expired."},
		{"bad optional invite", "open", "missing", false, 400, "That invite is invalid or has expired."},
		{"good invite", "invite", "", true, 201, ""},
		{"optional invite", "open", "", true, 201, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := newAPITest(t)
			ctx := context.Background()
			if err := a.st.SaveSettings(ctx, store.Settings{Registration: tt.mode}); err != nil {
				t.Fatal(err)
			}
			i := &store.Invite{Room: "default", Name: "friends", Remote: true, Image: true, Upload: true}
			if tt.invite {
				if err := a.st.CreateInvite(ctx, i); err != nil {
					t.Fatal(err)
				}
				tt.code = i.Code
			}
			c := a.client()
			req := map[string]string{"username": "Alice", "password": testPassword, "inviteCode": tt.code}
			if tt.message != "" {
				a.error(c, "POST", "/api/auth/register", req, tt.status, tt.message)
				a.me(c, "")
				users, err := a.st.ListUsers(ctx)
				if err != nil || len(users) != 0 {
					t.Fatalf("failed registration created user: %v %+v", err, users)
				}
				return
			}
			a.call(c, "POST", "/api/auth/register", req, 201, nil)
			a.me(c, "alice")
			u, err := a.st.UserByUsername(ctx, "alice")
			if err != nil {
				t.Fatal(err)
			}
			p, err := a.st.Permission(ctx, "default", u.ID)
			if err != nil || p.Invited != tt.invite || (tt.invite && (!p.Remote || !p.Image || !p.Upload || p.InviteName != "friends")) {
				t.Fatalf("registration permission: %v %+v", err, p)
			}
			a.error(a.client(), "POST", "/api/auth/register", req, 409, "That username is taken.")
		})
	}
}

func TestRegistrationValidationAndRateLimit(t *testing.T) {
	a := newAPITest(t)
	if err := a.st.SaveSettings(context.Background(), store.Settings{Registration: "open"}); err != nil {
		t.Fatal(err)
	}
	c := a.client()
	for _, username := range []string{"a", "a--b", "-alice", "longusername1"} {
		a.call(c, "POST", "/api/auth/register", map[string]string{"username": username, "password": testPassword}, 400, nil)
	}
	for _, password := range []string{"short", strings.Repeat("a", 73), strings.Repeat("é", 37), strings.Repeat("a", 101)} {
		a.error(c, "POST", "/api/auth/register", map[string]string{"username": "alice", "password": password}, 400, passwordError)
	}
	for _, username := range []string{"alice", "bob", "carol"} {
		a.call(a.client(), "POST", "/api/auth/register", map[string]string{"username": username, "password": testPassword}, 201, nil)
	}
	a.call(c, "POST", "/api/auth/register", map[string]string{"username": "dave", "password": testPassword}, 429, nil)
	users, err := a.st.ListUsers(context.Background())
	if err != nil || len(users) != 3 {
		t.Fatalf("rate limit created user: %v %+v", err, users)
	}
}

func TestCrossOriginAndInvalidJSON(t *testing.T) {
	a := newAPITest(t)
	c := a.client()
	req, err := http.NewRequest("POST", a.srv.URL+"/api/auth/login", strings.NewReader(`{"username":"alice","password":"password123"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "https://evil.example")
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 403 || out.Error != "cross-origin request rejected" {
		t.Fatalf("cross origin: %d %+v", res.StatusCode, out)
	}
	a.call(c, "POST", "/api/auth/login", map[string]string{"unexpected": "field"}, 400, nil)
}

func TestPublicSettingsAndRooms(t *testing.T) {
	a := newAPITest(t)
	c := a.client()
	var set store.Settings
	a.call(c, "GET", "/api/settings", nil, 200, &set)
	if set.Registration != "invite" {
		t.Fatalf("settings: %+v", set)
	}
	var rooms []struct {
		Name      string `json:"name"`
		Access    string `json:"access"`
		UserCount int    `json:"userCount"`
		Open      bool   `json:"open"`
	}
	a.call(c, "GET", "/api/rooms", nil, 200, &rooms)
	if len(rooms) != 1 || rooms[0].Name != "default" || rooms[0].Access != "public" || rooms[0].UserCount != 0 || !rooms[0].Open {
		t.Fatalf("rooms: %+v", rooms)
	}
}

func TestUnknownRoomSocket(t *testing.T) {
	a := newAPITest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, a.srv.URL+"/api/rooms/missing/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	var msg struct{ Type, Reason string }
	if err := wsjson.Read(ctx, conn, &msg); err != nil || msg.Type != "kicked" || msg.Reason != "not_found" {
		t.Fatalf("unknown room: %+v, %v", msg, err)
	}
	if err := wsjson.Read(ctx, conn, &msg); websocket.CloseStatus(err) != 4000 {
		t.Fatalf("unknown room close: %v", err)
	}
	_, res, err := websocket.Dial(ctx, a.srv.URL+"/api/rooms/missing/ws", &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": {"https://evil.example"}},
	})
	if err == nil || res == nil || res.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin unknown room: %v, %v", res, err)
	}
}
