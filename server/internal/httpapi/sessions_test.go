package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"cozycast/internal/auth"
	"cozycast/internal/store"
)

func TestEndedSessionsCloseRoomSockets(t *testing.T) {
	for _, action := range []string{"logout", "password", "admin reset"} {
		t.Run(action, func(t *testing.T) {
			p := newProxyTest(t, store.RoomSettings{DefaultRemote: true})
			hash, err := auth.HashPassword("password123")
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"alice", "bob"} {
				u := &store.User{Username: name, Nickname: name, PasswordHash: hash, Admin: name == "bob"}
				if err := p.st.CreateUser(p.ctx, u); err != nil {
					t.Fatal(err)
				}
			}
			request := func(c *http.Client, method, path, body string) *http.Response {
				t.Helper()
				req, err := http.NewRequestWithContext(p.ctx, method, p.srv.URL+path, strings.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Content-Type", "application/json")
				res, err := c.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				return res
			}
			post := func(c *http.Client, path, body string, status int) {
				t.Helper()
				res := request(c, "POST", path, body)
				defer res.Body.Close()
				if res.StatusCode != status {
					t.Fatalf("POST %s: status %d, want %d", path, res.StatusCode, status)
				}
			}
			browser := func(name string) *http.Client {
				t.Helper()
				jar, err := cookiejar.New(nil)
				if err != nil {
					t.Fatal(err)
				}
				c := &http.Client{Jar: jar, Timeout: 5 * time.Second}
				if name != "" {
					post(c, "/api/auth/login", `{"username":"`+name+`","password":"password123"}`, 200)
				}
				return c
			}
			first, second, other, anonymous := browser("alice"), browser("alice"), browser("bob"), browser("")
			type tab struct {
				session         int
				clientID, token string
				conn, proxy     *websocket.Conn
			}
			var tabs []tab
			for session, owner := range []*http.Client{first, second, other, anonymous} {
				n := 1
				if session < 2 {
					n = 2
				}
				for i := 0; i < n; i++ {
					conn, _, err := websocket.Dial(p.ctx, p.srv.URL+"/api/rooms/default/ws", &websocket.DialOptions{HTTPClient: owner})
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { conn.CloseNow() })
					tab := tab{session: session, conn: conn}
					for tab.token == "" {
						var msg struct{ Type, ClientID, Token string }
						if err := wsjson.Read(p.ctx, conn, &msg); err != nil {
							t.Fatal(err)
						}
						if msg.Type == "welcome" {
							tab.clientID = msg.ClientID
						}
						tab.token = msg.Token
					}
					tab.proxy = p.socket(tab.token)
					var init struct{ Event string }
					if err := wsjson.Read(p.ctx, tab.proxy, &init); err != nil || init.Event != "system/init" {
						t.Fatalf("neko init: %+v, %v", init, err)
					}
					tabs = append(tabs, tab)
				}
			}
			switch action {
			case "logout":
				post(first, "/api/auth/logout", "", 204)
			case "password":
				post(first, "/api/me/password", `{"current":"password123","new":"newpassword123"}`, 204)
			case "admin reset":
				post(other, "/api/admin/users/alice/password", `{"password":"newpassword123"}`, 204)
			}
			for _, tab := range tabs {
				ended := tab.session < 2 && (action == "admin reset" || (action == "logout" && tab.session == 0) || (action == "password" && tab.session == 1))
				if ended {
					var msg struct{ Type, Reason string }
					for msg.Type != "kicked" {
						if err := wsjson.Read(p.ctx, tab.conn, &msg); err != nil {
							t.Fatal(err)
						}
					}
					if msg.Reason != "session" {
						t.Fatalf("kick reason: %q", msg.Reason)
					}
					if err := wsjson.Read(p.ctx, tab.conn, &msg); websocket.CloseStatus(err) != statusKicked {
						t.Fatalf("session close: %v", err)
					}
					if _, _, err := tab.proxy.Read(p.ctx); err == nil || p.ctx.Err() != nil {
						t.Fatalf("neko proxy stayed open: %v", err)
					}
					if p.hub.Room("default").NekoTokenIssued(tab.token) {
						t.Fatal("ended tab's token is still issued")
					}
					if err := p.fake.WaitMemberDeleted(p.ctx, tab.clientID); err != nil {
						t.Fatal("ended tab's neko member survived:", err)
					}
					continue
				}
				if err := wsjson.Write(p.ctx, tab.conn, map[string]string{"type": "neko_token"}); err != nil {
					t.Fatal(err)
				}
				for {
					var msg struct{ Type, Token string }
					if err := wsjson.Read(p.ctx, tab.conn, &msg); err != nil {
						t.Fatal("surviving tab closed:", err)
					}
					if msg.Type == "kicked" {
						t.Fatal("unrelated tab was kicked")
					}
					if msg.Type == "neko" {
						if msg.Token == "" {
							t.Fatal("surviving tab has no neko token")
						}
						break
					}
				}
				if _, ok := p.fake.Member(tab.clientID); !ok {
					t.Fatal("surviving tab's neko member was deleted")
				}
				if err := wsjson.Write(p.ctx, tab.proxy, map[string]string{"event": "control/request"}); err != nil {
					t.Fatal("surviving neko proxy closed:", err)
				}
				if _, err := p.fake.WaitReceived(p.ctx, tab.clientID, 1); err != nil {
					t.Fatal("surviving neko proxy did not forward:", err)
				}
			}
			wantCount := 3
			if action == "admin reset" {
				wantCount = 2
			}
			if got := p.hub.Room("default").UserCount(); got != wantCount {
				t.Fatalf("room has %d identities, want %d", got, wantCount)
			}
			for i, c := range []*http.Client{first, second, other, anonymous} {
				res := request(c, "GET", "/api/me", "")
				var me struct{ User *json.RawMessage }
				err := json.NewDecoder(res.Body).Decode(&me)
				res.Body.Close()
				wantAccount := i == 2 || (action == "logout" && i == 1) || (action == "password" && i == 0)
				if err != nil || res.StatusCode != 200 || (me.User != nil) != wantAccount {
					t.Fatalf("session %d: user=%v, status=%d, err=%v", i, me.User, res.StatusCode, err)
				}
			}
		})
	}
}
