package diag

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"slices"
	"strconv"
	"strings"
	"time"
)

// A snapshot is the server's state now, as text an admin can paste into a
// message to whoever helps them: where the memory goes (the machine, the
// server, each room) and what the diagnostics kept of the last hours. Like
// the rest of the diagnostics, it names no one.

var started = time.Now()

// System is the machine's memory and the server container's, in bytes
// (0 = unknown), for adding up where the machine's memory goes.
type System struct {
	HostTotal, HostAvailable int64
	ServerUsed               int64 // the server's container, without the file cache dropped first
}

// WriteSystem writes the snapshot's heading, the machine and the server.
func WriteSystem(w io.Writer, now time.Time) System {
	var sys System
	fmt.Fprintf(w, "CozyCast snapshot, %s (times UTC)\n", now.UTC().Format(time.RFC3339))
	fmt.Fprintf(w, "server up %s, %s, %d CPUs\n", now.Sub(started).Round(time.Second), runtime.Version(), runtime.NumCPU())

	fmt.Fprintln(w, "\n== Machine (/proc/meminfo)")
	info := keyValues("/proc/meminfo", ":")
	kb := func(k string) int64 {
		v, _ := strconv.ParseInt(strings.TrimSuffix(info[k], " kB"), 10, 64)
		return v << 10
	}
	sys.HostTotal, sys.HostAvailable = kb("MemTotal"), kb("MemAvailable")
	if sys.HostTotal > 0 {
		fmt.Fprintf(w, "total %s, available %s (%d %%), used %s\n", MB(sys.HostTotal), MB(sys.HostAvailable),
			sys.HostAvailable*100/sys.HostTotal, MB(sys.HostTotal-sys.HostAvailable))
		var parts []string
		for _, k := range []string{"MemFree", "Cached", "Buffers", "Shmem", "AnonPages", "Slab", "SReclaimable", "KernelStack", "PageTables", "Committed_AS", "SwapTotal", "SwapFree"} {
			if _, ok := info[k]; ok {
				parts = append(parts, k+" "+MB(kb(k)))
			}
		}
		fmt.Fprintln(w, strings.Join(parts, ", "))
	} else {
		fmt.Fprintln(w, "(unreadable)")
	}
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		if f := strings.Fields(string(b)); len(f) >= 3 {
			fmt.Fprintf(w, "load %s\n", strings.Join(f[:3], " "))
		}
	}
	if b, err := os.ReadFile("/proc/pressure/memory"); err == nil {
		fmt.Fprintf(w, "memory pressure: %s\n", strings.Join(strings.Split(strings.TrimSpace(string(b)), "\n"), "; "))
	}

	fmt.Fprintln(w, "\n== Server process")
	status := keyValues("/proc/self/status", ":")
	var parts []string
	for _, k := range []string{"VmRSS", "VmHWM", "RssAnon", "RssFile", "VmSwap", "Threads"} {
		if v, ok := status[k]; ok {
			if n, err := strconv.ParseInt(strings.TrimSuffix(v, " kB"), 10, 64); err == nil && strings.HasSuffix(v, " kB") {
				v = MB(n << 10)
			}
			parts = append(parts, k+" "+v)
		}
	}
	fmt.Fprintln(w, strings.Join(parts, ", "))
	names := []string{"/memory/classes/total:bytes", "/memory/classes/heap/objects:bytes", "/memory/classes/heap/unused:bytes",
		"/memory/classes/heap/free:bytes", "/memory/classes/heap/released:bytes", "/memory/classes/heap/stacks:bytes",
		"/memory/classes/metadata/other:bytes", "/memory/classes/other:bytes", "/gc/heap/goal:bytes",
		"/gc/cycles/total:gc-cycles", "/sched/goroutines:goroutines"}
	ms := make([]metrics.Sample, len(names))
	for i, n := range names {
		ms[i].Name = n
	}
	metrics.Read(ms)
	parts = parts[:0]
	for _, m := range ms {
		if m.Value.Kind() != metrics.KindUint64 {
			continue
		}
		name, unit, _ := strings.Cut(strings.TrimPrefix(m.Name, "/"), ":")
		name = strings.TrimPrefix(strings.TrimPrefix(name, "memory/classes/"), "gc/")
		if unit == "bytes" {
			parts = append(parts, name+" "+MB(int64(m.Value.Uint64())))
		} else {
			parts = append(parts, name+" "+strconv.FormatUint(m.Value.Uint64(), 10))
		}
	}
	if limit := debug.SetMemoryLimit(-1); limit != math.MaxInt64 {
		parts = append(parts, "GOMEMLIMIT "+MB(limit))
	}
	fmt.Fprintf(w, "Go: %s\n", strings.Join(parts, ", "))

	fmt.Fprintln(w, "\n== Server container (cgroup)")
	cg, ok := cgroupMemory("/sys/fs/cgroup")
	if ok {
		sys.ServerUsed = cg.used
	}
	fmt.Fprintln(w, cg.text)
	return sys
}

