package diag

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScrub(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"dial tcp 203.0.113.7:443: i/o timeout", "dial tcp <ip>:443: i/o timeout"},
		{"from 2001:db8::1 and [2a00:1450:4001:82a::200e]:443", "from <ip> and <ip>:443"},
		{"mapped ::ffff:198.51.100.2", "mapped <ip>"},
		// The server's own, its rooms' and the tunnel's addresses help.
		{"peer 10.77.0.2:51820 via 172.18.0.3 and 127.0.0.1, fe80::1%eth0", "peer 10.77.0.2:51820 via 172.18.0.3 and 127.0.0.1, fe80::1%eth0"},
		{"at 12:34:56 for 3.5s", "at 12:34:56 for 3.5s"},
		// What "open page" logs: the helper's address with the page in its query.
		{`Post "http://room-default:8081/open?url=https%3A%2F%2Fexample.org%2Fshow%2Fepisode-5": context deadline exceeded`,
			`Post "http://room-default:8081/…": context deadline exceeded`},
		{"ws://room:8080/api/ws?token=c2VjcmV0LXRva2VuLXZhbHVl", "ws://room:8080/…"},
		{`Post "https://graphql.anilist.co": EOF`, `Post "https://graphql.anilist.co": EOF`},
		{"https://user:pw@203.0.113.9:8443/x", "https://<ip>:8443/…"},
		{"mail alice@example.org now", "mail <email> now"},
		{"key Zm9vYmFyYmF6cXV4MTIzNDU2Nzg5MA== bad", "key <redacted> bad"},
		{"handshake did not complete in time", "handshake did not complete in time"},
	} {
		if got := Scrub(tc.in); got != tc.want {
			t.Errorf("Scrub(%q)\n got %q\nwant %q", tc.in, got, tc.want)
		}
	}
	long := Scrub(strings.Repeat("ä", 400))
	if len(long) > MaxText+len("…") || !strings.HasSuffix(long, "…") || !strings.HasPrefix(long, "ää") {
		t.Errorf("long text cut to %d bytes: %q…", len(long), long[:10])
	}
	// The helper's own error, as Go makes it.
	err := &url.Error{Op: "Post", URL: "http://room:8081/open?url=" + url.QueryEscape("https://example.org/watch/frieren-05"), Err: errors.New("timeout")}
	if got := Scrub(err.Error()); strings.Contains(got, "frieren") || strings.Contains(got, "example.org") {
		t.Errorf("Scrub(%q) = %q", err, got)
	}
}

func TestHandler(t *testing.T) {
	s := openTest(t)
	var out bytes.Buffer
	log := slog.New(NewHandler(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}), s))
	log.With("room", "default", "client", "c123").Warn("open page from 203.0.113.7",
		"err", fmt.Errorf("dial 198.51.100.1: refused"), "username", "alice", "anime", 154587, "status", 503,
		slog.Group("g", "identity", "u:12", "status", 503))
	log.Debug("client joined", "identity", "u:12")
	log.Info("plain")
	for _, leak := range []string{"c123", "alice", "u:12", "203.0.113.7", "198.51.100.1", "client=", "username", "identity", "anime", "154587"} {
		if strings.Contains(out.String(), leak) {
			t.Errorf("stderr has %q: %s", leak, out.String())
		}
	}
	for _, want := range []string{"room=default", " status=503", "g.status=503", `err="dial <ip>: refused"`, "client joined"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stderr lacks %q: %s", want, out.String())
		}
	}
	entries := flush(t, s, Filter{})
	if len(entries) != 2 {
		t.Fatalf("kept %d entries, want 2 (debug is not kept): %+v", len(entries), entries)
	}
	e := entries[1]
	if e.Kind != KindEvent || e.Level != slog.LevelWarn || e.Room != "default" || e.Msg != "open page from <ip>" {
		t.Errorf("entry = %+v", e)
	}
	if e.Attrs["status"] != float64(503) || e.Attrs["err"] != "dial <ip>: refused" || e.Attrs["g.status"] != float64(503) || len(e.Attrs) != 3 {
		t.Errorf("attrs = %v", e.Attrs)
	}
}

