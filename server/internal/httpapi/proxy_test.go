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
	nc, err := neko.NewClient(fake.URL(), "secret", nil)
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
		// Without the upload right there are no downloads from the desktop.
		{"GET", "/neko/default/api/filetransfer?filename=x&token=" + token, 403},
		{"DELETE", "/neko/default/api/filetransfer?filename=x&token=" + token, 404},
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
		{"GET", "/neko/default/api/filetransfer?filename=x&token=other", 403},
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

func TestNekoProxyDownloads(t *testing.T) {
	p := newProxyTest(t, store.RoomSettings{DefaultUpload: true})
	_, _, token := p.join()
	// neko answers 401 to the fake download: it got through.
	if got := p.status("GET", "/neko/default/api/filetransfer?filename=x&token="+token); got != 401 {
		t.Fatalf("download with the upload right: status %d", got)
	}
	if calls := p.fake.Calls(); calls[len(calls)-1] != (nekotest.Call{Method: "GET", Path: "/api/filetransfer"}) {
		t.Fatalf("last upstream call %+v, want the download", calls[len(calls)-1])
	}
	if got := p.status("DELETE", "/neko/default/api/filetransfer?filename=x&token="+token); got != 404 {
		t.Fatalf("delete: status %d", got)
	}
}

// A file from the desktop is saved, whatever it claims to be.
func TestDownloadsAreNeverDisplayed(t *testing.T) {
	for _, tt := range []struct {
		name, disposition string
		status            int
	}{
		{"movie.mkv", `attachment; filename=movie.mkv`, 200},
		{"evil page.html", `attachment; filename="evil page.html"`, 200},
		{"part.bin", `attachment; filename=part.bin`, 206},
		{"süß.svg", `attachment; filename*=utf-8''s%C3%BC%C3%9F.svg`, 200},
		{"missing", "", 404},
	} {
		res := &http.Response{StatusCode: tt.status, Header: http.Header{"Content-Type": {"text/html; charset=utf-8"}}}
		if err := asDownload(tt.name)(res); err != nil {
			t.Fatal(err)
		}
		h := res.Header
		if h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Content-Security-Policy") != "sandbox" {
			t.Errorf("%s: headers %v", tt.name, h)
		}
		if got := h.Get("Content-Disposition"); got != tt.disposition {
			t.Errorf("%s: disposition %q, want %q", tt.name, got, tt.disposition)
		}
		if saved := h.Get("Content-Type") == "application/octet-stream"; saved != (tt.disposition != "") {
			t.Errorf("%s: content type %q", tt.name, h.Get("Content-Type"))
		}
	}
}

// neko sends every session the list of the files in the desktop's Downloads
// folder. Only tabs with the upload right get it, or may ask for it.
func TestNekoSocketFileList(t *testing.T) {
	const list = `{"event":"filetransfer/update","payload":{"enabled":true,"files":[{"name":"secret.txt","type":"file","size":3}]}}`
	const other = `{"event":"clipboard/updated","payload":{"text":"filetransfer/update"}}`
	for _, allowed := range []bool{false, true} {
		p := newProxyTest(t, store.RoomSettings{DefaultUpload: allowed})
		_, clientID, token := p.join()
		conn := p.socket(token)
		for _, msg := range []string{list, other} {
			if err := p.fake.SendMember(p.ctx, clientID, msg); err != nil {
				t.Fatal(err)
			}
		}
		want := []string{other}
		if allowed {
			want = []string{list, other}
		}
		if _, _, err := conn.Read(p.ctx); err != nil { // system/init
			t.Fatal(err)
		}
		for _, w := range want {
			if _, got, err := conn.Read(p.ctx); err != nil || string(got) != w {
				t.Fatalf("allowed=%v: browser got %s (%v), want %s", allowed, got, err, w)
			}
		}

		for _, msg := range []string{`{"event":"filetransfer/update"}`, `{"event":"control/request"}`} {
			if err := conn.Write(p.ctx, websocket.MessageText, []byte(msg)); err != nil {
				t.Fatal(err)
			}
		}
		want = []string{`{"event":"control/request"}`}
		if allowed {
			want = []string{`{"event":"filetransfer/update"}`, `{"event":"control/request"}`}
		}
		if got := p.received(clientID, len(want)); !slices.Equal(got, want) {
			t.Fatalf("allowed=%v: neko received %q, want %q", allowed, got, want)
		}
	}
}

func TestNekoSocketForwardsPaste(t *testing.T) {
	p := newProxyTest(t, store.RoomSettings{DefaultRemote: true})
	_, clientID, token := p.join()
	conn := p.socket(token)
	const paste = `{"event":"control/paste","payload":{"text":"  local clipboard\n\tsecond line 😀"}}`
	if err := conn.Write(p.ctx, websocket.MessageText, []byte(paste)); err != nil {
		t.Fatal(err)
	}
	if got := p.received(clientID, 1); !slices.Equal(got, []string{paste}) {
		t.Fatalf("neko received %q, want %s", got, paste)
	}
	profile, ok := p.fake.Member(clientID)
	if !ok || !profile.CanHost || !profile.CanAccessClipboard {
		t.Fatalf("remote member profile: %+v (exists=%v)", profile, ok)
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

func TestRoomRemovalClosesNekoProxy(t *testing.T) {
	p := newProxyTest(t, store.RoomSettings{})
	_, _, token := p.join()
	conn := p.socket(token)
	if _, _, err := conn.Read(p.ctx); err != nil {
		t.Fatal(err)
	} // system/init
	if !p.hub.Remove("default", "not_found") {
		t.Fatal("room missing")
	}
	if _, _, err := conn.Read(p.ctx); err == nil || p.ctx.Err() != nil {
		t.Fatalf("proxy did not close promptly: %v", err)
	}
	if err := p.fake.WaitObservers(p.ctx, 0); err != nil {
		t.Fatal(err)
	}
	if got := p.status("GET", "/neko/default/api/ws?token="+token); got != 404 {
		t.Fatalf("removed room still proxied: %d", got)
	}
}
