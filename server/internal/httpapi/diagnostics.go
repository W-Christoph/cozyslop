package httpapi

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cozycast/internal/diag"
	"cozycast/internal/store"
)

// requireAdminReadOnly is requireAdmin without writing to the database, so
// that the diagnostics answer while it is busy (see auth.ReadOnlyUser).
func (s *Server) requireAdminReadOnly(w http.ResponseWriter, r *http.Request) *store.User {
	u, err := s.auth.ReadOnlyUser(r)
	switch {
	case err != nil:
		s.internalError(w, r, err)
		return nil
	case u == nil:
		writeError(w, http.StatusUnauthorized, "Please log in.")
		return nil
	case !u.Admin:
		writeError(w, http.StatusForbidden, "Admins only.")
		return nil
	}
	return u
}

// adminDiagnostics lists what the server kept for troubleshooting, newest
// first. Filters, all optional: kind and target (comma-separated; target
// selects outbound entries: anilist, helper), level (lowest one:
// debug, info, warn, error), room, since and until (unix ms), q (text in
// the message or attributes), before (an entry id: the next page), limit.
func (s *Server) adminDiagnostics(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminReadOnly(w, r) == nil {
		return
	}
	q := r.URL.Query()
	f := diag.Filter{Room: q.Get("room"), Text: q.Get("q")}
	if kinds := q.Get("kind"); kinds != "" {
		f.Kinds = strings.Split(kinds, ",")
	}
	if targets := q.Get("target"); targets != "" {
		f.Targets = strings.Split(targets, ",")
	}
	if level := q.Get("level"); level != "" {
		var l slog.Level
		if err := l.UnmarshalText([]byte(level)); err != nil {
			writeError(w, http.StatusBadRequest, "Unknown level.")
			return
		}
		f.MinLevel = &l
	}
	for _, p := range []struct {
		name string
		set  func(int64)
	}{
		{"since", func(v int64) { f.Since = time.UnixMilli(v) }},
		{"until", func(v int64) { f.Until = time.UnixMilli(v) }},
		{"before", func(v int64) { f.Before = v }},
		{"limit", func(v int64) { f.Limit = int(min(v, 1000)) }},
	} {
		if text := q.Get(p.name); text != "" {
			v, err := strconv.ParseInt(text, 10, 64)
			if err != nil || v < 0 {
				writeError(w, http.StatusBadRequest, "Invalid "+p.name+".")
				return
			}
			p.set(v)
		}
	}
	entries, err := s.diagnostics.Entries(r.Context(), f)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	size, err := s.diagnostics.Size(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":            s.diagnostics != nil,
		"retentionDays":      int(diag.Retention.Hours() / 24),
		"sizeBytes":          size,
		"dropped":            s.diagnostics.Dropped(),
		"tlsHandshakeErrors": diag.TLSHandshakeErrors.Load(),
		"entries":            entries,
	})
}

// adminDiagnosticsSummary counts what happened in the last hours (1 to
// 168, default 24), for the admin page.
func (s *Server) adminDiagnosticsSummary(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminReadOnly(w, r) == nil {
		return
	}
	hours := 24
	if text := r.URL.Query().Get("hours"); text != "" {
		v, err := strconv.Atoi(text)
		if err != nil || v < 1 || v > 168 {
			writeError(w, http.StatusBadRequest, "Invalid hours.")
			return
		}
		hours = v
	}
	now := time.Now()
	sum, err := s.diagnostics.Summarize(r.Context(), now.Add(-time.Duration(hours)*time.Hour), now)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	size, err := s.diagnostics.Size(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		diag.Summary
		Enabled            bool  `json:"enabled"`
		RetentionDays      int   `json:"retentionDays"`
		SampleDays         int   `json:"sampleDays"`
		SizeBytes          int64 `json:"sizeBytes"`
		Dropped            int64 `json:"dropped"`
		TLSHandshakeErrors int64 `json:"tlsHandshakeErrors"`
	}{sum, s.diagnostics != nil, int(diag.Retention.Hours() / 24), int(diag.SampleRetention.Hours() / 24),
		size, s.diagnostics.Dropped(), diag.TLSHandshakeErrors.Load()})
}

