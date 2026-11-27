package hub

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestMemoryDetails(t *testing.T) {
	desktop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/memory/detail" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"used": 2147483648, "limit": 3355443200, "oomKills": 1, "peak": 3355443200,
			"stat": {"anon": 1610612736, "shmem": 268435456}, "events": {"max": 4, "oom_kill": 1},
			"shm": {"used": 262144000, "size": 2147483648},
			"programs": [{"name": "firefox-bin", "count": 1, "rss": 1073741824, "anon": 805306368, "shmem": 1048576}]}`))
	}))
	defer desktop.Close()

	f := newFixture(t, store.RoomSettings{})
	f.r.memoryURL = desktop.URL + "/memory"
	f.h.Locks()
	rooms := f.h.MemoryDetails(f.ctx)
	if len(rooms) != 1 || rooms[0].Detail == nil || rooms[0].People != 0 {
		t.Fatalf("rooms: %+v", rooms)
	}
	d := rooms[0].Detail
	if d.Used != 2<<30 || *d.Limit != 3200<<20 || d.Stat["shmem"] != 256<<20 || d.Events["max"] != 4 ||
		d.Shm.Used != 250<<20 || len(d.Programs) != 1 || d.Programs[0].Name != "firefox-bin" || d.Programs[0].RSS != 1<<30 {
		t.Errorf("detail: %+v", d)
	}

	f.r.memoryURL = desktop.URL + "/old"
	if rooms := f.h.MemoryDetails(f.ctx); rooms[0].Detail != nil || !strings.Contains(rooms[0].Err, "too old") {
		t.Errorf("old helper: %+v", rooms[0])
	}
}
