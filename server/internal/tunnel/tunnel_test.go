package tunnel_test

import (
	"context"
	"io"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cozycast/internal/tunnel"
)

var (
	network = netip.MustParsePrefix("10.77.0.0/24")
	hubAddr = netip.MustParseAddr("10.77.0.1")
	nodeIP  = netip.MustParseAddr("10.77.0.2")
)

func open(t *testing.T, addr netip.Addr) *tunnel.Tunnel {
	t.Helper()
	key, err := tunnel.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	tun, err := tunnel.Open(tunnel.Config{PrivateKey: key, Address: addr, Network: network})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tun.Close() })
	return tun
}

// pair connects a hub and a node the way pairing will: the node knows where
// the hub is and keeps the tunnel open; the hub only knows the node's key.
func pair(t *testing.T) (hub, node *tunnel.Tunnel) {
	t.Helper()
	hub, node = open(t, hubAddr), open(t, nodeIP)
	if err := hub.SetPeer(tunnel.Peer{Key: node.PublicKey(), Address: nodeIP}); err != nil {
		t.Fatal(err)
	}
	err := node.SetPeer(tunnel.Peer{Key: hub.PublicKey(), Address: hubAddr,
		Endpoint: netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(hub.Port())), Keepalive: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return hub, node
}

func serve(t *testing.T, node *tunnel.Tunnel, body string) {
	t.Helper()
	l, err := node.Listen(8080)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, body) })}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
}

func get(hub *tunnel.Tunnel, url string, timeout time.Duration) (string, error) {
	client := &http.Client{Timeout: timeout, Transport: &http.Transport{DialContext: hub.DialContext}}
	res, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	return string(b), err
}

func TestHubReachesNode(t *testing.T) {
	hub, node := pair(t)
	serve(t, node, "neko here")
	got, err := get(hub, "http://10.77.0.2:8080/health", 10*time.Second)
	if err != nil || got != "neko here" {
		t.Fatalf("through the tunnel: %q %v", got, err)
	}
	if hub.LastHandshake(node.PublicKey()).IsZero() || node.LastHandshake(hub.PublicKey()).IsZero() {
		t.Fatal("no handshake recorded")
	}
	if !hub.LastHandshake(hubAddrKey(t)).IsZero() {
		t.Fatal("handshake reported for an unknown peer")
	}

	// Only tunnel addresses can be dialed.
	if _, err := hub.DialContext(context.Background(), "tcp", "192.0.2.1:80"); err == nil {
		t.Fatal("dialed outside the tunnel")
	}
	if _, err := hub.DialContext(context.Background(), "tcp", "example.com:80"); err == nil {
		t.Fatal("dialed a host name")
	}

	// A removed peer is cut off at once.
	if err := hub.RemovePeer(node.PublicKey()); err != nil {
		t.Fatal(err)
	}
	if _, err := get(hub, "http://10.77.0.2:8080/health", 2*time.Second); err == nil {
		t.Fatal("removed peer still reachable")
	}
}

func TestNodeReconnectsAfterHubRestart(t *testing.T) {
	hubKey, err := tunnel.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	cfg := tunnel.Config{PrivateKey: hubKey, Address: hubAddr, Network: network}
	hub, err := tunnel.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	node := open(t, nodeIP)
	serve(t, node, "still here")
	cfg.Port = hub.Port()
	endpoint := netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(cfg.Port))
	if err := node.SetPeer(tunnel.Peer{Key: hub.PublicKey(), Address: hubAddr, Endpoint: endpoint, Keepalive: time.Second}); err != nil {
		t.Fatal(err)
	}
	if err := hub.SetPeer(tunnel.Peer{Key: node.PublicKey(), Address: nodeIP}); err != nil {
		t.Fatal(err)
	}
	if _, err := get(hub, "http://10.77.0.2:8080/", 10*time.Second); err != nil {
		t.Fatal(err)
	}
	seen := hub.Endpoint(node.PublicKey())
	if !seen.IsValid() || hub.Endpoint(hubAddrKey(t)).IsValid() {
		t.Fatalf("endpoint: %v", seen)
	}
	// The hub restarts with the same key and port, and knows the node and
	// where it last was from its database; the node has to do nothing.
	hub.Close()
	hub, err = tunnel.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { hub.Close() })
	if err := hub.SetPeer(tunnel.Peer{Key: node.PublicKey(), Address: nodeIP, Endpoint: seen}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		got, err := get(hub, "http://10.77.0.2:8080/", 3*time.Second)
		if err == nil && got == "still here" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("node not reachable after hub restart: %v", err)
		}
	}
}

func hubAddrKey(t *testing.T) tunnel.Key {
	k, err := tunnel.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return k.Public()
}

func TestSetPeerRejectsAddresses(t *testing.T) {
	hub := open(t, hubAddr)
	key := hubAddrKey(t)
	for _, addr := range []string{"10.77.0.1", "10.78.0.2", "192.168.1.5"} {
		if hub.SetPeer(tunnel.Peer{Key: key, Address: netip.MustParseAddr(addr)}) == nil {
			t.Errorf("accepted peer address %s", addr)
		}
	}
	if _, err := tunnel.Open(tunnel.Config{Address: netip.MustParseAddr("10.1.0.1"), Network: network}); err == nil {
		t.Error("opened with an address outside the network")
	}
}

func TestKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "wireguard.key")
	k, err := tunnel.LoadKey(path)
	if err != nil {
		t.Fatal(err)
	}
	again, err := tunnel.LoadKey(path)
	if err != nil || again != k {
		t.Fatalf("key not kept: %v", err)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("key file mode: %v %v", fi.Mode(), err)
	}
	parsed, err := tunnel.ParseKey(k.Public().String())
	if err != nil || parsed != k.Public() || k.Public() == k {
		t.Fatalf("public key round trip: %v", err)
	}
	for _, bad := range []string{"", "not base64!", "AAAA"} {
		if _, err := tunnel.ParseKey(bad); err == nil {
			t.Errorf("parsed %q", bad)
		}
	}
}

func TestNextAddress(t *testing.T) {
	a := func(s string) netip.Addr { return netip.MustParseAddr(s) }
	got, err := tunnel.NextAddress(network, hubAddr, nil)
	if err != nil || got != a("10.77.0.2") {
		t.Fatalf("first: %v %v", got, err)
	}
	got, err = tunnel.NextAddress(network, hubAddr, []netip.Addr{a("10.77.0.2"), a("10.77.0.4")})
	if err != nil || got != a("10.77.0.3") {
		t.Fatalf("gap: %v %v", got, err)
	}
	small := netip.MustParsePrefix("10.77.0.0/30") // .1 hub, .2 node, .3 broadcast
	if _, err := tunnel.NextAddress(small, a("10.77.0.1"), []netip.Addr{a("10.77.0.2")}); err == nil {
		t.Fatal("handed out the broadcast address")
	}
}
