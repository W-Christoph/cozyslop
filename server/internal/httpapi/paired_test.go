package httpapi_test

import (
	"context"
	"net/http/httptest"
	"net/netip"
	"testing"

	"cozycast/internal/auth"
	"cozycast/internal/httpapi"
	"cozycast/internal/hub"
	"cozycast/internal/neko"
	"cozycast/internal/store"
	"cozycast/internal/tunnel"
)

func TestPairedRoomAdmin(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	st, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	hubKey, _ := tunnel.GenerateKey()
	tun, err := tunnel.Open(tunnel.Config{PrivateKey: hubKey, Address: netip.MustParseAddr("10.77.0.1"), Network: netip.MustParsePrefix("10.77.0.0/24")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tun.Close() })
	nodeKey, _ := tunnel.GenerateKey()
	paired := store.RegisteredRoom{Name: "home", NekoURL: "http://10.77.0.2:8080", NekoToken: "secret",
		NodeKey: nodeKey.Public().String(), TunnelAddress: "10.77.0.2"}
	if err := st.CreateRegisteredRoom(ctx, &paired); err != nil {
		t.Fatal(err)
	}
	if err := tun.SetPeer(tunnel.Peer{Key: nodeKey.Public(), Address: netip.MustParseAddr("10.77.0.2")}); err != nil {
		t.Fatal(err)
	}
	local, err := neko.NewClient("http://127.0.0.1:1", "x", nil)
	if err != nil {
		t.Fatal(err)
	}
	home, err := hub.BuildRoomConfig("home", paired.NekoURL, paired.NekoToken, "", tun.DialContext)
	if err != nil {
		t.Fatal(err)
	}
	home.Source = "paired"
	h := hub.New(st, t.TempDir(), []hub.RoomConfig{{Name: "default", Source: "configured", Neko: local}, home})
	if err := h.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		for _, r := range h.Rooms() {
			h.Remove(r.Name, "not_found")
		}
	})
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{Store: st, Auth: auth.New(st, false), Hub: h, Tunnel: tun}).Handler())
	t.Cleanup(srv.Close)
	a := &apiTest{t: t, st: st, srv: srv, h: h}
	a.user("root", true)
	admin := a.login("root")

	var list []struct{ Name, Source string }
	a.call(admin, "GET", "/api/admin/rooms", nil, 200, &list)
	if len(list) != 2 || list[1].Name != "home" || list[1].Source != "paired" {
		t.Fatalf("list: %+v", list)
	}
	// Tunnel addresses are only handed out by pairing.
	a.error(admin, "POST", "/api/admin/rooms", map[string]string{"name": "sneaky", "nekoUrl": "http://10.77.0.9:8080"}, 400,
		"Addresses in 10.77.0.0/24 are for paired rooms.")
	const pairedOnly = "This room is connected by pairing. Remove it and pair the computer again."
	a.error(admin, "PATCH", "/api/admin/rooms/home", map[string]string{"nekoUrl": "http://elsewhere:8080"}, 409, pairedOnly)
	a.error(admin, "POST", "/api/admin/rooms/home/token", nil, 409, pairedOnly)

	if !tun.HasPeer(nodeKey.Public()) {
		t.Fatal("peer missing before removal")
	}
	a.call(admin, "DELETE", "/api/admin/rooms/home", nil, 204, nil)
	if tun.HasPeer(nodeKey.Public()) {
		t.Fatal("removed room's node still has a tunnel")
	}
	if _, err := st.RegisteredRoom(ctx, "home"); err == nil {
		t.Fatal("registration kept")
	}
}