type cgroup struct {
	used int64
	text string
}

// cgroupMemory describes the memory of the cgroup (v2) in dir.
func cgroupMemory(dir string) (cgroup, bool) {
	read := func(name string) string { b, _ := os.ReadFile(dir + "/" + name); return strings.TrimSpace(string(b)) }
	current, err := strconv.ParseInt(read("memory.current"), 10, 64)
	if err != nil {
		return cgroup{text: "(no memory cgroup)"}, false
	}
	stat := keyValues(dir+"/memory.stat", " ")
	n := func(m map[string]string, k string) int64 { v, _ := strconv.ParseInt(m[k], 10, 64); return v }
	used := max(0, current-n(stat, "inactive_file"))
	text := fmt.Sprintf("used %s (of %s with the file cache)", MB(used), MB(current))
	if cap := read("memory.max"); cap != "" && cap != "max" {
		if v, err := strconv.ParseInt(cap, 10, 64); err == nil {
			text += ", cap " + MB(v)
		}
	} else {
		text += ", no cap"
	}
	if v, err := strconv.ParseInt(read("memory.peak"), 10, 64); err == nil {
		text += ", peak " + MB(v)
	}
	var parts []string
	for _, k := range []string{"anon", "file", "shmem", "kernel", "sock"} {
		if _, ok := stat[k]; ok {
			parts = append(parts, k+" "+MB(n(stat, k)))
		}
	}
	if len(parts) > 0 {
		text += "\n" + strings.Join(parts, ", ")
	}
	events := keyValues(dir+"/memory.events", " ")
	if k := n(events, "oom_kill"); k > 0 {
		text += fmt.Sprintf("\nprograms ended at the cap: %d", k)
	}
	return cgroup{used: used, text: text}, true
}

