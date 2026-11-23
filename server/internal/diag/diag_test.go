package diag

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
		"err", fmt.Errorf("dial 198.51.100.1: refused"), "username", "alice", "anime", 154587,
		slog.Group("g", "identity", "u:12", "status", 503))
	log.Debug("client joined", "identity", "u:12")
	log.Info("plain")
	for _, leak := range []string{"c123", "alice", "u:12", "203.0.113.7", "198.51.100.1", "client=", "username", "identity"} {
		if strings.Contains(out.String(), leak) {
			t.Errorf("stderr has %q: %s", leak, out.String())
		}
	}
	for _, want := range []string{"room=default", "anime=154587", "g.status=503", `err="dial <ip>: refused"`, "client joined"} {
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
	if e.Attrs["anime"] != float64(154587) || e.Attrs["err"] != "dial <ip>: refused" || e.Attrs["g.status"] != float64(503) || len(e.Attrs) != 3 {
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
