package hub

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"cozycast/internal/store"
)

func waitRestart(t *testing.T, calls <-chan context.Context) {
	t.Helper()
	select {
	case ctx := <-calls:
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > 2*time.Minute {
			t.Fatalf("restart context deadline = (%v, %v), want two-minute timeout", deadline, ok)
		}
	case <-time.After(testTimeout):
		t.Fatal("restart callback was not invoked")
	}
}

func TestRestartDisabledAndWelcome(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			var restart func(context.Context) error
			if enabled {
				restart = func(context.Context) error { return nil }
			}
			f := newFixtureWithRestart(t, store.RoomSettings{}, restart)
			_, guest := f.join(anon("guest"))
			c, rec := f.join(f.flags(f.user("admin"), true, false))
			for _, rec := range []*recording{guest, rec} {
				welcome := rec.wait(t, "welcome").(welcomeMsg)
				requireEqual(t, welcome.Restart, enabled)
				// A disabled feature must still advertise restart:false on the wire.
				data, err := json.Marshal(welcome)
				requireOK(t, err)
				var wire map[string]json.RawMessage
				requireOK(t, json.Unmarshal(data, &wire))
				requireEqual(t, string(wire["restart"]), map[bool]string{false: "false", true: "true"}[enabled])
			}
			if !enabled {
				f.r.Handle(f.ctx, c, ClientMsg{Type: "restart"})
				expectError(t, rec, "Restarting is not enabled on this server.")
				requireEqual(t, guest.count("restarting"), 0)
				requireEqual(t, f.r.lastRestart.IsZero(), true)
			}
		})
	}
}

func TestRestartDenied(t *testing.T) {
	calls := make(chan context.Context, 1)
	f := newFixtureWithRestart(t, store.RoomSettings{}, func(ctx context.Context) error { calls <- ctx; return nil })
	guest, g := f.join(anon("guest"))
	user, u := f.join(f.user("plain"))
	for i, c := range []*Client{guest, user} {
		f.r.Handle(f.ctx, c, ClientMsg{Type: "restart"})
		expectError(t, []*recording{g, u}[i], notAllowed)
	}
	requireEqual(t, g.count("restarting"), 0)
	requireEqual(t, u.count("restarting"), 0)
	requireEqual(t, f.r.lastRestart.IsZero(), true)
	select {
	case <-calls:
		t.Fatal("unauthorized request invoked restart")
	default:
	}
}

func TestRestartTrustedCooldownAndBroadcast(t *testing.T) {
	calls := make(chan context.Context, 4)
	f := newFixtureWithRestart(t, store.RoomSettings{}, func(ctx context.Context) error { calls <- ctx; return nil }, store.RoomSettings{Name: "second"})
	alice := f.user("alice")
	bob := f.user("bob")
	f.permission(alice, store.Permission{Trusted: true})
	f.permission(bob, store.Permission{Trusted: true})
	c, a := f.join(alice)
	tab, at := f.join(alice)
	bc, b := f.join(bob)
	_, guest := f.join(anon("guest"))
	_, admin := f.join(f.flags(f.user("admin"), true, false))
	recs := []*recording{a, at, b, guest, admin}
	f.r.Handle(f.ctx, c, ClientMsg{Type: "restart"})
	for _, rec := range recs {
		msg := rec.wait(t, "restarting").(restartingMsg)
		requireEqual(t, msg.By, alice.User.Nickname)
	}
	waitRestart(t, calls)
	first := f.r.lastRestart
	for i, client := range []*Client{c, tab, bc} {
		f.r.Handle(f.ctx, client, ClientMsg{Type: "restart"})
		msg := []*recording{a, at, b}[i].wait(t, "error").(errorMsg)
		if !strings.Contains(msg.Message, "Try again in") {
			t.Fatalf("missing cooldown error: %q", msg.Message)
		}
		requireEqual(t, f.r.lastRestart, first)
	}
	for _, rec := range recs {
		requireEqual(t, rec.count("restarting"), 1)
	}

	// The cooldown belongs to the room, so a different room is still allowed.
	second := f.h.Room("second")
	p := store.Permission{Room: "second", UserID: alice.User.ID, Trusted: true}
	requireOK(t, f.s.SavePermission(f.ctx, p))
	secondRec := newRecording()
	secondClient, err := second.Join(f.ctx, JoinRequest{Identity: alice, Send: secondRec.send, Kill: secondRec.kill})
	requireOK(t, err)
	t.Cleanup(func() { second.Leave(context.Background(), secondClient) })
	second.Handle(f.ctx, secondClient, ClientMsg{Type: "restart"})
	requireEqual(t, secondRec.wait(t, "restarting").(restartingMsg).By, alice.User.Nickname)
	waitRestart(t, calls)

	f.r.mu.Lock()
	f.r.lastRestart = time.Now().Add(-RestartCooldown - time.Second)
	f.r.mu.Unlock()
	f.r.Handle(f.ctx, bc, ClientMsg{Type: "restart"})
	for _, rec := range recs {
		requireEqual(t, rec.wait(t, "restarting").(restartingMsg).By, bob.User.Nickname)
	}
	waitRestart(t, calls)
}

func TestRestartAdminBypassesCooldown(t *testing.T) {
	calls := make(chan context.Context, 4)
	f := newFixtureWithRestart(t, store.RoomSettings{}, func(ctx context.Context) error { calls <- ctx; return nil })
	admin := f.flags(f.user("admin"), true, false)
	ac, a := f.join(admin)
	trusted := f.user("trusted")
	f.permission(trusted, store.Permission{Trusted: true})
	c, rec := f.join(trusted)
	f.r.Handle(f.ctx, c, ClientMsg{Type: "restart"})
	waitRestart(t, calls)
	for range 2 {
		f.r.Handle(f.ctx, ac, ClientMsg{Type: "restart"})
		waitRestart(t, calls)
	}
	requireEqual(t, a.count("restarting"), 3)
	requireEqual(t, rec.count("restarting"), 3)
	f.r.Handle(f.ctx, c, ClientMsg{Type: "restart"})
	if msg := rec.wait(t, "error").(errorMsg).Message; !strings.Contains(msg, "Try again in") {
		t.Fatalf("admin restart did not impose trusted cooldown: %q", msg)
	}
	msgs := a.snapshot()
	var by []string
	for _, msg := range msgs {
		if msg, ok := msg.(restartingMsg); ok {
			by = append(by, msg.By)
		}
	}
	requireEqual(t, strings.Join(by, ","), trusted.User.Nickname+","+admin.User.Nickname+","+admin.User.Nickname)
}
