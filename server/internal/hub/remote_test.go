package hub

import (
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