// adminDiagnosticsDump answers a stall report's goroutine dump (the
// "dump" of a stall entry), as text.
func (s *Server) adminDiagnosticsDump(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminReadOnly(w, r) == nil {
		return
	}
	dump, err := s.diagnostics.Dump(r.PathValue("name"))
	if errors.Is(err, diag.ErrNoDump) {
		writeError(w, http.StatusNotFound, "No such dump.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer dump.Close()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.Copy(w, dump)
}

// adminClearDiagnostics removes everything kept for troubleshooting.
func (s *Server) adminClearDiagnostics(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminReadOnly(w, r) == nil {
		return
	}
	if err := s.diagnostics.Clear(r.Context()); err != nil {
		s.internalError(w, r, err)
		return
	}
	s.log.Info("diagnostics cleared")
	w.WriteHeader(http.StatusNoContent)
}

// adminDiagnosticsSnapshot answers the server's state now as text, to paste
// into a message: the machine's memory, the server's, each room's and what
// uses it there, then what the diagnostics kept of the last hours.
func (s *Server) adminDiagnosticsSnapshot(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminReadOnly(w, r) == nil {
		return
	}
	now := time.Now()
	var b strings.Builder
	sys := diag.WriteSystem(&b, now)

	fmt.Fprintln(&b, "\n== Rooms")
	rooms := s.hub.MemoryDetails(r.Context())
	if len(rooms) == 0 {
		fmt.Fprintln(&b, "(none seen yet)")
	}
	var local int64
	for _, room := range rooms {
		people := "people unknown"
		if room.People == 1 {
			people = "1 person"
		} else if room.People >= 0 {
			people = fmt.Sprintf("%d people", room.People)
		}
		state := "awake"
		if room.Asleep {
			state = "asleep (programs paused, memory kept)"
		}
		fmt.Fprintf(&b, "%s (%s, %s, %s)", room.Name, room.Source, people, state)
		d := room.Detail
		if d == nil {
			fmt.Fprintf(&b, ": %s\n", room.Err)
			continue
		}
		if room.Source == "configured" {
			local += d.Used
		}
		fmt.Fprintf(&b, ": used %s", diag.MB(d.Used))
		if d.Limit != nil && *d.Limit > 0 {
			fmt.Fprintf(&b, " of its cap %s (%d %%)", diag.MB(*d.Limit), d.Used*100 / *d.Limit)
		} else {
			fmt.Fprint(&b, ", no cap")
		}
		if d.Peak > 0 {
			fmt.Fprintf(&b, ", peak %s (file cache included)", diag.MB(d.Peak))
		}
		fmt.Fprintln(&b)
		var parts []string
		for _, k := range []string{"anon", "file", "shmem", "kernel", "pagetables", "sock"} {
			if v, ok := d.Stat[k]; ok {
				parts = append(parts, k+" "+diag.MB(v))
			}
		}
		if d.Shm != nil {
			parts = append(parts, fmt.Sprintf("/dev/shm %s of %s", diag.MB(d.Shm.Used), diag.MB(d.Shm.Size)))
		}
		if d.Swap > 0 {
			parts = append(parts, "swap "+diag.MB(d.Swap))
		}
		fmt.Fprintf(&b, "  %s\n", strings.Join(parts, ", "))
		if d.Events["max"] > 0 || d.OOMKills > 0 {
			fmt.Fprintf(&b, "  at the cap %d times since the room started, programs ended %d\n", d.Events["max"], d.OOMKills)
		}
		if len(d.Programs) > 0 {
			fmt.Fprintln(&b, "  programs by resident memory (a page several share counts in each):")
		}
		for _, p := range d.Programs {
			fmt.Fprintf(&b, "    %-20s x%-3d %8s  anon %s, shared memory %s\n", diag.Scrub(p.Name), p.Count, diag.MB(p.RSS), diag.MB(p.Anon), diag.MB(p.Shmem))
		}
	}

	if sys.HostTotal > 0 {
		used, server := sys.HostTotal-sys.HostAvailable, "unknown"
		if sys.ServerUsed > 0 {
			server = diag.MB(sys.ServerUsed)
		}
		fmt.Fprintf(&b, "\n== Where the machine's memory goes (roughly)\nused %s = server %s + rooms in compose.yaml %s + the rest %s (Docker, the system, other programs; rooms added or paired in Admin are left out, they may run elsewhere)\n",
			diag.MB(used), server, diag.MB(local), diag.MB(max(0, used-sys.ServerUsed-local)))
	}

	if err := s.diagnostics.WriteHistory(r.Context(), &b, now); err != nil {
		fmt.Fprintf(&b, "\n(history unreadable: %s)\n", diag.Scrub(err.Error()))
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, b.String())
}
