package diag

import (
	"context"
	"log/slog"
	"slices"
)

// allowed is every attribute that may be logged. Others are left out, so
// that a new log call cannot leak someone's name or address by accident:
// to log something new, its key goes here, and only if it says nothing
// about anyone.
var allowed = map[string]bool{
	"err": true, "room": true, "component": true, "neko": true,
	"type": true, "status": true, "method": true, "route": true,
	"port": true, "addr": true, "address": true, "network": true, "tls": true,
	"socket": true, "project": true, "dir": true, "screen": true, "replaced": true,
	"state": true, "kind": true, "action": true, "attempt": true, "count": true,
	"duration_ms": true, "bytes": true,
	// What the watchlog asks AniList and the room helpers about.
	"anime": true, "episode": true, "target": true, "op": true, "caller": true,
	"outcome": true, "retry_after": true, "rate_remaining": true,
	// The legacy import's counts.
	"users": true, "rooms": true, "permissions": true, "invites": true,
	"logins": true, "avatars": true, "skipped": true,
}

// Allowed says whether attributes named key are logged.
func Allowed(key string) bool { return allowed[key] }

// Handler logs to out and keeps records from Info up in store (which may
// be nil), with what says who or what someone does taken out: attributes
// not on the allowed list, and in messages and values public IP addresses,
// URL paths and queries, e-mail addresses and tokens (Scrub).
type Handler struct {
	out   slog.Handler
	store *Store
	attrs []slog.Attr // scrubbed already
	group string      // prefix for keys, "a.b."
}

func NewHandler(out slog.Handler, store *Store) *Handler {
	return &Handler{out: out, store: store}
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= slog.LevelInfo || h.out.Enabled(ctx, level)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h2 := *h
	h2.attrs = slices.Clip(h.attrs)
	for _, a := range attrs {
		h2.attrs = appendSafe(h2.attrs, h.group, a)
	}
	return &h2
}

func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h2 := *h
	h2.group = h.group + name + "."
	return &h2
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	attrs := slices.Clip(h.attrs)
	r.Attrs(func(a slog.Attr) bool {
		attrs = appendSafe(attrs, h.group, a)
		return true
	})
	msg := Scrub(r.Message)
	var err error
	if h.out.Enabled(ctx, r.Level) {
		out := slog.NewRecord(r.Time, r.Level, msg, r.PC)
		out.AddAttrs(attrs...)
		err = h.out.Handle(ctx, out)
	}
	if h.store != nil && r.Level >= slog.LevelInfo {
		e := Entry{Time: r.Time, Kind: KindEvent, Level: r.Level, Msg: msg}
		for _, a := range attrs {
			if a.Key == "room" {
				e.Room = a.Value.String()
				continue
			}
			if e.Attrs == nil {
				e.Attrs = make(map[string]any, len(attrs))
			}
			e.Attrs[a.Key] = a.Value.Any()
		}
		h.store.Add(e)
	}
	return err
}

// appendSafe appends a, flattened and scrubbed, if its key is allowed.
func appendSafe(attrs []slog.Attr, group string, a slog.Attr) []slog.Attr {
	v := a.Value.Resolve()
	if v.Kind() == slog.KindGroup {
		prefix := group
		if a.Key != "" {
			prefix += a.Key + "."
		}
		for _, g := range v.Group() {
			attrs = appendSafe(attrs, prefix, g)
		}
		return attrs
	}
	if a.Key == "" || !allowed[a.Key] {
		return attrs
	}
	switch v.Kind() {
	case slog.KindString, slog.KindAny, slog.KindLogValuer:
		v = slog.StringValue(Scrub(v.String()))
	}
	return append(attrs, slog.Attr{Key: group + a.Key, Value: v})
}
