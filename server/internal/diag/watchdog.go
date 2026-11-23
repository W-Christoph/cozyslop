package diag

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"runtime"
	"runtime/metrics"
	"runtime/pprof"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Lock is a lock the watchdog keeps an eye on, and how long it is held.
type Lock struct {
	Name string
	Held time.Duration
}

// Request is a request being answered, and for how long.
type Request struct {
	Route string
	Age   time.Duration
	// Long-lived: WebSockets, the neko proxy, uploads. They do not count
	// as stuck however long they take.
	Long bool
}

// Probe is something the watchdog tries regularly, at most once at a time:
// if one try takes too long, the server is stuck there.
type Probe struct {
	Name string
	Try  func(ctx context.Context) error

	started atomic.Int64 // unix ns of the try under way; 0 if none
	took    atomic.Int64 // ns the last try took
	failed  atomic.Pointer[string]
}

// When the watchdog calls the server stuck.
var (
	StuckLock = 5 * time.Second
	// Below SQLite's busy timeout (5 s), after which a waiting writer fails.
	StuckProbe   = 3 * time.Second
	StuckRequest = 15 * time.Second
	StuckLag     = 2 * time.Second
	// At most one goroutine dump per this; SIGUSR1 makes one any time.
	DumpEvery = 10 * time.Minute
)

// Watchdog looks at the server every few seconds and keeps a health
// sample every other time. When something is stuck (a lock held for
// long, a database whose write lock cannot be had, a request that does not
// end, the watchdog itself running late), it keeps a stall entry and a
// dump of what every goroutine is doing, and another entry when the server
// is well again. Everything it calls must not wait for the server: lock
// ages and requests are read, not taken, and probes run on their own.
type Watchdog struct {
	Store    *Store
	Log      *slog.Logger // for stderr only: entries go to Store
	Every    time.Duration
	Locks    func() []Lock
	Requests func() []Request
	Probes   []*Probe
	// Gauges adds numbers of the server's to a sample (open connections,
	// requests under way, …).
	Gauges func(map[string]any)

	mu        sync.Mutex
	recent    []map[string]any // the last samples, for stall reports
	stall     time.Time        // since when the server is stuck; zero if not
	reasons   []string
	lastDump  time.Time
	dumping   atomic.Bool
	prev      sampleState
	sampleIdx int
}

// Run watches until ctx ends.
func (w *Watchdog) Run(ctx context.Context) {
	if w.Every <= 0 {
		w.Every = 5 * time.Second
	}
	runtime.SetMutexProfileFraction(100)
	t := time.NewTicker(w.Every)
	defer t.Stop()
	last := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			w.check(ctx, now, now.Sub(last)-w.Every)
			last = now
		}
	}
}

func (w *Watchdog) check(ctx context.Context, now time.Time, lag time.Duration) {
	var reasons []string
	if lag > StuckLag {
		reasons = append(reasons, fmt.Sprintf("watchdog %s late", lag.Round(time.Millisecond)))
	}
	var locks []Lock
	if w.Locks != nil {
		locks = w.Locks()
	}
	for _, l := range locks {
		if l.Held > StuckLock {
			reasons = append(reasons, fmt.Sprintf("lock %s held %s", l.Name, l.Held.Round(time.Millisecond)))
		}
	}
	for _, p := range w.Probes {
		if r := p.check(ctx, now); r != "" {
			reasons = append(reasons, r)
		}
	}
	var requests []Request
	if w.Requests != nil {
		requests = w.Requests()
	}
	for _, r := range requests {
		if !r.Long && r.Age > StuckRequest {
			reasons = append(reasons, fmt.Sprintf("request %s running %s", r.Route, r.Age.Round(time.Second)))
		}
	}
	slices.Sort(reasons)
	reasons = slices.Compact(reasons)

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.sampleIdx++; w.sampleIdx%2 == 1 {
		sample := w.sample(now, lag, locks, requests)
		w.recent = append(w.recent, sample)
		if len(w.recent) > 60 {
			w.recent = w.recent[1:]
		}
		w.Store.Add(Entry{Time: now, Kind: KindSample, Level: slog.LevelDebug, Msg: "sample", Attrs: sample})
	}
	switch {
	case len(reasons) > 0 && w.stall.IsZero():
		w.stall, w.reasons = now, reasons
		attrs := map[string]any{"reasons": reasons}
		if now.Sub(w.lastDump) >= DumpEvery {
			w.lastDump = now
			attrs["dump"] = w.dumpLocked(now, "stuck: "+strings.Join(reasons, "; "), locks, requests)
		}
		w.Store.Add(Entry{Time: now, Kind: KindStall, Level: slog.LevelError, Msg: "server stuck", Attrs: attrs})
		w.log().Error("server stuck: " + strings.Join(reasons, "; "))
	case len(reasons) > 0:
		for _, r := range reasons {
			if !slices.ContainsFunc(w.reasons, func(s string) bool { return reasonKind(s) == reasonKind(r) }) {
				w.reasons = append(w.reasons, r)
			}
		}
	case !w.stall.IsZero():
		d := now.Sub(w.stall)
		w.Store.Add(Entry{Time: now, Kind: KindStall, Level: slog.LevelWarn, Msg: "server well again",
			Attrs: map[string]any{"duration_ms": d.Milliseconds(), "reasons": w.reasons}})
		w.log().Warn(fmt.Sprintf("server well again after %s", d.Round(time.Second)))
		w.stall, w.reasons = time.Time{}, nil
	}
}

