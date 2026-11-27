package hub

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cozycast/internal/diag"
)

// Room memory: the desktop's helper (worker/window-title.py, GET /memory)
// says how much memory the room's container uses, its cap, and how many
// programs the kernel ended there for lack of memory. The health samples
// carry the fullest room (Hub.RoomMemory); a program ended is a warning.

// memoryInterval is how often a room is asked; the health samples are
// taken every 10 s.
const memoryInterval = 10 * time.Second

// memoryGauge is the room's last answer, read by the watchdog without a lock.
type memoryGauge struct {
	used  atomic.Int64 // bytes; 0 = unknown (no answer)
	limit atomic.Int64 // bytes; 0 = no cap
}

type memoryReport struct {
	Used     int64  `json:"used"`
	Limit    *int64 `json:"limit"`
	OOMKills int64  `json:"oomKills"`
}

func (r *Room) watchMemory(ctx context.Context) {
	defer r.memory.used.Store(0)
	kills := int64(-1) // unknown until the first answer
	t := time.NewTicker(memoryInterval)
	defer t.Stop()
	for {
		kills = r.checkMemory(ctx, kills)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// checkMemory asks the room and returns the count of programs ended so far,
// warning about those since the last answer (kills, -1 = none yet). A room
// that restarted counts from 0 again.
func (r *Room) checkMemory(ctx context.Context, kills int64) int64 {
	m, ok := fetchMemory(ctx, r.neko.Transport(), r.memoryURL)
	if !ok {
		r.memory.used.Store(0)
		return kills
	}
	limit := int64(0)
	if m.Limit != nil {
		limit = *m.Limit
	}
	r.memory.used.Store(m.Used)
	r.memory.limit.Store(limit)
	ended := m.OOMKills - kills
	if m.OOMKills < kills {
		ended = m.OOMKills
	}
	if kills >= 0 && ended > 0 {
		r.log.Warn("out of memory: programs ended", "ended", ended, "used_mb", m.Used>>20, "limit_mb", limit>>20)
	}
	return m.OOMKills
}

func fetchMemory(ctx context.Context, transport http.RoundTripper, url string) (memoryReport, bool) {
	var m memoryReport
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return m, false
	}
	// The helper sits next to neko, reached the same way.
	res, err := (&http.Client{Timeout: 5 * time.Second, Transport: transport}).Do(req)
	if err != nil {
		return m, false
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&m) != nil {
		return m, false
	}
	return m, m.Used >= 0 && m.OOMKills >= 0
}

// RoomMemory is the memory of the room that uses the most and its cap
// (0 = none), for the health samples; false if no room has answered. Rooms
// are the ones Locks last saw.
func (h *Hub) RoomMemory() (used, limit int64, ok bool) {
	rooms := h.lockRooms.Load()
	if rooms == nil {
		return 0, 0, false
	}
	for _, r := range *rooms {
		if u := r.memory.used.Load(); u > used {
			used, limit = u, r.memory.limit.Load()
		}
	}
	return used, limit, used > 0
}

// MemoryDetail is what a room's helper says makes up its memory
// (GET /memory/detail), for an admin's snapshot. Bytes.
type MemoryDetail struct {
	memoryReport
	Peak     int64                       `json:"peak"` // the most it ever used, file cache included; 0 = unknown
	Swap     int64                       `json:"swap"`
	Stat     map[string]int64            `json:"stat"`   // of memory.stat: anon, file, shmem, kernel, …
	Events   map[string]int64            `json:"events"` // of memory.events: max, oom_kill, …
	Shm      *struct{ Used, Size int64 } `json:"shm"`
	Programs []struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
		RSS   int64  `json:"rss"`
		Anon  int64  `json:"anon"`
		Shmem int64  `json:"shmem"`
	} `json:"programs"` // by resident memory, the most first
}

// RoomMemoryDetail is one room in a snapshot.
type RoomMemoryDetail struct {
	Name, Source string
	People       int  // -1 = unknown (its lock was taken)
	Asleep       bool // its programs are paused (hibernate.go)
	Detail       *MemoryDetail
	Err          string // why Detail is nil
}

// MemoryDetails asks every room's helper what uses its memory, all at once
// and for at most 5 s. Rooms are the ones Locks last saw.
func (h *Hub) MemoryDetails(ctx context.Context) []RoomMemoryDetail {
	rooms := h.lockRooms.Load()
	if rooms == nil {
		return nil
	}
	out := make([]RoomMemoryDetail, len(*rooms))
	var wg sync.WaitGroup
	for i, r := range *rooms {
		d := &out[i]
		d.Name, d.Source, d.People = r.Name, r.Source, -1
		if r.mu.TryLock() {
			d.People, d.Asleep = len(r.members), r.sleep == sleepFrozen
			r.mu.Unlock()
		}
		if r.memoryURL == "" {
			d.Err = "no helper"
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.Detail, d.Err = fetchMemoryDetail(ctx, r.neko.Transport(), r.memoryURL+"/detail")
		}()
	}
	wg.Wait()
	slices.SortFunc(out, func(a, b RoomMemoryDetail) int { return strings.Compare(a.Name, b.Name) })
	return out
}

func fetchMemoryDetail(ctx context.Context, transport http.RoundTripper, url string) (*MemoryDetail, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err.Error()
	}
	res, err := (&http.Client{Timeout: 5 * time.Second, Transport: transport}).Do(req)
	if err != nil {
		return nil, "no answer: " + diag.Scrub(err.Error())
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return nil, "the room's helper is too old (rebuild the room's image) or the room has no memory cgroup"
	}
	if res.StatusCode != http.StatusOK {
		return nil, "answered " + res.Status
	}
	var d MemoryDetail
	if err := json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&d); err != nil {
		return nil, "unreadable answer"
	}
	return &d, ""
}
