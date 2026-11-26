package hub

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"cozycast/internal/store"
)

func TestRoomMemory(t *testing.T) {
	var mu sync.Mutex
	answer, status := `{"used": 2147483648, "limit": 3355443200, "oomKills": 1}`, http.StatusOK
	desktop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer))
	}))
	defer desktop.Close()
	set := func(a string, s int) { mu.Lock(); answer, status = a, s; mu.Unlock() }

	f := newFixture(t, store.RoomSettings{})
	f.r.memoryURL = desktop.URL
	f.h.Locks() // RoomMemory reads the rooms Locks saw
	if _, _, ok := f.h.RoomMemory(); ok {
		t.Fatal("memory before any answer")
	}

	// The first answer is the baseline: programs ended before are not news.
	kills := f.r.checkMemory(f.ctx, -1)
	used, limit, ok := f.h.RoomMemory()
	if !ok || used != 2<<30 || limit != 3200<<20 || kills != 1 {
		t.Fatalf("used %d, limit %d, ok %v, kills %d", used, limit, ok, kills)
	}

	set(`{"used": 3355443200, "limit": null, "oomKills": 3}`, http.StatusOK)
	if kills = f.r.checkMemory(f.ctx, kills); kills != 3 {
		t.Fatalf("kills %d", kills)
	}
	if _, limit, _ := f.h.RoomMemory(); limit != 0 {
		t.Fatalf("no cap: limit %d", limit)
	}
	// A room that restarted counts from 0; no answer keeps the count.
	if kills = f.r.checkMemory(f.ctx, kills+5); kills != 3 {
		t.Fatalf("after a restart: kills %d", kills)
	}
	set("", http.StatusNotFound)
	if kills = f.r.checkMemory(f.ctx, kills); kills != 3 {
		t.Fatalf("no answer: kills %d", kills)
	}
	if _, _, ok := f.h.RoomMemory(); ok {
		t.Fatal("memory without an answer")
	}
}