// reasonKind is a reason without its numbers, to keep one of each.
func reasonKind(reason string) string {
	if i := strings.LastIndexByte(reason, ' '); i > 0 {
		return reason[:i]
	}
	return reason
}

func (w *Watchdog) log() *slog.Logger {
	if w.Log != nil {
		return w.Log
	}
	return slog.New(slog.DiscardHandler)
}

// Dump keeps a goroutine dump now, whatever the server's state (SIGUSR1).
func (w *Watchdog) Dump(why string) {
	now := time.Now()
	var locks []Lock
	if w.Locks != nil {
		locks = w.Locks()
	}
	var requests []Request
	if w.Requests != nil {
		requests = w.Requests()
	}
	w.mu.Lock()
	name := w.dumpLocked(now, why, locks, requests)
	w.mu.Unlock()
	if name == "" {
		w.log().Warn("no dump kept: diagnostics are off, or a dump is being written")
		return
	}
	w.Store.Add(Entry{Time: now, Kind: KindStall, Level: slog.LevelInfo, Msg: "dump", Attrs: map[string]any{"dump": name, "reasons": []string{why}}})
	w.log().Info("dump kept: " + name)
}

// dumpLocked starts writing a dump and answers its name; the writing
// happens on its own, so that a stuck disk cannot stop the watchdog.
func (w *Watchdog) dumpLocked(now time.Time, why string, locks []Lock, requests []Request) string {
	name := fmt.Sprintf("stall-%d.txt.gz", now.UnixMilli())
	if w.Store == nil || w.Store.dumps == "" || !w.dumping.CompareAndSwap(false, true) {
		return ""
	}
	recent := slices.Clone(w.recent)
	go func() {
		defer w.dumping.Store(false)
		err := w.Store.writeDump(name, func(out io.Writer) error {
			b := bufio.NewWriter(out)
			fmt.Fprintf(b, "CozyCast stall report, %s\n%s\n\n", now.UTC().Format(time.RFC3339), why)
			fmt.Fprintln(b, "== Locks held")
			for _, l := range locks {
				if l.Held > 0 {
					fmt.Fprintf(b, "%s\t%s\n", l.Name, l.Held.Round(time.Millisecond))
				}
			}
			fmt.Fprintln(b, "\n== Requests under way")
			for _, r := range requests {
				fmt.Fprintf(b, "%s\t%s\tlong=%t\n", r.Route, r.Age.Round(time.Millisecond), r.Long)
			}
			fmt.Fprintln(b, "\n== Probes")
			for _, p := range w.Probes {
				fmt.Fprintln(b, p.describe(now))
			}
			fmt.Fprintln(b, "\n== Recent samples (oldest first)")
			for _, s := range recent {
				line, _ := json.Marshal(s)
				fmt.Fprintf(b, "%s\n", line)
			}
			for _, profile := range []struct {
				name  string
				debug int
			}{{"goroutine", 2}, {"mutex", 1}} {
				fmt.Fprintf(b, "\n== %s profile\n", profile.name)
				var text strings.Builder
				if err := pprof.Lookup(profile.name).WriteTo(&text, profile.debug); err != nil {
					fmt.Fprintf(b, "(%v)\n", err)
				}
				b.WriteString(ScrubIPs(text.String()))
			}
			return b.Flush()
		})
		if err != nil {
			w.log().Warn("write dump", "err", err)
		}
	}()
	return name
}

