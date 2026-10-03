package nekotest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"cozycast/internal/neko"
	"cozycast/internal/neko/nekotest"
	"github.com/coder/websocket"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func status(t *testing.T, s *nekotest.Server, method, path, token string, body any) int {
	t.Helper()
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		must(t, err)
	}
	req, err := http.NewRequest(method, s.URL()+path, bytes.NewReader(data))
	must(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	must(t, err)
	defer res.Body.Close()
	return res.StatusCode
}
func TestHTTPContract(t *testing.T) {
	s := nekotest.New(t, "admin")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := neko.NewClient(s.URL(), "admin")
	must(t, err)
	if !c.Healthy(ctx) {
		t.Fatal("initially unhealthy")
	}
	s.SetHealthy(false)
	if c.Healthy(ctx) {
		t.Fatal("health switch ignored")
	}
	s.SetHealthy(true)
	for _, endpoint := range []struct {
		method, path string
		body         any
	}{
		{"GET", "/api/members", nil}, {"POST", "/api/members", map[string]any{"username": "forbidden", "password": "secret"}},
		{"POST", "/api/members/member", neko.Profile{}}, {"DELETE", "/api/members/member", nil},
		{"POST", "/api/room/settings", map[string]bool{"implicit_hosting": true}}, {"POST", "/api/room/control/reset", nil},
	} {
		for _, token := range []string{"", "wrong"} {
			if got := status(t, s, endpoint.method, endpoint.path, token, endpoint.body); got != 401 {
				t.Fatalf("%s %s with %q: %d", endpoint.method, endpoint.path, token, got)
			}
		}
	}
	p := neko.Profile{Name: "Member", CanLogin: true, CanConnect: true, CanHost: true, Plugins: map[string]any{"filetransfer.enabled": true}}
	must(t, c.CreateMember(ctx, "member", "password", p))
	members, err := c.ListMembers(ctx)
	must(t, err)
	if len(members) != 1 || members[0].ID != "member" || !reflect.DeepEqual(members[0].Profile, p) {
		t.Fatalf("members=%#v", members)
	}
	got, exists := s.Member("member")
	if !exists || !reflect.DeepEqual(got, p) {
		t.Fatalf("Member=%#v,%v", got, exists)
	}
	got.Plugins["filetransfer.enabled"] = false
	detached, _ := s.Member("member")
	if detached.Plugins["filetransfer.enabled"] != true {
		t.Fatal("Member exposes state")
	}
	if !reflect.DeepEqual(s.Members(), []string{"member"}) {
		t.Fatal("Members", s.Members())
	}
	for _, credentials := range []struct{ id, password string }{{"missing", "password"}, {"member", "wrong"}} {
		if got := status(t, s, "POST", "/api/login", "", map[string]string{"username": credentials.id, "password": credentials.password}); got != 401 {
			t.Fatalf("invalid login status=%d", got)
		}
	}
	token, err := c.Login(ctx, "member", "password")
	must(t, err)
	if token == "" {
		t.Fatal("empty login token")
	}
	p.Name = "Updated"
	p.CanHost = false
	must(t, c.UpdateProfile(ctx, "member", p))
	got, _ = s.Member("member")
	if !reflect.DeepEqual(got, p) {
		t.Fatal("UpdateProfile ignored")
	}
	must(t, c.SetImplicitHosting(ctx, true))
	if !s.ImplicitHosting() {
		t.Fatal("settings not recorded")
	}
	must(t, c.DeleteMember(ctx, "member"))
	if len(s.Members()) != 0 {
		t.Fatal("member survived delete")
	}
	for _, err := range []error{c.DeleteMember(ctx, "member"), c.UpdateProfile(ctx, "member", p)} {
		if !errors.Is(err, neko.ErrNotFound) {
			t.Fatalf("missing member error=%v", err)
		}
	}
}