func TestEntriesFilter(t *testing.T) {
	s := openTest(t)
	base := time.UnixMilli(1_700_000_000_000)
	s.Add(Entry{Time: base, Kind: KindEvent, Level: slog.LevelInfo, Room: "a", Msg: "one"})
	s.Add(Entry{Time: base.Add(time.Second), Kind: KindEvent, Level: slog.LevelError, Room: "b", Msg: "two", Attrs: map[string]any{"err": "disk full"}})
	s.Add(Entry{Time: base.Add(2 * time.Second), Kind: KindOutbound, Level: slog.LevelInfo, Msg: "anilist"})
	all := flush(t, s, Filter{})
	if len(all) != 3 || all[0].Msg != "anilist" {
		t.Fatalf("entries = %+v", all)
	}
	warn := slog.LevelWarn
	for _, tc := range []struct {
		f    Filter
		want string
	}{
		{Filter{Kinds: []string{KindOutbound}}, "anilist"},
		{Filter{Kinds: []string{KindOutbound, KindEvent}, Limit: 2}, "anilist,two"},
		{Filter{MinLevel: &warn}, "two"},
		{Filter{Room: "a"}, "one"},
		{Filter{Since: base.Add(time.Second), Until: base.Add(2 * time.Second)}, "two"},
		{Filter{Before: all[1].ID}, "one"},
		{Filter{Text: "DISK"}, "two"},
	} {
		list, err := s.Entries(context.Background(), tc.f)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, e := range list {
			got = append(got, e.Msg)
		}
		if strings.Join(got, ",") != tc.want {
			t.Errorf("%+v: got %v, want %s", tc.f, got, tc.want)
		}
	}
}

func TestSweep(t *testing.T) {
	s := openTest(t)
	now := time.Now()
	s.Add(Entry{Time: now.Add(-Retention - time.Minute), Kind: KindEvent, Msg: "old"})
	s.Add(Entry{Time: now.Add(-SampleRetention - time.Minute), Kind: KindSample, Msg: "old sample"})
	s.Add(Entry{Time: now.Add(-SampleRetention - time.Minute), Kind: KindEvent, Msg: "kept"})
	s.Add(Entry{Time: now, Kind: KindSample, Msg: "new sample"})
	flush(t, s, Filter{})
	defer func(n int) { MaxEntries = n }(MaxEntries)
	MaxEntries = 1
	s.sweep()
	list, _ := s.Entries(context.Background(), Filter{})
	if len(list) != 1 || list[0].Msg != "new sample" {
		t.Errorf("after sweep with room for 1: %+v", list)
	}
	MaxEntries = 10
	s.Add(Entry{Time: now.Add(-SampleRetention - time.Minute), Kind: KindEvent, Msg: "kept"})
	s.Add(Entry{Time: now.Add(-Retention - time.Minute), Kind: KindEvent, Msg: "old"})
	flush(t, s, Filter{})
	s.sweep()
	list, _ = s.Entries(context.Background(), Filter{})
	if len(list) != 2 || list[0].Msg != "kept" {
		t.Errorf("after sweep: %+v", list)
	}
	if err := s.Clear(context.Background()); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.Entries(context.Background(), Filter{}); len(list) != 0 {
		t.Errorf("after Clear: %+v", list)
	}
}