func (p *Probe) check(ctx context.Context, now time.Time) string {
	if started := p.started.Load(); started != 0 {
		if age := now.Sub(time.Unix(0, started)); age > StuckProbe {
			return fmt.Sprintf("%s pending %s", p.Name, age.Round(time.Millisecond))
		}
		return ""
	}
	// The last try's result, while the next one is on its way.
	reason := ""
	if failed := p.failed.Load(); failed != nil {
		reason = fmt.Sprintf("%s failed: %s", p.Name, *failed)
	} else if took := time.Duration(p.took.Load()); took > StuckProbe {
		reason = fmt.Sprintf("%s took %s", p.Name, took.Round(time.Millisecond))
	}
	p.started.Store(now.UnixNano())
	go func() {
		ctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		start := time.Now()
		err := p.Try(ctx)
		p.took.Store(int64(time.Since(start)))
		if err != nil {
			msg := Scrub(err.Error())
			p.failed.Store(&msg)
		} else {
			p.failed.Store(nil)
		}
		p.started.Store(0)
	}()
	return reason
}

func (p *Probe) describe(now time.Time) string {
	s := fmt.Sprintf("%s\tlast took %s", p.Name, time.Duration(p.took.Load()).Round(time.Millisecond))
	if started := p.started.Load(); started != 0 {
		s += fmt.Sprintf(", pending %s", now.Sub(time.Unix(0, started)).Round(time.Millisecond))
	}
	if failed := p.failed.Load(); failed != nil {
		s += ", failed: " + *failed
	}
	return s
}

// sampleState is what samples need of the one before, for rates.
type sampleState struct {
	at        time.Time
	cpuTicks  int64
	mutexWait float64
	gcCPU     float64
	throttled int64
	tls       int64
	dropped   int64
	sched     []uint64
}

var sampleMetrics = []string{
	"/sched/goroutines:goroutines",
	"/memory/classes/heap/objects:bytes",
	"/memory/classes/total:bytes",
	"/sync/mutex/wait/total:seconds",
	"/cpu/classes/gc/total:cpu-seconds",
	"/sched/latencies:seconds",
}

