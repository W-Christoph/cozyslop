package diag

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func short(t *testing.T) {
	t.Helper()
	lock, probe, request := StuckLock, StuckProbe, StuckRequest
	t.Cleanup(func() { StuckLock, StuckProbe, StuckRequest = lock, probe, request })
	StuckLock, StuckProbe, StuckRequest = 20*time.Millisecond, 20*time.Millisecond, 20*time.Millisecond
}

func TestWatchdogStall(t *testing.T) {
	short(t)
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "diagnostics.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var mu Mutex
	var hub RWMutex
	w := &Watchdog{Store: s,
		Locks: func() []Lock { return []Lock{{"hub", hub.Held()}, {"room default", mu.Held()}} },
		Requests: func() []Request {
			return []Request{{Route: "GET /api/rooms/{room}/ws", Age: time.Hour, Long: true}}
		},
	}
	w.check(ctx, time.Now(), 0)
	mu.Lock()
	time.Sleep(30 * time.Millisecond)
	if mu.Held() < 30*time.Millisecond || hub.Held() != 0 {
		t.Fatalf("held %s and %s", mu.Held(), hub.Held())
	}
	w.check(ctx, time.Now(), 0)
	w.check(ctx, time.Now(), 3*time.Second) // still stuck, and late too
	mu.Unlock()
	w.check(ctx, time.Now(), 0)

	list := flush(t, s, Filter{Kinds: []string{KindStall}})
	if len(list) != 2 || list[1].Msg != "server stuck" || list[0].Msg != "server well again" {
		t.Fatalf("stall entries: %+v", list)
	}
	reasons, _ := list[0].Attrs["reasons"].([]any)
	if len(reasons) != 2 || !strings.HasPrefix(reasons[0].(string), "lock room default held") ||
		!strings.HasPrefix(reasons[1].(string), "watchdog 3s late") {
		t.Errorf("reasons: %v (a WebSocket open for long is no reason)", reasons)
	}
	name, _ := list[1].Attrs["dump"].(string)
	for w.dumping.Load() {
		time.Sleep(5 * time.Millisecond)
	}
	dump, err := s.Dump(name)
	if err != nil {
		t.Fatalf("dump %q: %v", name, err)
	}
	text, _ := io.ReadAll(dump)
	dump.Close()
	for _, want := range []string{"stuck: lock room default held", "== goroutine profile", "TestWatchdogStall", "== mutex profile"} {
		if !strings.Contains(string(text), want) {
			t.Errorf("dump lacks %q", want)
		}
	}
	if _, err := s.Dump("../diagnostics.db"); err != ErrNoDump {
		t.Errorf("a dump outside the folder: %v", err)
	}
	if samples := flush(t, s, Filter{Kinds: []string{KindSample}}); len(samples) != 2 || samples[0].Attrs["goroutines"] == nil ||
		samples[0].Attrs["lock_max_ms"] == nil || samples[0].Attrs["long_requests"] != float64(1) {
		t.Errorf("samples: %+v", samples)
	}
	if err := s.Clear(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Dump(name); err != ErrNoDump {
		t.Errorf("dump after Clear: %v", err)
	}
}

func TestWatchdogProbe(t *testing.T) {
	short(t)
	ctx := context.Background()
	release := make(chan struct{})
	var tries atomic.Int32
	p := &Probe{Name: "db_main", Try: func(ctx context.Context) error {
		tries.Add(1)
		<-release
		return nil
	}}
	w := &Watchdog{Probes: []*Probe{p}}
	w.check(ctx, time.Now(), 0)
	time.Sleep(30 * time.Millisecond)
	w.check(ctx, time.Now(), 0) // pending: not tried again
	if w.stall.IsZero() || !strings.HasPrefix(w.reasons[0], "db_main pending") {
		t.Fatalf("reasons %v", w.reasons)
	}
	close(release)
	for p.started.Load() != 0 {
		time.Sleep(time.Millisecond)
	}
	if tries.Load() != 1 {
		t.Errorf("tried %d times while the first was pending", tries.Load())
	}
	// The try took long: that is said once more (and it is tried again),
	// then it is well.
	w.check(ctx, time.Now(), 0)
	if w.stall.IsZero() {
		t.Error("a try that took long is no reason")
	}
	for p.started.Load() != 0 {
		time.Sleep(time.Millisecond)
	}
	w.check(ctx, time.Now(), 0)
	if !w.stall.IsZero() || tries.Load() < 2 {
		t.Errorf("stall since %v after %d tries", w.stall, tries.Load())
	}
}

func TestTransport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "42")
		if r.URL.Path == "/fail" {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		io.WriteString(w, "twelve bytes")
	}))
	defer srv.Close()
	s := openTest(t)
	c := &http.Client{Transport: Transport{Store: s, Target: "anilist"}}
	ctx := WithOp(WithCaller(context.Background(), "room_detect", "default"), "search")
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/?q=frieren", strings.NewReader(`{"query":"…"}`))
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if list := flush(t, s, Filter{}); len(list) != 0 {
		t.Fatalf("kept before the answer was read: %+v", list)
	}
	io.ReadAll(res.Body)
	res.Body.Close()
	res.Body.Close()
	list := flush(t, s, Filter{})
	if len(list) != 1 {
		t.Fatalf("entries: %+v", list)
	}
	e := list[0]
	if e.Kind != KindOutbound || e.Room != "default" || e.Msg != "anilist search" || e.Attrs["caller"] != "room_detect" ||
		e.Attrs["status"] != float64(200) || e.Attrs["rate_remaining"] != float64(42) || e.Attrs["duration_ms"] == nil ||
		e.Attrs["sent_bytes"] != float64(len(`{"query":"…"}`)) || e.Attrs["received_bytes"] != float64(12) {
		t.Errorf("entry: %+v", e)
	}

	poll := &http.Client{Transport: Transport{Store: s, Target: "helper", Room: "default", Summarize: true}}
	for _, p := range []string{"/status", "/status", "/fail"} {
		res, err := poll.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
	}
	if _, err := poll.Get("http://127.0.0.1:1/status"); err == nil {
		t.Fatal("no error from a closed port")
	}
	sums := s.summaries()
	if len(sums) != 2 {
		t.Fatalf("summaries: %+v", sums)
	}
	for _, e := range sums {
		switch e.Attrs["op"] {
		case "status":
			if e.Attrs["count"] != 3 || e.Attrs["errors"] != 1 || e.Attrs["kind"] != "refused" || e.Room != "default" {
				t.Errorf("status polls: %+v", e)
			}
		case "fail":
			if e.Attrs["count"] != 1 || e.Attrs["errors"] != 1 || e.Attrs["status"] != 503 {
				t.Errorf("failed poll: %+v", e)
			}
		default:
			t.Errorf("summary: %+v", e)
		}
	}
	if len(s.summaries()) != 0 {
		t.Error("summaries are taken once")
	}
}

func TestWatchdogProbeFails(t *testing.T) {
	ctx := context.Background()
	var fail atomic.Bool
	fail.Store(true)
	p := &Probe{Name: "db_main", Try: func(ctx context.Context) error {
		if fail.Load() {
			return errors.New("database is locked (5) (SQLITE_BUSY)")
		}
		return nil
	}}
	w := &Watchdog{Probes: []*Probe{p}}
	wait := func() {
		for p.started.Load() != 0 {
			time.Sleep(time.Millisecond)
		}
	}
	w.check(ctx, time.Now(), 0)
	wait()
	w.check(ctx, time.Now(), 0)
	if w.stall.IsZero() || !strings.HasPrefix(w.reasons[0], "db_main failed: database is locked") {
		t.Fatalf("reasons %v", w.reasons)
	}
	wait()
	fail.Store(false)
	w.check(ctx, time.Now(), 0) // the last try failed still
	wait()
	w.check(ctx, time.Now(), 0)
	if !w.stall.IsZero() {
		t.Errorf("still stuck: %v", w.reasons)
	}
}
