package hub

import (
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync"
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

// A desktop that stops answering (a home PC that lost power: nothing closes
// its connections) holds up nothing but the requests meant for it.
func TestHangingDesktopDoesNotHoldUpChat(t *testing.T) {
	f := newFixture(t, store.RoomSettings{})
	fake := nekotest.New(t, "home-secret")
	target, err := url.Parse(fake.URL())
	requireOK(t, err)
	forward := httputil.NewSingleHostReverseProxy(target)
	var hanging atomic.Bool
	var logins atomic.Int32
	release := make(chan struct{})
	answer := sync.OnceFunc(func() { close(release) })
	link := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hanging.Load() {
			if r.URL.Path == "/api/login" {
				logins.Add(1)
			}
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		forward.ServeHTTP(w, r)
	}))
	t.Cleanup(link.Close)
	t.Cleanup(answer)
	nc, err := neko.NewClient(link.URL, "home-secret", nil)
	requireOK(t, err)
	requireOK(t, f.h.Add(f.ctx, RoomConfig{Name: "home", Source: "registered", Neko: nc}))
	r := f.h.Room("home")
	t.Cleanup(func() { f.h.Remove("home", "not_found") })
	waitCtx, cancel := observerDeadline()
	defer cancel()
	requireOK(t, fake.WaitObservers(waitCtx, 1))

	rec := newRecording()
	c, err := r.Join(f.ctx, JoinRequest{Identity: anon("viewer"), Send: rec.send, Kill: rec.kill})
	requireOK(t, err)
	r.Handle(f.ctx, c, ClientMsg{Type: "neko_token"})
	rec.wait(t, "neko")

	hanging.Store(true)
	handled := make(chan struct{})
	go func() {
		defer close(handled)
		// The second request finds the first one waiting and is dropped.
		r.Handle(f.ctx, c, ClientMsg{Type: "neko_token"})
		r.Handle(f.ctx, c, ClientMsg{Type: "neko_token"})
		r.Handle(f.ctx, c, ClientMsg{Type: "file_delete", Name: "a.mp4"})
		r.Handle(f.ctx, c, ClientMsg{Type: "chat_send", Body: "still here"})
	}()
	select {
	case <-handled:
	case <-time.After(testTimeout):
		t.Fatal("a message waited for the desktop")
	}
	requireEqual(t, rec.wait(t, "chat").(chatMsg).Message.Body, "still here")
	requireEqual(t, rec.count("neko"), 1) // the one from before

	answer()
	rec.wait(t, "neko")
	requireEqual(t, logins.Load(), 1)
}

// While neko is gone and nobody knows yet for how long, a token request
// waits instead of asking it.
func TestNekoTokenWaitsWhileDesktopIsLost(t *testing.T) {
	grace := offlineGrace
	t.Cleanup(func() { offlineGrace = grace })
	for _, back := range []bool{true, false} {
		offlineGrace = time.Minute
		if !back {
			offlineGrace = 200 * time.Millisecond
		}
		f := newFixture(t, store.RoomSettings{})
		c, rec := f.join(anon("viewer"))
		f.r.SendNekoToken(f.ctx, c)
		rec.wait(t, "neko")
		logins := func() (n int) {
			for _, call := range f.fake.Calls() {
				if call.Path == "/api/login" {
					n++
				}
			}
			return n
		}
		before := logins()

		f.r.desktopLost()
		f.r.Handle(f.ctx, c, ClientMsg{Type: "neko_token"})
		if back {
			time.Sleep(100 * time.Millisecond)
			requireEqual(t, logins(), before)
			f.r.desktopFound()
			rec.wait(t, "neko")
			requireEqual(t, rec.count("desktop"), 0)
			continue
		}
		// Told with everyone, and once more as the answer.
		requireEqual(t, rec.wait(t, "desktop").(desktopMsg).State, "offline")
		requireEqual(t, rec.wait(t, "desktop").(desktopMsg).State, "offline")
		requireEqual(t, logins(), before)
		requireEqual(t, rec.count("neko"), 1) // the one from before
		f.r.desktopFound()
	}
}
