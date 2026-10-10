package hub

import (
	"cozycast/internal/neko"
	"cozycast/internal/store"
	"testing"
)

func TestRemoteHostAndReset(t *testing.T) {
	f := newFixture(t, store.RoomSettings{})
	owner := f.user("owner")
	admin := f.flags(f.user("admin"), true, false)
	c, a := f.join(owner)
	tab, b := f.join(owner)
	ac, mod := f.join(admin)
	_, watch := f.join(anon("watcher"))
	recs := []*recording{a, b, mod, watch}
	for _, host := range []*Client{c, tab} {
		f.fake.SetHost(host.ID)
		for _, rec := range recs {
			m := rec.wait(t, "remote").(remoteMsg)
			if m.Holder == nil {
				t.Fatal("remote holder missing")
			}
			requireEqual(t, *m.Holder, owner.Key())
		}
	}
	f.r.Handle(f.ctx, c, ClientMsg{Type: "remote_reset"})
	expectError(t, a, notAllowed)
	for _, call := range f.fake.Calls() {
		if call.Path == "/api/room/control/reset" {
			t.Fatal("non-admin reset reached neko")
		}
	}
	f.r.Handle(f.ctx, ac, ClientMsg{Type: "remote_reset"})
	for _, rec := range recs {
		requireEqual(t, rec.wait(t, "remote").(remoteMsg).Holder, (*string)(nil))
	}
	resets := 0
	for _, call := range f.fake.Calls() {
		if call.Path == "/api/room/control/reset" {
			resets++
		}
	}
	requireEqual(t, resets, 1)
	f.fake.SetHost(c.ID)
	for _, rec := range recs {
		rec.wait(t, "remote")
	}
	f.fake.ClearHost()
	for _, rec := range recs {
		requireEqual(t, rec.wait(t, "remote").(remoteMsg).Holder, (*string)(nil))
	}
}

// Input over the WebSocket only reaches neko from the tab that holds the
// remote, as the hub knows it from neko or from what it just passed on.
func TestNekoControl(t *testing.T) {
	f := newFixture(t, store.RoomSettings{DefaultRemote: true})
	a, ra := f.join(f.user("first"))
	b, rb := f.join(f.user("second"))
	connA, connB := f.nekoConn(a), f.nekoConn(b)
	input := func(conn *NekoConn) bool { return f.r.NekoControl(conn, neko.ControlInput) }
	reported := func(id string) {
		t.Helper()
		if id == "" {
			f.fake.ClearHost()
		} else {
			f.fake.SetHost(id)
		}
		ra.wait(t, "remote")
		rb.wait(t, "remote")
	}

	requireEqual(t, f.r.NekoControl(&NekoConn{}, neko.ControlRequest), false) // not attached
	requireEqual(t, input(connA), false)                                      // would take the free remote
	requireEqual(t, f.r.NekoControl(connA, neko.ControlRequest), true)
	requireEqual(t, input(connA), true) // before neko reports it
	reported(a.ID)
	requireEqual(t, input(connA), true)
	requireEqual(t, input(connB), false)

	// Taken over: what the first tab still sends must not take it back.
	requireEqual(t, f.r.NekoControl(connB, neko.ControlRequest), true)
	requireEqual(t, input(connA), false)
	requireEqual(t, input(connB), true)
	reported(b.ID)
	requireEqual(t, input(connA), false)
	requireEqual(t, input(connB), true)

	// Dropped: the tab's own input must not pick it up again.
	requireEqual(t, f.r.NekoControl(connA, neko.ControlRelease), true) // not the holder's: changes nothing
	requireEqual(t, input(connB), true)
	requireEqual(t, f.r.NekoControl(connB, neko.ControlRelease), true)
	requireEqual(t, input(connB), false)
	reported("")
	requireEqual(t, input(connB), false)

	// A request neko never grants stops counting.
	old := remoteSettle
	remoteSettle = 0
	defer func() { remoteSettle = old }()
	reported(a.ID)
	requireEqual(t, f.r.NekoControl(connB, neko.ControlRequest), true)
	requireEqual(t, input(connA), true)
	requireEqual(t, input(connB), false)
}

// With remote ownership a request for a held remote changes nothing.
func TestNekoControlOwnership(t *testing.T) {
	f := newFixture(t, store.RoomSettings{DefaultRemote: true, RemoteOwnership: true})
	a, ra := f.join(f.user("first"))
	b, rb := f.join(f.user("second"))
	connA, connB := f.nekoConn(a), f.nekoConn(b)
	f.fake.SetHost(a.ID)
	ra.wait(t, "remote")
	rb.wait(t, "remote")
	requireEqual(t, f.r.NekoControl(connB, neko.ControlRequest), true)
	requireEqual(t, f.r.NekoControl(connA, neko.ControlInput), true)
	requireEqual(t, f.r.NekoControl(connB, neko.ControlInput), false)
}

// nekoConn is a tab's connection to neko, attached as the proxy does it.
func (f *fixture) nekoConn(c *Client) *NekoConn {
	f.t.Helper()
	token, err := f.r.NekoToken(f.ctx, c)
	requireOK(f.t, err)
	conn := &NekoConn{Send: func([]byte) {}, Close: func() {}}
	detach, ok := f.r.AttachNeko(token, conn)
	requireEqual(f.t, ok, true)
	f.t.Cleanup(detach)
	return conn
}