func TestBootAndBrokenFile(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "diagnostics.db")
	if err := os.WriteFile(path, []byte("not a database, not at all, not even close to one"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("a broken file is replaced: %v", err)
	}
	s.Boot(ctx)
	s.Shutdown()
	// Stored after the stop, with an earlier time: the last minute's
	// summaries of calls.
	s.Add(Entry{Time: time.Now().Add(-40 * time.Second), Kind: KindOutbound, Level: slog.LevelInfo, Msg: "helper memory (per minute)"})
	s.Close()

	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	s.Boot(ctx)
	if b := flush(t, s, Filter{Kinds: []string{KindBoot}}); len(b) != 2 || b[0].Attrs["previous"] != nil {
		t.Errorf("boot after a clean shutdown: %+v", b)
	}
	s.Close() // no Shutdown: as if it had crashed

	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Boot(ctx)
	if b := flush(t, s, Filter{Kinds: []string{KindBoot}}); len(b) != 3 || b[0].Attrs["previous"] != "no clean shutdown" || b[0].Attrs["last_seen"] == nil {
		t.Errorf("boot after a crash: %+v", b)
	}
	s.Close()

	// What Go wrote when it ended the server.
	crash := filepath.Join(DumpDir(path), "crash.txt")
	if info, err := os.Stat(crash); err != nil || info.Size() != 0 {
		t.Fatalf("crash file after a boot: %v %v", info, err)
	}
	if err := os.WriteFile(crash, []byte("panic: boom near 203.0.113.9\n\ngoroutine 1 [running]:\nmain.main()\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Boot(ctx)
	b := flush(t, s, Filter{Kinds: []string{KindBoot}})
	dump, _ := b[0].Attrs["dump"].(string)
	if b[0].Attrs["crash"] != "panic: boom near <ip>" || !strings.HasPrefix(dump, "crash-") {
		t.Fatalf("boot after a Go crash: %+v", b[0])
	}
	r, err := s.Dump(dump)
	if err != nil {
		t.Fatal(err)
	}
	text, _ := io.ReadAll(r)
	r.Close()
	if !strings.Contains(string(text), "goroutine 1 [running]") || strings.Contains(string(text), "203.0.113.9") {
		t.Errorf("crash dump: %q", text)
	}
	if info, err := os.Stat(crash); err != nil || info.Size() != 0 {
		t.Errorf("crash file not started anew: %v %v", info, err)
	}
}

func TestAddNeverWaits(t *testing.T) {
	var s *Store
	s.Add(Entry{Msg: "nil store"}) // keeps nothing, and does not panic
	s = &Store{entries: make(chan Entry, 1), now: time.Now}
	s.Add(Entry{Msg: "one"})
	done := make(chan struct{})
	go func() { s.Add(Entry{Msg: "two"}); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Add waited for the writer")
	}
	if s.Dropped() != 1 {
		t.Errorf("dropped %d, want 1", s.Dropped())
	}
}

func TestServerErrorLog(t *testing.T) {
	var out bytes.Buffer
	l := ServerErrorLog(slog.New(NewHandler(slog.NewTextHandler(&out, nil), nil)))
	before := TLSHandshakeErrors.Load()
	l.Printf("http: TLS handshake error from 203.0.113.5:5555: EOF")
	l.Printf("http: panic serving 203.0.113.5:5555: boom")
	if TLSHandshakeErrors.Load() != before+1 {
		t.Error("handshake error not counted")
	}
	if s := out.String(); strings.Contains(s, "handshake") || strings.Contains(s, "203.0.113.5") || !strings.Contains(s, "panic serving <ip>:5555") {
		t.Errorf("logged %q", s)
	}
}

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// flush writes everything added, and returns what f selects.
func flush(t *testing.T, s *Store, f Filter) []Entry {
	t.Helper()
	s.Sync()
	list, err := s.Entries(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func TestSummarize(t *testing.T) {
	s, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	until := time.UnixMilli(1_800_000_000_000)
	since := until.Add(-24 * time.Hour)
	at := func(ago time.Duration) time.Time { return until.Add(-ago) }
	anilist := func(ago time.Duration, level slog.Level) Entry {
		attrs := map[string]any{"target": "anilist", "status": 200, "sent_bytes": 100, "received_bytes": 1000}
		if level >= slog.LevelWarn {
			attrs["status"] = 429
		}
		return Entry{Time: at(ago), Kind: KindOutbound, Level: level, Msg: "anilist search", Attrs: attrs}
	}
	for _, e := range []Entry{
		{Time: at(25 * time.Hour), Kind: KindEvent, Level: slog.LevelError, Msg: "too old"},
		{Time: at(25 * time.Hour), Kind: KindBoot, Level: slog.LevelInfo, Msg: "server started"},
		{Time: at(20 * time.Hour), Kind: KindBoot, Level: slog.LevelInfo, Msg: "server started", Attrs: map[string]any{"previous": "no clean shutdown"}},
		{Time: at(time.Hour), Kind: KindEvent, Level: slog.LevelError, Msg: "boom"},
		{Time: at(time.Hour), Kind: KindEvent, Level: slog.LevelWarn, Msg: "hm"},
		{Time: at(time.Hour), Kind: KindEvent, Level: slog.LevelInfo, Msg: "fine"},
		{Time: at(time.Hour), Kind: KindStall, Level: slog.LevelError, Msg: "server stuck"},
		{Time: at(time.Hour), Kind: KindStall, Level: slog.LevelWarn, Msg: "server well again"},
		{Time: at(time.Hour), Kind: KindRequest, Level: slog.LevelWarn, Msg: "GET /x", Attrs: map[string]any{"status": 502}},
		{Time: at(time.Hour), Kind: KindRequest, Level: slog.LevelInfo, Msg: "GET /y", Attrs: map[string]any{"status": 200}},
		{Time: at(time.Hour), Kind: KindOutbound, Level: slog.LevelWarn, Msg: "helper status (per minute)", Attrs: map[string]any{"target": "helper"}},
		anilist(3*time.Hour-5*time.Minute, slog.LevelInfo), anilist(3*time.Hour-5*time.Minute, slog.LevelInfo),
		anilist(3*time.Hour-5*time.Minute, slog.LevelWarn), anilist(3*time.Hour-3*time.Minute, slog.LevelInfo),
		{Time: at(2 * time.Hour), Kind: KindSample, Level: slog.LevelDebug, Msg: "sample", Attrs: map[string]any{"cpu_pct": 12, "lag_ms": 3, "host_free_mb": 900}},
		{Time: at(2*time.Hour - 10*time.Second), Kind: KindSample, Level: slog.LevelDebug, Msg: "sample", Attrs: map[string]any{"cpu_pct": 40, "host_free_mb": 700, "room_memory_mb": 2100, "room_limit_mb": 3200}},
	} {
		s.Add(e)
	}
	s.Sync()
	sum, err := s.Summarize(context.Background(), since, until)
	if err != nil {
		t.Fatal(err)
	}
	want := Counts{Errors: 1, Warnings: 1, Stalls: 1, Requests: 2, FailedRequests: 1, AniList: 4, AniListFailed: 1,
		AniListLimited: 1, AniListPeak: 3, AniListSent: 400, AniListGot: 4000, Boots: 1, Crashes: 1}
	if sum.Counts != want {
		t.Errorf("counts %+v, want %+v", sum.Counts, want)
	}
	if sum.BucketMs != 10*60_000 || sum.LastBoot == nil || sum.LastBoot.Attrs["previous"] == nil || len(sum.Boots) != 1 {
		t.Errorf("bucket %d, last boot %+v", sum.BucketMs, sum.LastBoot)
	}
	// 3 calls in one minute, 1 two minutes later, in the bucket from 3 h ago.
	if len(sum.AniList) != 1 || sum.AniList[0] != (AniListBucket{Time: at(3 * time.Hour).UnixMilli(), Calls: 4, Failed: 1, Limited: 1, Peak: 3, Sent: 400, Got: 4000}) {
		t.Errorf("anilist %+v", sum.AniList)
	}
	if len(sum.Samples) != 1 || *sum.Samples[0].CPU != 40 || *sum.Samples[0].LagMs != 3 || *sum.Samples[0].HostFreeMB != 700 || sum.Samples[0].MemoryMB != nil ||
		*sum.Samples[0].RoomMemoryMB != 2100 || *sum.Samples[0].RoomLimitMB != 3200 {
		t.Errorf("samples %+v", sum.Samples)
	}
	var none *Store
	if sum, err := none.Summarize(context.Background(), since, until); err != nil || sum.Samples == nil || sum.LastBoot != nil {
		t.Errorf("off: %+v %v", sum, err)
	}
}

func TestWriteHistory(t *testing.T) {
	s, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.UnixMilli(1_800_000_000_000) // 08:00 UTC
	sample := func(ago time.Duration, attrs map[string]any) Entry {
		return Entry{Time: now.Add(-ago), Kind: KindSample, Level: slog.LevelDebug, Msg: "sample", Attrs: attrs}
	}
	for _, e := range []Entry{
		{Time: now.Add(-time.Hour), Kind: KindEvent, Level: slog.LevelWarn, Room: "default", Msg: "out of memory: programs ended", Attrs: map[string]any{"ended": 1}},
		sample(12*time.Minute, map[string]any{"memory_mb": 30}),
		sample(3*time.Minute, map[string]any{"memory_mb": 60, "host_free_mb": 700, "people": 3}),
		sample(2*time.Minute, map[string]any{"memory_mb": 40, "host_free_mb": 900, "room_memory_mb": 2100, "room_limit_mb": 3200}),
	} {
		s.Add(e)
	}
	s.Sync()
	var b strings.Builder
	if err := s.WriteHistory(context.Background(), &b, now); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"07:58:00: host_free_mb=900 memory_mb=40 room_limit_mb=3200 room_memory_mb=2100",
		// The two samples of 07:55 to 08:00 as one row: the most memory, the least free.
		"time   cpu%  serverMB  freeMB  roomMB/cap  people  lockMs  dbMs  reqMs  lagMs\n",
		"07:45     -        30       -  -/-              -",
		"07:55     -        60     700  2100/3200        3",
		"07:00     -        60     700  2100/3200        3",
		"01-15 07:00:00 WARN out of memory: programs ended room=default {\"ended\":1}",
	} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("history without %q:\n%s", want, b.String())
		}
	}
}
