package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cozycast/internal/auth"
	"cozycast/internal/config"
	"cozycast/internal/egress"
	"cozycast/internal/fwd"
	"cozycast/internal/httpapi"
	"cozycast/internal/hub"
	"cozycast/internal/neko/nekotest"
	"cozycast/internal/node"
	"cozycast/internal/store"
)

// lockedBuffer collects what the agent prints.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// relay stands in for the room container: its address is fixed before the
// room (a fake neko, which needs the token pairing creates) exists.
type relay struct{ target atomic.Value }

func newRelay(t *testing.T) (*relay, string) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	r := &relay{}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				target, _ := r.target.Load().(string)
				up, err := net.Dial("tcp", target)
				if err != nil {
					return
				}
				defer up.Close()
				done := make(chan struct{}, 2)
				go func() { io.Copy(up, c); done <- struct{}{} }()
				go func() { io.Copy(c, up); done <- struct{}{} }()
				<-done
			}()
		}
	}()
	return r, l.Addr().String()
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestPairingEndToEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cfg := config.Config{DataDir: t.TempDir(), TunnelPort: freeUDPPort(t), TunnelNet: netip.MustParsePrefix("10.77.0.0/24")}
	tun, err := openTunnel(ctx, db, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer tun.Close()
	h := hub.New(db, t.TempDir(), nil)
	if err := h.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, r := range h.Rooms() {
			h.Remove(r.Name, "not_found")
		}
	}()
	// Media: the server's public port is on 127.0.0.1, the room's neko
	// (an echo here) on 127.0.0.2 behind the agent, both on the same port.
	mediaPort := freeUDPPort(t)
	media := &fwd.Ports{Host: "127.0.0.1", Dial: tun.DialContext}
	publicIP := netip.MustParseAddr("203.0.113.10")
	api := httpapi.New(httpapi.Deps{Store: db, Auth: auth.New(db, false), Hub: h, Tunnel: tun, BuildRoom: roomBuilder(cfg, tun),
		Media: media, PublicIP: publicIP, MediaPorts: [2]int{mediaPort, mediaPort}})
	srv := httptest.NewServer(api.Handler())
	defer srv.Close()
	l, err := tun.Listen(80)
	if err != nil {
		t.Fatal(err)
	}
	go http.Serve(l, api.NodeHandler())
	// The way out, allowed to reach only the test's "internet" (a loopback
	// site); everything else counts as a private network.
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "a website") }))
	defer site.Close()
	wayOut := egress.New()
	wayOut.Allow = func(ip netip.Addr) bool { return ip == netip.MustParseAddr("127.0.0.1") }
	el, err := tun.Listen(egress.Port)
	if err != nil {
		t.Fatal(err)
	}
	go http.Serve(el, wayOut)

	// An admin, logged in.
	hash, _ := auth.HashPassword("password123")
	if err := db.CreateUser(ctx, &store.User{Username: "root", Nickname: "root", PasswordHash: hash, Admin: true}); err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	admin := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	call := func(method, path string, body any, out any) int {
		t.Helper()
		data, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, srv.URL+path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", srv.URL)
		res, err := admin.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if out != nil {
			json.NewDecoder(res.Body).Decode(out)
		}
		return res.StatusCode
	}
	if code := call("POST", "/api/auth/login", map[string]string{"username": "root", "password": "password123"}, nil); code != 200 {
		t.Fatal("login", code)
	}

	// The computer at home: an agent in front of a room that is not up yet.
	room, roomAddr := newRelay(t)
	pl, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	proxyAddr := pl.Addr().String()
	pl.Close()
	stateDir := t.TempDir()
	envFile := filepath.Join(t.TempDir(), "room.env")
	agent := func(out io.Writer) (context.CancelFunc, chan error) {
		actx, stop := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() {
			done <- node.Run(actx, node.Config{Hub: srv.URL, Name: "home", StateDir: stateDir, EnvFile: envFile,
				Forward: map[int]string{8080: roomAddr}, ProxyListen: proxyAddr, RoomHost: "127.0.0.2",
				Out: out, CheckEvery: 100 * time.Millisecond, RetryEvery: 100 * time.Millisecond})
		}()
		return stop, done
	}
	out := &lockedBuffer{}
	stop, done := agent(out)

	// The admin sees the request with the code the computer shows.
	type request struct{ ID, Name, Code, IP string }
	var pending []request
	eventually(t, "request", func() bool {
		call("GET", "/api/admin/pairing", nil, &pending)
		return len(pending) == 1 && strings.Contains(out.String(), "Code: ")
	})
	shown := regexp.MustCompile(`Code: (\S+)`).FindStringSubmatch(out.String())[1]
	if pending[0].Code != shown || pending[0].Name != "home" {
		t.Fatalf("admin sees %+v, computer shows %s", pending[0], shown)
	}
	if !strings.Contains(out.String(), "is plain HTTP") {
		t.Fatalf("no plain HTTP warning:\n%s", out)
	}
	// A name that is taken is refused; the request stays.
	if code := call("POST", "/api/admin/pairing/"+pending[0].ID+"/accept", map[string]string{"name": "bad name"}, nil); code != 400 {
		t.Fatal("bad name accepted", code)
	}
	var accepted struct{ Name, Source string }
	if code := call("POST", "/api/admin/pairing/"+pending[0].ID+"/accept", map[string]string{"name": "living-room"}, &accepted); code != 200 || accepted.Source != "paired" {
		t.Fatalf("accept: %d %+v", code, accepted)
	}
	if code := call("POST", "/api/admin/pairing/"+pending[0].ID+"/accept", map[string]string{"name": "again"}, nil); code != 404 {
		t.Fatal("accepted twice", code)
	}

	// The computer learns its room, gets the token through the tunnel and
	// writes it for the room container.
	var env string
	eventually(t, "room settings", func() bool {
		data, err := os.ReadFile(envFile)
		env = string(data)
		return err == nil
	})
	m := regexp.MustCompile(`(?m)^COZYCAST_ROOM='living-room'\nCOZYCAST_NEKO_TOKEN='([A-Za-z0-9_-]+)'$`).FindStringSubmatch(env)
	if m == nil {
		t.Fatalf("env file:\n%s", env)
	}
	// neko announces the server's address and the media port.
	for _, line := range []string{
		fmt.Sprintf("NEKO_WEBRTC_UDPMUX='%d'", mediaPort), fmt.Sprintf("NEKO_WEBRTC_TCPMUX='%d'", mediaPort),
		"NEKO_WEBRTC_ICELITE='true'", "NEKO_WEBRTC_NAT1TO1='203.0.113.10'",
	} {
		if !strings.Contains(env, line+"\n") {
			t.Fatalf("env file lacks %s:\n%s", line, env)
		}
	}
	// A viewer's media reaches neko through the server and the computer,
	// over UDP and TCP, and the answers come back.
	echoMedia(t, "127.0.0.2", mediaPort)
	viewerUDP, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", fmt.Sprint(mediaPort)))
	if err != nil {
		t.Fatal(err)
	}
	defer viewerUDP.Close()
	eventually(t, "media over UDP", func() bool { return exchange(viewerUDP, "rtp") == "echo:rtp" })
	viewerTCP, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprint(mediaPort)))
	if err != nil {
		t.Fatal(err)
	}
	defer viewerTCP.Close()
	if got := exchange(viewerTCP, "ice-tcp"); got != "echo:ice-tcp" {
		t.Fatalf("media over TCP: %q", got)
	}
	if fi, _ := os.Stat(envFile); fi.Mode().Perm() != 0o600 {
		t.Fatalf("env file mode %v", fi.Mode())
	}
	// The room comes up; the server reaches it through tunnel and agent.
	fake := nekotest.New(t, m[1])
	room.target.Store(strings.TrimPrefix(fake.URL(), "http://"))
	waitCtx, waitStop := context.WithTimeout(ctx, 30*time.Second)
	defer waitStop()
	if err := fake.WaitObservers(waitCtx, 1); err != nil {
		t.Fatalf("server never reached the room: %v\n%s", err, out)
	}
	eventually(t, "connected in the admin list", func() bool {
		var rooms []struct {
			Name      string
			Connected bool
		}
		call("GET", "/api/admin/rooms", nil, &rooms)
		return len(rooms) == 1 && rooms[0].Name == "living-room" && rooms[0].Connected
	})

	// The room's apps reach the internet only through the agent's proxy, the
	// tunnel and the server; private addresses stay out of reach.
	proxyURL, _ := url.Parse("http://" + proxyAddr)
	browser := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	res, err := browser.Get(site.URL)
	if err != nil {
		t.Fatal("browsing through the tunnel:", err)
	}
	page, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(page) != "a website" {
		t.Fatalf("page: %q", page)
	}
	_, sitePort, _ := net.SplitHostPort(strings.TrimPrefix(site.URL, "http://"))
	if res, err := browser.Get("http://127.0.0.2:" + sitePort); err != nil || res.StatusCode != http.StatusForbidden {
		t.Fatalf("private address through the tunnel: %v %v", res, err)
	}

	// The computer restarts: no code, straight back.
	observerDials := func() int {
		n := 0
		for _, c := range fake.Calls() {
			if c.Path == "/api/ws" {
				n++
			}
		}
		return n
	}
	before := observerDials()
	stop()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	again := &lockedBuffer{}
	stop, done = agent(again)
	defer func() { stop(); <-done }()
	eventually(t, "reconnect", func() bool { return strings.Contains(again.String(), `Connected to`) })
	if strings.Contains(again.String(), "Code:") {
		t.Fatalf("paired again after a restart:\n%s", again)
	}
	// The server notices the restart from the agent's check-in and does
	// not wait for its next ping (20 s) to find the old connection dead.
	restarted := time.Now()
	eventually(t, "a new observer", func() bool { return observerDials() > before })
	quick, quickStop := context.WithTimeout(ctx, 5*time.Second)
	defer quickStop()
	if err := fake.WaitObservers(quick, 1); err != nil || time.Since(restarted) > 5*time.Second {
		t.Fatalf("server slow to reach the restarted computer: %v after %v", err, time.Since(restarted))
	}

	// A second computer, rejected.
	other := &lockedBuffer{}
	otherCfg := node.Config{Hub: srv.URL, Name: "intruder", StateDir: t.TempDir(), EnvFile: filepath.Join(t.TempDir(), "env"),
		Out: other, RetryEvery: 100 * time.Millisecond}
	otherDone := make(chan error, 1)
	go func() { otherDone <- node.Run(ctx, otherCfg) }()
	eventually(t, "second request", func() bool {
		call("GET", "/api/admin/pairing", nil, &pending)
		return len(pending) == 1 && pending[0].Name == "intruder"
	})
	if code := call("DELETE", "/api/admin/pairing/"+pending[0].ID, nil, nil); code != 204 {
		t.Fatal("reject", code)
	}
	select {
	case err := <-otherDone:
		if !errors.Is(err, node.ErrRejected) {
			t.Fatalf("rejected computer: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("rejected computer kept waiting")
	}
	// Restarted, it does not ask again.
	if err := node.Run(ctx, otherCfg); !errors.Is(err, node.ErrRejected) {
		t.Fatalf("rejected computer after a restart: %v", err)
	}
	if call("GET", "/api/admin/pairing", nil, &pending); len(pending) != 0 {
		t.Fatalf("asked again: %+v", pending)
	}
}

// echoMedia stands in for neko's media port: UDP and TCP, answering
// "echo:" and what it got.
func echoMedia(t *testing.T, host string, port int) {
	addr := net.JoinHostPort(host, fmt.Sprint(port))
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			pc.WriteTo(append([]byte("echo:"), buf[:n]...), from)
		}
	}()
	l, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				buf := make([]byte, 2048)
				for {
					n, err := c.Read(buf)
					if err != nil {
						return
					}
					c.Write(append([]byte("echo:"), buf[:n]...))
				}
			}()
		}
	}()
}

// exchange sends msg and returns the answer, "" if none came.
func exchange(c net.Conn, msg string) string {
	c.SetDeadline(time.Now().Add(time.Second))
	if _, err := c.Write([]byte(msg)); err != nil {
		return ""
	}
	buf := make([]byte, 2048)
	n, _ := c.Read(buf)
	return string(buf[:n])
}
