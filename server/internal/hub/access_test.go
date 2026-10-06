package hub

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"cozycast/internal/auth"
	"cozycast/internal/neko"
	"cozycast/internal/neko/nekotest"
	"cozycast/internal/rights"
	"cozycast/internal/store"
)

func TestAdmission(t *testing.T) {
	// Columns: anonymous, account, verified, invited, trusted, admin.
	modes := []struct {
		mode    string
		allowed [6]bool
	}{
		{"public", [6]bool{true, true, true, true, true, true}},
		{"account", [6]bool{false, true, true, true, true, true}},
		{"verified", [6]bool{false, false, true, true, false, true}},
		{"invite", [6]bool{false, false, false, true, true, true}},
	}
	for _, mode := range modes {
		t.Run(mode.mode, func(t *testing.T) {
			f := newFixture(t, store.RoomSettings{Access: mode.mode})
			ids := []auth.Identity{anon("anon"), f.user("account"), f.user("verified"), f.user("invited"), f.user("trusted"), f.user("admin")}
			ids[2] = f.flags(ids[2], false, true)
			ids[5] = f.flags(ids[5], true, false)
			f.permission(ids[3], store.Permission{Invited: true})
			f.permission(ids[4], store.Permission{Trusted: true})
			for i, id := range ids {
				t.Run([]string{"anonymous", "account", "verified", "invited", "trusted", "admin"}[i], func(t *testing.T) {
					if !mode.allowed[i] {
						assertDenied(t, f.r, f.ctx, id, "", mode.mode, nil)
						return
					}
					_, rec := f.join(id)
					requireEqual(t, rec.wait(t, "welcome").(welcomeMsg).Self.Key, id.Key())
				})
			}
		})
	}
}

func TestAccountBans(t *testing.T) {
	now := time.Now().Unix()
	future, past := now+3600, now-3600
	for _, tc := range []struct {
		name    string
		until   *int64
		allowed bool
	}{{"timed", &future, false}, {"expired", &past, true}, {"forever", nil, false}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, store.RoomSettings{})
			id := f.flags(f.user("admin"), true, true)
			f.permission(id, store.Permission{Trusted: true, Invited: true, Banned: true, BannedUntil: tc.until})
			if tc.allowed {
				f.join(id)
			} else {
				assertDenied(t, f.r, f.ctx, id, "", "banned", tc.until)
			}
		})
	}
}
func TestAnonymousBans(t *testing.T) {
	future, past := time.Now().Unix()+3600, time.Now().Unix()-3600
	for _, tc := range []struct {
		name, anonID, ip string
		until            *int64
		denied           bool
	}{
		{"id", "blocked", "203.0.113.9", &future, true}, {"ip", "new-cookie", "192.0.2.1", nil, true},
		{"expired", "blocked", "192.0.2.1", &past, false}, {"unrelated", "other", "203.0.113.9", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, store.RoomSettings{})
			requireOK(t, f.s.AddAnonBan(f.ctx, f.r.Name, "blocked", "192.0.2.1", tc.until))
			id := auth.Identity{AnonID: tc.anonID, IP: tc.ip}
			if tc.denied {
				assertDenied(t, f.r, f.ctx, id, "", "banned", tc.until)
			} else {
				f.join(id)
			}
		})
	}
}

