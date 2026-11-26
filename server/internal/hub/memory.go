package hub

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"
	"time"
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
