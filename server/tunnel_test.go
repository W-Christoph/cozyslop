package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"testing"
	"time"

	"cozycast/internal/config"
	"cozycast/internal/hub"
	"cozycast/internal/neko/nekotest"
	"cozycast/internal/store"
	"cozycast/internal/tunnel"
)

func freeUDPPort(t *testing.T) int {
	t.Helper()
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port
}

// A paired room as it will run at someone's home: its neko is only
// reachable inside the node's end of the tunnel.
func TestPairedRoomThroughTunnel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cfg := config.Config{DataDir: t.TempDir(), TunnelPort: freeUDPPort(t), TunnelNet: netip.MustParsePrefix("10.77.0.0/24"), DefaultScreen: "1280x720@30"}

	nodeKey, err := tunnel.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	nodeAddr := netip.MustParseAddr("10.77.0.2")
	err = db.CreateRegisteredRoom(ctx, &store.RegisteredRoom{Name: "home", NekoURL: "http://10.77.0.2:8080",
		NekoToken: "node-secret", NodeKey: nodeKey.Public().String(), TunnelAddress: nodeAddr.String()})
	if err != nil {
		t.Fatal(err)
	}

	tun, err := openTunnel(ctx, db, cfg)
	if err != nil || tun == nil {
		t.Fatalf("tunnel: %v", err)
	}
	defer tun.Close()
	if tun.Address() != netip.MustParseAddr("10.77.0.1") || tun.Port() != cfg.TunnelPort {
		t.Fatalf("hub end: %s:%d", tun.Address(), tun.Port())
	}
	// The key survives a restart.
	tun.Close()
	again, err := openTunnel(ctx, db, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if again.PublicKey() != tun.PublicKey() {
		t.Fatal("server key changed across restarts")
	}
	tun = again
	defer tun.Close()

	// The node: neko behind its tunnel address only.
	node, err := tunnel.Open(tunnel.Config{PrivateKey: nodeKey, Address: nodeAddr, Network: cfg.TunnelNet})
	if err != nil {
		t.Fatal(err)
	}
	defer node.Close()
	err = node.SetPeer(tunnel.Peer{Key: tun.PublicKey(), Address: tun.Address(), Keepalive: time.Second,
		Endpoint: netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(cfg.TunnelPort))})
	if err != nil {
		t.Fatal(err)
	}
	fake := nekotest.New(t, "node-secret")
	target, _ := url.Parse(fake.URL())
	l, err := node.Listen(8080)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: httputil.NewSingleHostReverseProxy(target)}
	go srv.Serve(l)
	defer srv.Close()

	rooms, err := loadRooms(ctx, db, cfg, roomBuilder(cfg, tun))
	if err != nil || len(rooms) != 1 || rooms[0].Source != "paired" {
		t.Fatalf("rooms: %+v %v", rooms, err)
	}
	// The other computer's desktop size stands.
	if rooms[0].DefaultScreen != "" {
		t.Fatalf("paired room got the server's default screen %q", rooms[0].DefaultScreen)
	}
	h := hub.New(db, t.TempDir(), rooms)
	if err := h.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, r := range h.Rooms() {
			h.Remove(r.Name, "not_found")
		}
	}()
	// The server's observer reaches neko's API and WebSocket through the
	// tunnel; nothing else could reach it.
	waitCtx, stop := context.WithTimeout(ctx, 20*time.Second)
	defer stop()
	if err := fake.WaitObservers(waitCtx, 1); err != nil {
		t.Fatal("observer through the tunnel:", err)
	}

	// Where the node was seen is remembered for the next start.
	saveEndpoints(ctx, db, tun)
	saved, err := db.RegisteredRoom(ctx, "home")
	if err != nil {
		t.Fatal(err)
	}
	if ep, err := netip.ParseAddrPort(saved.NodeEndpoint); err != nil || ep.Port() != uint16(node.Port()) {
		t.Fatalf("endpoint %q, node port %d: %v", saved.NodeEndpoint, node.Port(), err)
	}
	if p := nodePeer(saved); p.Key != nodeKey.Public() || p.Address != nodeAddr || p.Endpoint.String() != saved.NodeEndpoint {
		t.Fatalf("peer from storage: %+v", p)
	}
}

func TestTunnelOff(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tun, err := openTunnel(ctx, db, config.Config{DataDir: t.TempDir()})
	if err != nil || tun != nil {
		t.Fatalf("tunnel without a port: %v %v", tun, err)
	}
	// Without a tunnel nothing is routed into it.
	if tun.Owns("http://10.77.0.2:8080") {
		t.Fatal("nil tunnel owns an address")
	}
}
