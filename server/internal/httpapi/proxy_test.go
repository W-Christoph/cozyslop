package httpapi

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"cozycast/internal/auth"
	"cozycast/internal/hub"
	"cozycast/internal/neko"
	"cozycast/internal/neko/nekotest"
	"cozycast/internal/store"
)

type proxyTest struct {
	t    *testing.T
	ctx  context.Context
	st   *store.Store
	hub  *hub.Hub
	fake *nekotest.Server
	nc   *neko.Client
	srv  *httptest.Server
	http *http.Client
}

func newProxyTest(t *testing.T, set store.RoomSettings) *proxyTest {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	st, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	set.Name, set.Access = "default", "public"
	if err := st.SaveRoomSettings(ctx, set); err != nil {
		t.Fatal(err)
	}
	fake := nekotest.New(t, "secret")
	nc, err := neko.NewClient(fake.URL(), "secret")
	if err != nil {
		t.Fatal(err)
	}
	h := hub.New(st, t.TempDir(), []hub.RoomConfig{{Name: "default", Neko: nc}})
	hubCtx, stop := context.WithCancel(context.Background())
	if err := h.Start(hubCtx); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(Deps{Store: st, Auth: auth.New(st, false), Hub: h}).Handler())
	t.Cleanup(func() {
		srv.Close()
		stop()
		waitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		fake.WaitObservers(waitCtx, 0)
	})
	// The room's stream is only known once neko reported its pipelines.
	for h.Room("default").Streams() == nil {
		select {
		case <-ctx.Done():
			t.Fatal("no streams reported")
		case <-time.After(time.Millisecond):
		}
	}
	jar, _ := cookiejar.New(nil)
	return &proxyTest{t, ctx, st, h, fake, nc, srv, &http.Client{Jar: jar, Timeout: 5 * time.Second}}
}

// join enters the room as a new anonymous browser and returns its identity
// key, client id (= neko member id) and neko token.
func (p *proxyTest) join() (key, clientID, token string) {
	p.t.Helper()
	conn, _, err := websocket.Dial(p.ctx, p.srv.URL+"/api/rooms/default/ws", &websocket.DialOptions{HTTPClient: p.http})
	if err != nil {
		p.t.Fatal(err)
	}
	p.t.Cleanup(func() { conn.CloseNow() })
	for token == "" {
		var msg struct {
			Type, ClientID, Token string
			Self                  struct{ Key string }
		}
		if err := wsjson.Read(p.ctx, conn, &msg); err != nil {
			p.t.Fatal(err)
		}
		if msg.Type == "welcome" {
			key, clientID = msg.Self.Key, msg.ClientID
		}
		token = msg.Token
	}
	go func() { // keep answering pings
		for {
			if _, _, err := conn.Read(context.Background()); err != nil {
				return
			}
		}
	}()
	return key, clientID, token
}