type hostEvent struct {
	Event   string `json:"event"`
	Payload struct {
		HasHost     bool   `json:"has_host"`
		HostID      string `json:"host_id"`
		ControlHost struct {
			HasHost bool   `json:"has_host"`
			HostID  string `json:"host_id"`
		} `json:"control_host"`
	} `json:"payload"`
}

func readHost(t *testing.T, ctx context.Context, conn *websocket.Conn, event, host string) {
	t.Helper()
	_, data, err := conn.Read(ctx)
	must(t, err)
	var msg hostEvent
	must(t, json.Unmarshal(data, &msg))
	if msg.Event != event {
		t.Fatalf("event=%s, want %s", msg.Event, event)
	}
	id, has := msg.Payload.HostID, msg.Payload.HasHost
	if event == "system/init" {
		id, has = msg.Payload.ControlHost.HostID, msg.Payload.ControlHost.HasHost
	}
	if id != host || has != (host != "") {
		t.Fatalf("host=%q has=%v, want %q", id, has, host)
	}
}
func TestWebSocketAndRestart(t *testing.T) {
	s := nekotest.New(t, "admin")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := neko.NewClient(s.URL(), "admin")
	must(t, err)
	must(t, c.CreateMember(ctx, "member", "password", neko.Profile{CanLogin: true}))
	memberToken, err := c.Login(ctx, "member", "password")
	must(t, err)
	for _, token := range []string{"", "wrong", "admin", memberToken} {
		if got := status(t, s, "GET", "/api/ws?token="+token, "", nil); got != 401 {
			t.Fatalf("nonobserver ws status=%d", got)
		}
	}
	must(t, c.CreateMember(ctx, neko.ObserverID, "observe", neko.Profile{IsAdmin: true, CanLogin: true, CanConnect: true}))
	token, err := c.Login(ctx, neko.ObserverID, "observe")
	must(t, err)
	s.SetHost("initial")
	var conns []*websocket.Conn
	for range 2 {
		conn, _, err := websocket.Dial(ctx, strings.Replace(s.URL(), "http:", "ws:", 1)+"/api/ws?token="+token, nil)
		must(t, err)
		defer conn.CloseNow()
		conns = append(conns, conn)
		readHost(t, ctx, conn, "system/init", "initial")
	}
	must(t, s.WaitObservers(ctx, 2))
	s.SetHost("member")
	for _, conn := range conns {
		readHost(t, ctx, conn, "control/host", "member")
	}
	must(t, c.ResetControl(ctx))
	for _, conn := range conns {
		readHost(t, ctx, conn, "control/host", "")
	}
	s.SetHost("member")
	s.ClearHost()
	for _, conn := range conns {
		readHost(t, ctx, conn, "control/host", "member")
		readHost(t, ctx, conn, "control/host", "")
	}
	must(t, c.SetImplicitHosting(ctx, true))
	s.Restart()
	must(t, s.WaitObservers(ctx, 0))
	if len(s.Members()) != 0 || s.ImplicitHosting() {
		t.Fatal("restart retained state")
	}
	for _, conn := range conns {
		if _, _, err := conn.Read(ctx); err == nil {
			t.Fatal("restart retained observer connection")
		}
	}
	if got := status(t, s, "GET", "/api/ws?token="+token, "", nil); got != 401 {
		t.Fatal("restart retained token", got)
	}
	if _, err := c.Login(ctx, "member", "password"); err == nil {
		t.Fatal("restart retained member password")
	}
	// Tokens remain invalid even if the observer is recreated with the same name.
	must(t, c.CreateMember(ctx, neko.ObserverID, "new", neko.Profile{IsAdmin: true, CanLogin: true}))
	newToken, err := c.Login(ctx, neko.ObserverID, "new")
	must(t, err)
	if newToken == token {
		t.Fatal("restart reused token")
	}
	if got := status(t, s, "GET", "/api/ws?token="+token, "", nil); got != 401 {
		t.Fatal("old token became valid again", got)
	}
}
