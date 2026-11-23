package httpapi

import (
	"errors"
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
// first. Filters, all optional: kind (comma-separated), level (lowest one:
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