func (p *proxyTest) status(method, path string) int {
	p.t.Helper()
	req, _ := http.NewRequestWithContext(p.ctx, method, p.srv.URL+path, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		p.t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

func (p *proxyTest) socket(token string) *websocket.Conn {
	p.t.Helper()
	conn, _, err := websocket.Dial(p.ctx, p.srv.URL+"/neko/default/api/ws?token="+token, nil)
	if err != nil {
		p.t.Fatal(err)
	}
	p.t.Cleanup(func() { conn.CloseNow() })
	return conn
}

func (p *proxyTest) received(clientID string, n int) []string {
	p.t.Helper()
	got, err := p.fake.WaitReceived(p.ctx, clientID, n)
	if err != nil {
		p.t.Fatalf("neko received %q, want %d messages", got, n)
	}
	return got
}

func TestNekoProxyRoutes(t *testing.T) {
	p := newProxyTest(t, store.RoomSettings{})
	_, _, token := p.join()
	for _, tt := range []struct {
		method, path string
		want         int
	}{
		// neko answers 401 to the fake upload: it got through.
		{"POST", "/neko/default/api/filetransfer?token=" + token, 401},
		// Downloads from the desktop are not offered.
		{"GET", "/neko/default/api/filetransfer?filename=x&token=" + token, 404},
		{"POST", "/neko/default/api/filetransfer/x?token=" + token, 404},
		{"POST", "/neko/default/api/ws?token=" + token, 404},
		{"GET", "/neko/default/api/ws/x?token=" + token, 404},
		{"GET", "/neko/default/api/members?token=" + token, 404},
		{"POST", "/neko/default/api/members?token=" + token, 404},
		{"POST", "/neko/default/api/login", 404},
		{"GET", "/neko/default/api/room/upload/drop?token=" + token, 404},
		{"GET", "/neko/default/api/wsx?token=" + token, 404},
		{"GET", "/neko/other/api/ws?token=" + token, 404},
		// Only tokens the hub issued get through.
		{"POST", "/neko/default/api/filetransfer?token=other", 403},
		{"POST", "/neko/default/api/filetransfer", 403},
		{"GET", "/neko/default/api/ws?token=other", 403},
		{"GET", "/neko/default/api/ws", 403},
	} {
		if got := p.status(tt.method, tt.path); got != tt.want {
			t.Errorf("%s %s: status %d, want %d", tt.method, tt.path, got, tt.want)
		}
	}
	calls := p.fake.Calls()
	if n := len(calls); calls[n-1].Method != "POST" || calls[n-1].Path != "/api/filetransfer" {
		t.Errorf("last upstream call %+v, want the upload", calls[n-1])
	}
	if slices.ContainsFunc(calls, func(c nekotest.Call) bool { return c.Path == "/api/filetransfer" && c.Method != "POST" }) {
		t.Error("a download reached neko")
	}
}

// A member that someone created with neko's admin token (which the room's
// desktop user can read) has a token neko accepts. The proxy must not.
func TestNekoProxyRefusesForeignMembers(t *testing.T) {
	p := newProxyTest(t, store.RoomSettings{})
	if err := p.nc.CreateMember(p.ctx, "rogue", "pw", neko.Profile{IsAdmin: true, CanLogin: true, CanConnect: true, CanWatch: true, CanHost: true}); err != nil {
		t.Fatal(err)
	}
	token, err := p.nc.Login(p.ctx, "rogue", "pw")
	if err != nil {
		t.Fatal(err)
	}
	if got := p.status("GET", "/neko/default/api/ws?token="+token); got != 403 {
		t.Fatal("rogue member socket:", got)
	}
	if got := p.status("POST", "/neko/default/api/filetransfer?token="+token); got != 403 {
		t.Fatal("rogue member upload:", got)
	}
}

func TestNekoSocketKeepsViewersOnTheRoomStream(t *testing.T) {
	room, other, next := nekotest.Streams[1], nekotest.Streams[2], nekotest.Streams[0]
	p := newProxyTest(t, store.RoomSettings{Stream: room})
	key, clientID, token := p.join()
	conn := p.socket(token)

	// neko's messages reach the browser.
	var init struct {
		Event   string
		Payload struct {
			SessionID string `json:"session_id"`
		}
	}
	if err := wsjson.Read(p.ctx, conn, &init); err != nil || init.Event != "system/init" || init.Payload.SessionID != clientID {
		t.Fatalf("first message %+v, %v", init, err)
	}

	// Whatever pipeline the browser asks for, neko is asked for the room's.
	selector := func(id string) string { return `"selector":{"id":"` + id + `","type":"exact"}` }
	for _, msg := range []string{
		`{"event":"signal/request","payload":{"video":{` + selector(other) + `},"audio":{}}}`,
		`{"event":"signal/video","payload":{` + selector(other) + `,"auto":true}}`,
		`{"event":"control/request"}`,
		`not json`,
		`{"event":"signal/candidate","payload":{"candidate":"x"}}`,
	} {
		if err := conn.Write(p.ctx, websocket.MessageBinary, []byte(msg)); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{
		`{"event":"signal/request","payload":{"video":{` + selector(room) + `},"audio":{}}}`,
		`{"event":"signal/video","payload":{` + selector(room) + `}}`,
		`{"event":"control/request"}`,
		`{"event":"signal/candidate","payload":{"candidate":"x"}}`,
	}
	if got := p.received(clientID, len(want)); !slices.Equal(got, want) {
		t.Fatalf("neko received\n %s\nwant\n %s", strings.Join(got, "\n "), strings.Join(want, "\n "))
	}

	// When the room's stream changes, the connection is moved along even if
	// the browser does nothing.
	if err := p.st.SaveRoomSettings(p.ctx, store.RoomSettings{Name: "default", Access: "public", Stream: next}); err != nil {
		t.Fatal(err)
	}
	p.hub.RoomSettingsChanged(p.ctx, "default")
	if got := p.received(clientID, len(want)+1); got[len(want)] != `{"event":"signal/video","payload":{`+selector(next)+`}}` {
		t.Fatal("after a stream change neko received", got[len(want)])
	}

	// Kicked: the connection ends and the token is worth nothing.
	if err := p.hub.Room("default").Kick(key); err != nil {
		t.Fatal(err)
	}
	for {
		if _, _, err := conn.Read(p.ctx); err != nil {
			break
		}
	}
	if p.ctx.Err() != nil {
		t.Fatal("the proxied socket stayed open after a kick")
	}
	if got := p.status("GET", "/neko/default/api/ws?token="+token); got != 403 {
		t.Fatal("socket with a kicked tab's token:", got)
	}
}