// keyValues reads a file of "key<sep> value" lines.
func keyValues(path, sep string) map[string]string {
	m := map[string]string{}
	b, err := os.ReadFile(path)
	if err != nil {
		return m
	}
	for _, line := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(line, sep); ok {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return m
}

// MB is b in whole MB, as the snapshot writes it.
func MB(b int64) string { return fmt.Sprintf("%d MB", b>>20) }

// WriteHistory writes what the store kept: the newest health sample, the
// last hour per 5 minutes, the last day per hour, the counts, and the
// warnings, errors, stalls and starts.
func (s *Store) WriteHistory(ctx context.Context, w io.Writer, now time.Time) error {
	if s == nil {
		fmt.Fprintln(w, "\n(diagnostics are off: no history)")
		return nil
	}
	latest, err := s.Entries(ctx, Filter{Kinds: []string{KindSample}, Limit: 1})
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "\n== Newest health sample")
	if len(latest) > 0 {
		keys := make([]string, 0, len(latest[0].Attrs))
		for k := range latest[0].Attrs {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = fmt.Sprintf("%s=%v", k, latest[0].Attrs[k])
		}
		fmt.Fprintf(w, "%s: %s\n", latest[0].Time.UTC().Format("15:04:05"), strings.Join(parts, " "))
	} else {
		fmt.Fprintln(w, "(none)")
	}

	for _, t := range []struct {
		title string
		span  time.Duration
		per   time.Duration
	}{{"Last hour, per 5 minutes", time.Hour, 5 * time.Minute}, {"Last day, per hour", 24 * time.Hour, time.Hour}} {
		sum, err := s.Summarize(ctx, now.Add(-t.span), now)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "\n== %s (highest; free memory the lowest)\n", t.title)
		writeSampleTable(w, sum.Samples, t.per)
		if t.span == 24*time.Hour {
			c := sum.Counts
			fmt.Fprintf(w, "\n== Last day, counted\nerrors %d, warnings %d, stalls %d, slow or failed requests %d (failed %d), AniList calls %d (failed %d, too many %d), starts %d (after going down %d), entries dropped %d\n",
				c.Errors, c.Warnings, c.Stalls, c.Requests, c.FailedRequests, c.AniList, c.AniListFailed, c.AniListLimited, c.Boots, c.Crashes, s.Dropped())
		}
	}

	warn := slog.LevelWarn
	for _, l := range []struct {
		title string
		f     Filter
	}{
		{"Warnings and errors, last day (newest first)", Filter{Kinds: []string{KindEvent}, MinLevel: &warn, Since: now.Add(-24 * time.Hour), Limit: 40}},
		{"Stalls, starts and stops, last week (newest first)", Filter{Kinds: []string{KindStall, KindBoot, KindShutdown}, Since: now.Add(-7 * 24 * time.Hour), Limit: 30}},
	} {
		entries, err := s.Entries(ctx, l.f)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "\n== %s\n", l.title)
		if len(entries) == 0 {
			fmt.Fprintln(w, "(none)")
		}
		for _, e := range entries {
			line := fmt.Sprintf("%s %s %s", e.Time.UTC().Format("01-02 15:04:05"), e.Level, e.Msg)
			if e.Room != "" {
				line += " room=" + e.Room
			}
			if len(e.Attrs) > 0 {
				attrs, _ := json.Marshal(e.Attrs)
				line += " " + string(attrs)
			}
			fmt.Fprintln(w, line)
		}
	}
	return nil
}

// writeSampleTable writes samples merged into rows of per.
func writeSampleTable(w io.Writer, samples []SampleBucket, per time.Duration) {
	if len(samples) == 0 {
		fmt.Fprintln(w, "(no samples)")
		return
	}
	const row = "%-5s %5s %9s %7s  %-11s %6s %7s %5s %6s %6s\n"
	fmt.Fprintf(w, row, "time", "cpu%", "serverMB", "freeMB", "roomMB/cap", "people", "lockMs", "dbMs", "reqMs", "lagMs")
	var rows []SampleBucket
	step := per.Milliseconds()
	for _, b := range samples {
		start := b.Time / step * step
		if n := len(rows); n == 0 || rows[n-1].Time != start {
			rows = append(rows, SampleBucket{Time: start})
		}
		r := &rows[len(rows)-1]
		for _, f := range []struct{ to, from **float64 }{{&r.CPU, &b.CPU}, {&r.MemoryMB, &b.MemoryMB}, {&r.People, &b.People},
			{&r.RoomMemoryMB, &b.RoomMemoryMB}, {&r.RoomLimitMB, &b.RoomLimitMB}, {&r.LockMs, &b.LockMs}, {&r.DBMs, &b.DBMs},
			{&r.RequestMs, &b.RequestMs}, {&r.LagMs, &b.LagMs}} {
			if *f.from != nil && (*f.to == nil || **f.from > **f.to) {
				*f.to = *f.from
			}
		}
		if b.HostFreeMB != nil && (r.HostFreeMB == nil || *b.HostFreeMB < *r.HostFreeMB) {
			r.HostFreeMB = b.HostFreeMB
		}
	}
	num := func(v *float64) string {
		if v == nil {
			return "-"
		}
		return strconv.FormatInt(int64(math.Round(*v)), 10)
	}
	for _, r := range rows {
		fmt.Fprintf(w, row, time.UnixMilli(r.Time).UTC().Format("15:04"), num(r.CPU), num(r.MemoryMB), num(r.HostFreeMB),
			num(r.RoomMemoryMB)+"/"+num(r.RoomLimitMB), num(r.People), num(r.LockMs), num(r.DBMs), num(r.RequestMs), num(r.LagMs))
	}
}