func TestTemporaryInvites(t *testing.T) {
	f := newFixture(t, store.RoomSettings{Access: "invite"}, store.RoomSettings{Name: "other", Access: "invite"})
	max := 2
	inv := &store.Invite{Room: f.r.Name, Temporary: true, Remote: true, Image: true, Upload: true, MaxUses: &max}
	requireOK(t, f.s.CreateInvite(f.ctx, inv))
	requireOK(t, f.s.AddAnonBan(f.ctx, f.r.Name, "banned", "203.0.113.1", nil))
	assertDenied(t, f.r, f.ctx, auth.Identity{AnonID: "banned", IP: "203.0.113.1"}, inv.Code, "banned", nil)
	checkUses := func(want int) {
		t.Helper()
		got, err := f.s.Invite(f.ctx, inv.Code)
		requireOK(t, err)
		requireEqual(t, got.Uses, want)
	}
	checkUses(0)
	// A second live room on the same hub/store proves grants cannot cross rooms.
	other := f.h.Room("other")
	id := f.user("invited")
	assertDenied(t, other, f.ctx, id, inv.Code, "invite", nil)
	checkUses(0)
	otherSet := store.RoomSettings{Name: "other", Access: "public"}
	requireOK(t, f.s.SaveRoomSettings(f.ctx, otherSet))
	f.h.RoomSettingsChanged(f.ctx, "other")
	r := newRecording()
	oc, err := other.Join(f.ctx, JoinRequest{Identity: id, AccessCode: inv.Code, Send: r.send, Kill: r.kill})
	requireOK(t, err)
	defer other.Leave(f.ctx, oc)
	requireEqual(t, r.wait(t, "welcome").(welcomeMsg).Rights, rights.Rights{})
	checkUses(0)
	c, rec := f.join(id, inv.Code)
	requireEqual(t, rec.wait(t, "welcome").(welcomeMsg).Rights, rights.Rights{Remote: true, Image: true, Upload: true})
	checkUses(1)
	// Each successful tab join consumes one use, even for an existing identity.
	tab, tabRec := f.join(id, inv.Code)
	tabRec.wait(t, "welcome")
	checkUses(2)
	assertDenied(t, f.r, f.ctx, anon("exhausted"), inv.Code, "invite", nil)
	checkUses(2)
	f.r.Leave(f.ctx, c)
	f.r.Leave(f.ctx, tab)
	assertDenied(t, f.r, f.ctx, id, inv.Code, "invite", nil)
	checkUses(2)

	// Anonymous holders get remote/upload but never image rights.
	anonInvite := &store.Invite{Room: f.r.Name, Temporary: true, Remote: true, Image: true, Upload: true}
	requireOK(t, f.s.CreateInvite(f.ctx, anonInvite))
	_, a := f.join(anon("temporary"), anonInvite.Code)
	requireEqual(t, a.wait(t, "welcome").(welcomeMsg).Rights, rights.Rights{Remote: true, Upload: true})
	for _, kind := range []string{"expired", "permanent", "unknown"} {
		code := "missing"
		if kind != "unknown" {
			past := time.Now().Unix() - 3600
			i := &store.Invite{Room: f.r.Name, Temporary: kind == "expired"}
			if kind == "expired" {
				i.ExpiresAt = &past
			}
			requireOK(t, f.s.CreateInvite(f.ctx, i))
			code = i.Code
		}
		assertDenied(t, f.r, f.ctx, anon(kind), code, "invite", nil)
	}
	set := f.r.Settings()
	set.Access = "public"
	requireOK(t, f.s.SaveRoomSettings(f.ctx, set))
	f.h.RoomSettingsChanged(f.ctx, f.r.Name)
	_, exhausted := f.join(anon("public-exhausted"), inv.Code)
	requireEqual(t, exhausted.wait(t, "welcome").(welcomeMsg).Rights, rights.Rights{})
	checkUses(2)

}

