package hub

import (
	"reflect"
	"testing"

	"cozycast/internal/rights"
	"cozycast/internal/store"
)

func TestGrantAnonymous(t *testing.T) {
	f := newFixture(t, store.RoomSettings{})
	c, guest := f.join(anon("guest"))
	welcome := guest.wait(t, "welcome").(welcomeMsg)
	requireEqual(t, welcome.Rights, rights.Rights{})
	key := welcome.Self.Key
	_, other := f.join(anon("another"))
	other.wait(t, "welcome")
	f.r.SendNekoToken(f.ctx, c)
	guest.wait(t, "neko")
	if p, _ := f.fake.Member(c.ID); p.CanHost {
		t.Fatal("neko lets a guest without the remote right host")
	}

	// Given the remote: the tab is told, and neko lets it host.
	requireOK(t, f.r.GrantAnonymous(f.ctx, AnonGrant{Key: key, Remote: true}))
	requireEqual(t, guest.wait(t, "rights").(rightsMsg).Rights, rights.Rights{Remote: true})
	if p, _ := f.fake.Member(c.ID); !p.CanHost {
		t.Fatal("neko does not let the guest host")
	}
	requireEqual(t, other.count("rights"), 0)
	if got, want := f.r.AnonGrants(), []AnonGrant{{Key: key, Remote: true}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("grants %+v, want %+v", got, want)
	}

	// A second tab of theirs has it too, and one tab leaving changes nothing.
	second, tab := f.join(anon("guest"))
	requireEqual(t, tab.wait(t, "welcome").(welcomeMsg).Rights, rights.Rights{Remote: true})
	f.r.Leave(f.ctx, second)
	requireEqual(t, len(f.r.AnonGrants()), 1)

	// Changed and taken back.
	requireOK(t, f.r.GrantAnonymous(f.ctx, AnonGrant{Key: key, Upload: true}))
	requireEqual(t, guest.wait(t, "rights").(rightsMsg).Rights, rights.Rights{Upload: true})
	requireOK(t, f.r.GrantAnonymous(f.ctx, AnonGrant{Key: key}))
	requireEqual(t, guest.wait(t, "rights").(rightsMsg).Rights, rights.Rights{})
	requireEqual(t, len(f.r.AnonGrants()), 0)

	// It lasts while they are in the room: once their last tab leaves (a
	// page reload does that), it is gone when they come back.
	requireOK(t, f.r.GrantAnonymous(f.ctx, AnonGrant{Key: key, Remote: true, Upload: true}))
	requireEqual(t, guest.wait(t, "rights").(rightsMsg).Rights, rights.Rights{Remote: true, Upload: true})
	f.r.Leave(f.ctx, c)
	requireEqual(t, len(f.r.AnonGrants()), 0)
	_, again := f.join(anon("guest"))
	back := again.wait(t, "welcome").(welcomeMsg)
	requireEqual(t, back.Self.Key, key)
	requireEqual(t, back.Rights, rights.Rights{})

	// Accounts have stored permissions; someone who is not there gets nothing.
	_, alice := f.join(f.user("alice"))
	account := alice.wait(t, "welcome").(welcomeMsg).Self.Key
	requireEqual(t, f.r.GrantAnonymous(f.ctx, AnonGrant{Key: account, Remote: true}), ErrNotAnonymous)
	requireEqual(t, f.r.GrantAnonymous(f.ctx, AnonGrant{Key: "a:nobody", Remote: true}), ErrNotPresent)
	requireEqual(t, len(f.r.AnonGrants()), 0)
}
