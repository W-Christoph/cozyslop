package diag

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path"
	"strconv"
	"sync"
	"syscall"
	"time"
)

type callKey struct{}

// call is what a request is for, carried in its context.
type call struct{ op, caller, room string }

// WithCaller says which part of the server asks, and for which room ("" if
// none), of the outbound requests made with ctx.
func WithCaller(ctx context.Context, caller, room string) context.Context {
	c, _ := ctx.Value(callKey{}).(call)
	c.caller, c.room = caller, room
	return context.WithValue(ctx, callKey{}, c)
}

// WithOp says what the outbound requests made with ctx ask, e.g. "search".
func WithOp(ctx context.Context, op string) context.Context {
	c, _ := ctx.Value(callKey{}).(call)
	c.op = op
	return context.WithValue(ctx, callKey{}, c)
}

// Transport keeps each request through it as an outbound entry: what it
// was for (WithCaller, WithOp; else the last part of its path), how long it
// took, its status, AniList's rate limit headers, and how it failed. With
// Summarize, frequent requests (polling) are kept as one entry per minute,
// room and operation instead.
type Transport struct {
	Base      http.RoundTripper // nil = http.DefaultTransport
	Store     *Store
	Target    string // e.g. "anilist", "helper"
	Room      string // the room it serves, if not in the context
	Summarize bool
}

func (t Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	if t.Store == nil {
		return base.RoundTrip(req)
	}
	start := time.Now()
	res, err := base.RoundTrip(req)
	d := time.Since(start)

	c, _ := req.Context().Value(callKey{}).(call)
	if c.op == "" {
		c.op = path.Base(req.URL.Path)
	}
	if c.room == "" {
		c.room = t.Room
	}
	status := 0
	if res != nil {
		status = res.StatusCode
	}
	if t.Summarize {
		t.Store.summarize(t.Target, c.op, c.room, d, status, err)
		return res, err
	}
	attrs := map[string]any{"target": t.Target, "op": c.op, "duration_ms": d.Milliseconds()}
	if c.caller != "" {
		attrs["caller"] = c.caller
	}
	if req.ContentLength > 0 {
		attrs["sent_bytes"] = req.ContentLength
	}
	level := slog.LevelInfo
	if err != nil {
		attrs["err"], attrs["kind"] = Scrub(err.Error()), ErrorKind(err)
		level = slog.LevelWarn
	} else {
		attrs["status"] = status
		if status >= 400 {
			level = slog.LevelWarn
		}
		if v, err := strconv.Atoi(res.Header.Get("X-RateLimit-Remaining")); err == nil {
			attrs["rate_remaining"] = v
		}
		if v := res.Header.Get("Retry-After"); v != "" {
			attrs["retry_after"] = Scrub(v)
		}
	}
	e := Entry{Time: start, Kind: KindOutbound, Level: level, Room: c.room, Msg: t.Target + " " + c.op, Attrs: attrs}
	if res == nil || res.Body == nil {
		t.Store.Add(e)
		return res, err
	}
	// Kept once the answer has been read: how much came back, and how long
	// that took.
	res.Body = &countedBody{ReadCloser: res.Body, done: func(n int64) {
		e.Attrs["received_bytes"], e.Attrs["duration_ms"] = n, time.Since(start).Milliseconds()
		t.Store.Add(e)
	}}
	return res, err
}

// countedBody counts what is read from a response, and tells done when it
// is closed.
type countedBody struct {
	io.ReadCloser
	n    int64
	done func(n int64)
	once sync.Once
}

func (b *countedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.n += int64(n)
	return n, err
}

func (b *countedBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(func() { b.done(b.n) })
	return err
}

// ErrorKind sorts a request's error: timeout, dns, refused, reset, tls,
// canceled or other.
func ErrorKind(err error) string {
	var dns *net.DNSError
	var netErr net.Error
	var certErr *tls.CertificateVerificationError
	var alert tls.AlertError
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.As(err, &dns):
		return "dns"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		return "timeout"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "refused"
	case errors.Is(err, syscall.ECONNRESET):
		return "reset"
	case errors.As(err, &certErr), errors.As(err, &alert):
		return "tls"
	}
	return "other"
}

type sumKey struct{ target, op, room string }

type sum struct {
	start            time.Time
	count, errors    int
	total, longest   time.Duration
	lastErr, errKind string
	lastStatus       int
}

func (s *Store) summarize(target, op, room string, d time.Duration, status int, err error) {
	s.sumMu.Lock()
	defer s.sumMu.Unlock()
	if s.sums == nil {
		s.sums = make(map[sumKey]*sum)
	}
	k := sumKey{target, op, room}
	v := s.sums[k]
	if v == nil {
		v = &sum{start: s.now()}
		s.sums[k] = v
	}
	v.count++
	v.total += d
	v.longest = max(v.longest, d)
	v.lastStatus = status
	if err != nil || status >= 400 {
		v.errors++
		if err != nil {
			v.lastErr, v.errKind = Scrub(err.Error()), ErrorKind(err)
		}
	}
}

// summaries takes the summarized requests so far, as entries.
func (s *Store) summaries() []Entry {
	s.sumMu.Lock()
	sums := s.sums
	s.sums = nil
	s.sumMu.Unlock()
	var list []Entry
	for k, v := range sums {
		attrs := map[string]any{"target": k.target, "op": k.op, "count": v.count, "errors": v.errors,
			"avg_ms": (v.total / time.Duration(v.count)).Milliseconds(), "max_ms": v.longest.Milliseconds(),
			"status": v.lastStatus}
		level := slog.LevelInfo
		if v.errors > 0 {
			level = slog.LevelWarn
			if v.lastErr != "" {
				attrs["err"], attrs["kind"] = v.lastErr, v.errKind
			}
		}
		list = append(list, Entry{Time: v.start, Kind: KindOutbound, Level: level, Room: k.room, Msg: k.target + " " + k.op + " (per minute)", Attrs: attrs})
	}
	return list
}