func TestPermissionsChanged(t *testing.T) {
	f := newFixture(t, store.RoomSettings{})
	id := f.user("alice")
	c, a := f.join(id)
	tab, b := f.join(id)
	f.r.SendNekoToken(f.ctx, c)
	a.wait(t, "neko")
	f.r.SendNekoToken(f.ctx, tab)
	b.wait(t, "neko")
	for _, p := range []store.Permission{{Remote: true, Image: true, Upload: true}, {}} {
		f.permission(id, p)
		f.h.PermissionsChanged(f.ctx, f.r.Name, id.User.ID)
		want := rights.Rights{Remote: p.Remote, Image: p.Image, Upload: p.Upload}
		requireEqual(t, a.wait(t, "rights").(rightsMsg).Rights, want)
		requireEqual(t, b.wait(t, "rights").(rightsMsg).Rights, want)
		assertProfile(t, f.fake, c, id.User.Nickname, want)
		assertProfile(t, f.fake, tab, id.User.Nickname, want)
	}
	// Notifications for missing rooms/accounts must be harmless.
	f.h.PermissionsChanged(f.ctx, "missing", id.User.ID)
	f.h.UserChanged(f.ctx, 9999)
}
func TestLiveAdmissionRevocation(t *testing.T) {
	for _, reason := range []string{"invite", "verified", "banned"} {
		t.Run(reason, func(t *testing.T) {
			access := reason
			if reason == "banned" {
				access = "public"
			}
			f := newFixture(t, store.RoomSettings{Access: access})
			id := f.flags(f.user("alice"), false, true)
			f.permission(id, store.Permission{Invited: true})
			_, a := f.join(id)
			_, b := f.join(id)
			var until *int64
			if reason == "banned" {
				u := time.Now().Unix() + 3600
				until = &u
				f.permission(id, store.Permission{Banned: true, BannedUntil: until})
			} else {
				f.permission(id, store.Permission{})
				if reason == "verified" {
					requireOK(t, f.s.UpdateFlags(f.ctx, id.User.ID, false, false, false))
				}
			}
			f.h.PermissionsChanged(f.ctx, f.r.Name, id.User.ID)
			for _, rec := range []*recording{a, b} {
				m := rec.wait(t, "kicked").(kickedMsg)
				requireEqual(t, m.Reason, reason)
				if !reflect.DeepEqual(m.BannedUntil, until) {
					t.Fatal("ban expiry lost")
				}
				rec.assertKills(t, 1)
			}
		})
	}
}
func TestUserChanged(t *testing.T) {
	for _, action := range []string{"disabled", "deleted", "profile", "admin"} {
		t.Run(action, func(t *testing.T) {
			f := newFixture(t, store.RoomSettings{})
			id := f.user("alice")
			c, a := f.join(id)
			_, b := f.join(id)
			_, watch := f.join(anon("watcher"))
			f.r.SendNekoToken(f.ctx, c)
			a.wait(t, "neko")
			switch action {
			case "disabled":
				requireOK(t, f.s.UpdateFlags(f.ctx, id.User.ID, false, false, true))
			case "deleted":
				requireOK(t, f.s.DeleteUser(f.ctx, id.User.ID))
			case "profile":
				requireOK(t, f.s.UpdateProfile(f.ctx, id.User.ID, "New Name", "#abcdef"))
				requireOK(t, f.s.UpdateAvatar(f.ctx, id.User.ID, "new.png"))
			case "admin":
				requireOK(t, f.s.UpdateFlags(f.ctx, id.User.ID, true, false, false))
			}
			f.h.UserChanged(f.ctx, id.User.ID)
			if action == "disabled" || action == "deleted" {
				for _, rec := range []*recording{a, b} {
					requireEqual(t, rec.wait(t, "kicked").(kickedMsg).Reason, "deleted")
					rec.assertKills(t, 1)
				}
				return
			}
			for _, rec := range []*recording{a, b, watch} {
				m := rec.wait(t, "user_updated").(userMsg)
				requireEqual(t, m.User.Key, id.Key())
				if action == "profile" {
					requireEqual(t, m.User.Nickname, "New Name")
					requireEqual(t, m.User.NameColor, "#abcdef")
					requireEqual(t, m.User.AvatarURL, "/media/avatars/new.png")
				} else {
					requireEqual(t, m.User.Admin, true)
				}
			}
			name := id.User.Nickname
			rt := rights.Rights{}
			if action == "profile" {
				name = "New Name"
			} else {
				rt = rights.Rights{Admin: true, Trusted: true, Remote: true, Image: true, Upload: true}
				requireEqual(t, a.wait(t, "rights").(rightsMsg).Rights, rt)
				requireEqual(t, b.wait(t, "rights").(rightsMsg).Rights, rt)
			}
			assertProfile(t, f.fake, c, name, rt)
		})
	}
}
func TestRoomSettingsChanged(t *testing.T) {
	f := newFixture(t, store.RoomSettings{})
	id := f.user("alice")
	c, a := f.join(id)
	guest, b := f.join(anon("guest"))
	_ = guest
	f.r.SendNekoToken(f.ctx, c)
	a.wait(t, "neko")
	set := store.RoomSettings{Name: f.r.Name, Access: "public", Hidden: true, RemoteOwnership: true, DefaultRemote: true, DefaultImage: true, DefaultUpload: true}
	requireOK(t, f.s.SaveRoomSettings(f.ctx, set))
	f.h.RoomSettingsChanged(f.ctx, f.r.Name)
	for _, rec := range []*recording{a, b} {
		requireEqual(t, rec.wait(t, "room_settings").(settingsMsg).Settings, set)
	}
	requireEqual(t, a.wait(t, "rights").(rightsMsg).Rights, rights.Rights{Remote: true, Image: true, Upload: true})
	requireEqual(t, b.wait(t, "rights").(rightsMsg).Rights, rights.Rights{Remote: true, Upload: true})
	requireEqual(t, f.fake.ImplicitHosting(), false)
	assertProfile(t, f.fake, c, id.User.Nickname, rights.Rights{Remote: true, Image: true, Upload: true})
	set.Access = "account"
	set.RemoteOwnership = false
	set.DefaultRemote = false
	set.DefaultImage = false
	set.DefaultUpload = false
	requireOK(t, f.s.SaveRoomSettings(f.ctx, set))
	f.h.RoomSettingsChanged(f.ctx, f.r.Name)
	requireEqual(t, a.wait(t, "room_settings").(settingsMsg).Settings, set)
	requireEqual(t, a.wait(t, "rights").(rightsMsg).Rights, rights.Rights{})
	requireEqual(t, b.wait(t, "room_settings").(settingsMsg).Settings, set)
	requireEqual(t, b.wait(t, "kicked").(kickedMsg).Reason, "account")
	b.assertKills(t, 1)
	requireEqual(t, f.fake.ImplicitHosting(), true)
	assertProfile(t, f.fake, c, id.User.Nickname, rights.Rights{})
	requireEqual(t, f.r.Settings(), set)
}
func TestModeration(t *testing.T) {
	for _, account := range []bool{false, true} {
		t.Run(fmt.Sprint(account), func(t *testing.T) {
			f := newFixture(t, store.RoomSettings{})
			id := anon("target")
			if account {
				id = f.user("target")
				f.permission(id, store.Permission{Remote: true})
			}
			c, a := f.join(id)
			tab, b := f.join(id)
			until := time.Now().Unix() + 3600
			requireOK(t, f.r.Ban(f.ctx, id.Key(), &until))
			for _, rec := range []*recording{a, b} {
				m := rec.wait(t, "kicked").(kickedMsg)
				requireEqual(t, m.Reason, "banned")
				requireEqual(t, *m.BannedUntil, until)
				rec.assertKills(t, 1)
			}
			if account {
				p, err := f.s.Permission(f.ctx, f.r.Name, id.User.ID)
				requireOK(t, err)
				requireEqual(t, p.Banned, true)
				requireEqual(t, *p.BannedUntil, until)
				requireEqual(t, p.Remote, true)
			} else {
				for _, probe := range []auth.Identity{{AnonID: id.AnonID, IP: "203.0.113.1"}, {AnonID: "new-browser", IP: id.IP}} {
					ban, err := f.s.AnonBan(f.ctx, f.r.Name, probe.AnonID, probe.IP)
					requireOK(t, err)
					requireEqual(t, ban.AnonID, id.AnonID)
					requireEqual(t, ban.IP, id.IP)
					requireEqual(t, *ban.BannedUntil, until)
				}
			}
			f.r.Leave(f.ctx, c)
			f.r.Leave(f.ctx, tab)
			assertDenied(t, f.r, f.ctx, id, "", "banned", &until)
			requireEqual(t, errors.Is(f.r.Ban(f.ctx, "missing", nil), ErrNotPresent), true)
			requireEqual(t, errors.Is(f.r.Kick("missing"), ErrNotPresent), true)
		})
	}
	t.Run("kick permits rejoin", func(t *testing.T) {
		f := newFixture(t, store.RoomSettings{})
		id := anon("target")
		c, a := f.join(id)
		tab, b := f.join(id)
		requireOK(t, f.r.Kick(id.Key()))
		for _, rec := range []*recording{a, b} {
			requireEqual(t, rec.wait(t, "kicked").(kickedMsg).Reason, "kicked")
			rec.assertKills(t, 1)
		}
		f.r.Leave(f.ctx, c)
		f.r.Leave(f.ctx, tab)
		f.join(id)
	})
}