// sample is the server's health now: Go's runtime, the process, the
// container's pressure, and the server's own numbers.
func (w *Watchdog) sample(now time.Time, lag time.Duration, locks []Lock, requests []Request) map[string]any {
	s := map[string]any{"lag_ms": lag.Milliseconds()}
	ms := make([]metrics.Sample, len(sampleMetrics))
	for i, name := range sampleMetrics {
		ms[i].Name = name
	}
	metrics.Read(ms)
	prev, next := w.prev, sampleState{at: now}
	secs := now.Sub(prev.at).Seconds()
	first := prev.at.IsZero()
	for _, m := range ms {
		switch m.Value.Kind() {
		case metrics.KindUint64:
			v := m.Value.Uint64()
			switch m.Name {
			case "/sched/goroutines:goroutines":
				s["goroutines"] = v
			case "/memory/classes/heap/objects:bytes":
				s["heap_mb"] = v >> 20
			case "/memory/classes/total:bytes":
				s["memory_mb"] = v >> 20
			}
		case metrics.KindFloat64:
			v := m.Value.Float64()
			switch m.Name {
			case "/sync/mutex/wait/total:seconds":
				next.mutexWait = v
				if !first {
					s["mutex_wait_ms"] = int64((v - prev.mutexWait) * 1000)
				}
			case "/cpu/classes/gc/total:cpu-seconds":
				next.gcCPU = v
				if !first {
					s["gc_cpu_ms"] = int64((v - prev.gcCPU) * 1000)
				}
			}
		case metrics.KindFloat64Histogram:
			h := m.Value.Float64Histogram()
			next.sched = slices.Clone(h.Counts)
			if !first && len(prev.sched) == len(h.Counts) {
				s["sched_p99_ms"] = histogramQuantile(h, prev.sched, 0.99) * 1000
			}
		}
	}
	if ticks, ok := processTicks(); ok {
		next.cpuTicks = ticks
		if !first && secs > 0 {
			// Clock ticks are hundredths of a second on Linux.
			s["cpu_pct"] = int64(float64(ticks-prev.cpuTicks) / secs)
		}
	}
	if fds, err := os.ReadDir("/proc/self/fd"); err == nil {
		s["fds"] = len(fds)
	}
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		if f := strings.Fields(string(b)); len(f) > 0 {
			s["load1"] = f[0]
		}
	}
	for _, res := range []string{"cpu", "memory", "io"} {
		if v, ok := pressure("/sys/fs/cgroup/" + res + ".pressure"); ok {
			s[res+"_pressure"] = v
		}
	}
	if throttled, ok := cgroupThrottled(); ok {
		next.throttled = throttled
		if !first {
			s["throttled_ms"] = (throttled - prev.throttled) / 1000
		}
	}
	next.tls, next.dropped = TLSHandshakeErrors.Load(), w.Store.Dropped()
	if !first {
		s["tls_errors"], s["dropped"] = next.tls-prev.tls, next.dropped-prev.dropped
	}
	var longest time.Duration
	for _, l := range locks {
		longest = max(longest, l.Held)
	}
	s["lock_max_ms"] = longest.Milliseconds()
	short, long := 0, 0
	var oldest time.Duration
	for _, r := range requests {
		if r.Long {
			long++
		} else {
			short++
			oldest = max(oldest, r.Age)
		}
	}
	s["requests"], s["long_requests"], s["oldest_request_ms"] = short, long, oldest.Milliseconds()
	for _, p := range w.Probes {
		s[p.Name+"_ms"] = time.Duration(p.took.Load()).Milliseconds()
	}
	if w.Gauges != nil {
		w.Gauges(s)
	}
	w.prev = next
	return s
}

// histogramQuantile is the q quantile of what h counted since prev.
func histogramQuantile(h *metrics.Float64Histogram, prev []uint64, q float64) float64 {
	var total uint64
	for i, c := range h.Counts {
		total += c - prev[i]
	}
	if total == 0 {
		return 0
	}
	want := uint64(float64(total) * q)
	var seen uint64
	for i, c := range h.Counts {
		if seen += c - prev[i]; seen > want {
			if upper := h.Buckets[i+1]; !math.IsInf(upper, 1) {
				return upper
			}
			return h.Buckets[i]
		}
	}
	return 0
}

// processTicks is the CPU time the process used, in clock ticks.
func processTicks() (int64, bool) {
	b, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0, false
	}
	// After the command's name in parentheses: state, then fields 4 on;
	// utime and stime are fields 14 and 15.
	i := strings.LastIndexByte(string(b), ')')
	if i < 0 {
		return 0, false
	}
	f := strings.Fields(string(b[i+1:]))
	if len(f) < 13 {
		return 0, false
	}
	utime, err1 := strconv.ParseInt(f[11], 10, 64)
	stime, err2 := strconv.ParseInt(f[12], 10, 64)
	return utime + stime, err1 == nil && err2 == nil
}

// pressure is the share of the last 10 seconds some tasks waited for a
// resource ("some avg10" of a cgroup's pressure file), in percent.
func pressure(path string) (float64, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(line, "some ") {
			continue
		}
		for _, f := range strings.Fields(line) {
			if v, ok := strings.CutPrefix(f, "avg10="); ok {
				p, err := strconv.ParseFloat(v, 64)
				return p, err == nil
			}
		}
	}
	return 0, false
}

// cgroupThrottled is how long the container was held back by its CPU
// limit, in µs.
func cgroupThrottled() (int64, bool) {
	b, err := os.ReadFile("/sys/fs/cgroup/cpu.stat")
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(line, "throttled_usec "); ok {
			n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			return n, err == nil
		}
	}
	return 0, false
}
