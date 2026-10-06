package hub

import (
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"cozycast/internal/neko"
	"cozycast/internal/neko/nekotest"
	"cozycast/internal/store"
)

func TestDesktopOfflineAndBack(t *testing.T) {
	grace := offlineGrace
	offlineGrace = 200 * time.Millisecond
	t.Cleanup(func() { offlineGrace = grace })
	f := newFixture(t, store.RoomSettings{})

	// The room's neko sits behind a link that can be cut, like a home PC
	// behind its tunnel.
	fake := nekotest.New(t, "home-secret")
	target, err := url.Parse(fake.URL())
	requireOK(t, err)
	forward := httputil.NewSingleHostReverseProxy(target)
	var down atomic.Bool
	link := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			http.Error(w, "unreachable", http.StatusBadGateway)
			return
		}
		forward.ServeHTTP(w, r)
	}))
	t.Cleanup(link.Close)
	nc, err := neko.NewClient(link.URL, "home-secret", nil)
	requireOK(t, err)
	requireOK(t, f.h.Add(f.ctx, RoomConfig{Name: "home", Source: "registered", Neko: nc}))
	r := f.h.Room("home")
	t.Cleanup(func() { f.h.Remove("home", "not_found") })
	waitCtx, cancel := observerDeadline()
	defer cancel()
	requireOK(t, fake.WaitObservers(waitCtx, 1))

	rec := newRecording()
	_, err = r.Join(f.ctx, JoinRequest{Identity: anon("viewer"), Send: rec.send, Kill: rec.kill})
	requireOK(t, err)
	requireEqual(t, rec.wait(t, "welcome").(welcomeMsg).Desktop, "online")

	down.Store(true)
	fake.Restart() // drops the observer; reconnecting fails while the link is down
	requireEqual(t, rec.wait(t, "desktop").(desktopMsg).State, "offline")
	if r.OfflineSince().IsZero() {
		t.Fatal("offline room reports no offline time")
	}

	// A tab joining now is told at once and gets no neko errors.
	late := newRecording()
	c, err := r.Join(f.ctx, JoinRequest{Identity: anon("late"), Send: late.send, Kill: late.kill})
	requireOK(t, err)
	requireEqual(t, late.wait(t, "welcome").(welcomeMsg).Desktop, "offline")
	r.SendNekoToken(f.ctx, c)
	requireEqual(t, late.wait(t, "desktop").(desktopMsg).State, "offline")
	requireEqual(t, late.count("neko_unavailable"), 0)

	down.Store(false)
	requireEqual(t, rec.wait(t, "desktop").(desktopMsg).State, "online")
	requireEqual(t, late.wait(t, "desktop").(desktopMsg).State, "online")
	requireEqual(t, r.OfflineSince().IsZero(), true)
	_, err = r.NekoToken(f.ctx, c)
	requireOK(t, err)
}

func TestDesktopShortDropStaysSilent(t *testing.T) {
	grace := offlineGrace
	offlineGrace = 100 * time.Millisecond
	t.Cleanup(func() { offlineGrace = grace })
	f := newFixture(t, store.RoomSettings{})
	rec := newRecording()
	_, err := f.r.Join(f.ctx, JoinRequest{Identity: anon("viewer"), Send: rec.send, Kill: rec.kill})
	requireOK(t, err)
	rec.wait(t, "welcome")
	f.r.desktopLost()
	f.r.desktopFound()
	time.Sleep(3 * offlineGrace)
	requireEqual(t, rec.count("desktop"), 0)
	requireEqual(t, f.r.OfflineSince().IsZero(), true)
}