// Use a background deadline when testing cancellation: a canceled hub context
// cannot also serve as the deadline for waiting for its observers to disconnect.
func observerDeadline() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), testTimeout)
}

func TestKickedTabIsGoneAtOnce(t *testing.T) {
	f := newFixture(t, store.RoomSettings{})
	id := f.flags(f.user("root"), true, true)
	c, rec := f.join(id)
	_, watch := f.join(anon("watcher"))
	rec.wait(t, "welcome")
	f.r.SendNekoToken(f.ctx, c)
	rec.wait(t, "neko")

	// The socket has not closed yet (no Leave), as while it drains.
	requireOK(t, f.r.Kick(id.Key()))
	rec.wait(t, "kicked")
	rec.assertKills(t, 1)
	requireEqual(t, f.r.UserCount(), 1)
	requireEqual(t, watch.wait(t, "user_left").(userLeftMsg).Key, id.Key())

	sent := len(rec.snapshot())
	for _, msg := range []ClientMsg{
		{Type: "chat_send", Body: "still here"}, {Type: "whisper", To: "a:watcher", Body: "psst"},
		{Type: "remote_reset"}, {Type: "restart"}, {Type: "typing", Typing: true}, {Type: "neko_token"}, {Type: "unknown"},
	} {
		f.r.Handle(f.ctx, c, msg)
	}
	requireEqual(t, len(rec.snapshot()), sent)
	requireEqual(t, watch.count("chat")+watch.count("typing")+watch.count("restarting"), 0)
	requireEqual(t, errors.Is(f.r.PostMedia(f.ctx, id.Key(), "image", "none"), ErrNotPresent), true)

	// Once the socket is closed the tab cannot get a neko member back.
	f.r.Leave(f.ctx, c)
	if _, ok := f.fake.Member(c.ID); ok {
		t.Fatal("neko member survived Leave")
	}
	_, err := f.r.NekoToken(f.ctx, c)
	requireEqual(t, errors.Is(err, ErrNotPresent), true)
	if _, ok := f.fake.Member(c.ID); ok {
		t.Fatal("neko member recreated after Leave")
	}
}

