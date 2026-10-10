package hub

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"cozycast/internal/store"
)

func TestHibernation(t *testing.T) {
	var mu sync.Mutex
	var asked []string
	status := http.StatusNoContent
	desktop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		asked = append(asked, r.Method+" "+r.URL.Path+" "+r.Header.Get("Authorization"))
		w.WriteHeader(status)
	}))
	defer desktop.Close()
	wait := func(what string, n int) {
		t.Helper()
		deadline := time.Now().Add(testTimeout)
		for {
			mu.Lock()
			got := append([]string(nil), asked...)
			mu.Unlock()
			if len(got) == n && got[n-1] == what+" Bearer secret" {
				return
			}
			if len(got) > n || time.Now().After(deadline) {
				t.Fatalf("want %d requests ending in %q, got %q", n, what, got)
			}
			time.Sleep(time.Millisecond)
		}
	}
	oldRetry := wakeRetry
	wakeRetry = time.Millisecond
	defer func() { wakeRetry = oldRetry }()

	f := newFixture(t, store.RoomSettings{})
	hibernating := func(want bool) {
		t.Helper()
		deadline := time.Now().Add(testTimeout)
		for f.r.Hibernating() != want {
			if time.Now().After(deadline) {
				t.Fatalf("hibernating: want %v", want)
			}
			time.Sleep(time.Millisecond)
		}
	}
	const after = 300 * time.Millisecond
	f.r.mu.Lock()
	f.r.hibernateURL, f.r.playToken = desktop.URL, "secret"
	f.h.HibernateAfter = after
	f.r.mu.Unlock()

	// A start of the server: the programs may be paused from its last run,
	// so they are continued for the first to join.
	alice, _ := f.join(anon("alice"))
	wait("POST /thaw", 1)
	// Known to run: nothing is asked for the next tab or person, and a
	// room with someone in it does not hibernate.
	bob, _ := f.join(anon("bobby"))
	alice2, _ := f.join(anon("alice"))
	f.r.Leave(f.ctx, alice2)
	f.r.Leave(f.ctx, alice)
	time.Sleep(after + after/2)
	wait("POST /thaw", 1)

	// Empty, but not for long enough: the wait starts over each time.
	f.r.Leave(f.ctx, bob)
	time.Sleep(after / 3)
	carol, _ := f.join(anon("carol"))
	time.Sleep(after / 3)
	f.r.Leave(f.ctx, carol)
	time.Sleep(after / 2)
	wait("POST /thaw", 1)
	hibernating(false)

	// Empty for long enough.
	wait("POST /freeze", 2)
	hibernating(true)
	requireEqual(t, f.r.UserCount(), 0)

	// The first to join wakes it.
	dave, _ := f.join(anon("david"))
	wait("POST /thaw", 3)
	hibernating(false)

	// A helper that fails: the room does not count as hibernating.
	mu.Lock()
	status = http.StatusInternalServerError
	mu.Unlock()
	f.r.Leave(f.ctx, dave)
	wait("POST /freeze", 4)
	hibernating(false)
	// It may have paused some programs all the same: continuing is tried
	// a few times, and again for the next to join.
	f.join(anon("erin1"))
	wait("POST /thaw", 4+wakeTries)
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	status = http.StatusNoContent
	mu.Unlock()
	f.join(anon("frank"))
	wait("POST /thaw", 5+wakeTries)

	// Off: an empty room is left alone.
	f.r.mu.Lock()
	f.h.HibernateAfter = 0
	f.r.mu.Unlock()
	f.r.kick("a:erin1", kickedMsg{Type: "kicked", Reason: "kicked"})
	f.r.kick("a:frank", kickedMsg{Type: "kicked", Reason: "kicked"})
	requireEqual(t, f.r.UserCount(), 0)
	time.Sleep(after)
	wait("POST /thaw", 5+wakeTries)
}