func TestStartLoadsSettingsBeforeAdmission(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := store.Open(ctx, ":memory:")
	requireOK(t, err)
	requireOK(t, s.SaveRoomSettings(ctx, store.RoomSettings{Name: "main", Access: "invite"}))
	fake := nekotest.New(t, "secret")
	nc, err := neko.NewClient(fake.URL(), "secret", nil)
	requireOK(t, err)

	// Settings are in place when Start returns, not some time later.
	h := New(s, t.TempDir(), []RoomConfig{{Name: "main", Neko: nc}})
	requireOK(t, h.Start(ctx))
	assertDenied(t, h.Room("main"), ctx, anon("guest"), "", "invite", nil)
	cancel()
	waitCtx, stop := observerDeadline()
	defer stop()
	requireOK(t, fake.WaitObservers(waitCtx, 0))

	// A server that cannot read its settings does not start.
	requireOK(t, s.Close())
	if err := New(s, t.TempDir(), []RoomConfig{{Name: "main", Neko: nc}}).Start(context.Background()); err == nil {
		t.Fatal("Start succeeded without settings")
	}
}

func TestCheckMembers(t *testing.T) {
	f := newFixture(t, store.RoomSettings{})
	id := f.user("alice")
	c, rec := f.join(id)
	f.r.SendNekoToken(f.ctx, c)
	rec.wait(t, "neko")
	_, idle := f.join(anon("no neko member yet"))
	idle.wait(t, "welcome")

	// Nothing to do: neko is left alone.
	before := len(f.fake.Calls())
	f.r.checkMembers(f.ctx)
	if calls := f.fake.Calls()[before:]; len(calls) != 1 || calls[0].Method != "GET" {
		t.Fatalf("calls for a room in order: %+v", calls)
	}

	// Someone with neko's admin token adds an admin and gives a tab the remote.
	requireOK(t, f.r.neko.CreateMember(f.ctx, "rogue", "pw", neko.Profile{Name: "rogue", IsAdmin: true, CanLogin: true, CanConnect: true, CanHost: true}))
	raised := f.r.nekoProfile(c)
	raised.CanHost, raised.IsAdmin = true, true
	raised.Plugins["filetransfer.enabled"] = true
	requireOK(t, f.r.neko.UpdateProfile(f.ctx, c.ID, raised))
	f.r.checkMembers(f.ctx)
	if _, ok := f.fake.Member("rogue"); ok {
		t.Fatal("unknown neko member survived")
	}
	assertProfile(t, f.fake, c, id.User.Nickname, rights.Rights{})
	if p, ok := f.fake.Member(neko.ObserverID); !ok || !p.IsAdmin {
		t.Fatal("observer was touched")
	}

	// A kicked tab's member goes even before its socket has closed.
	requireOK(t, f.r.Kick(id.Key()))
	f.r.checkMembers(f.ctx)
	if _, ok := f.fake.Member(c.ID); ok {
		t.Fatal("kicked tab kept its neko member")
	}
}
