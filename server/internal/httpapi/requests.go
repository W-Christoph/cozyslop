package httpapi

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cozycast/internal/diag"
)

// slowRequest is when a request is kept in the diagnostics although it
// did not fail.
const slowRequest = time.Second

// requests are the requests under way, for the watchdog.
type requests struct {
	mu     sync.Mutex
	nextID int64
	open   map[int64]*openRequest
	total  atomic.Int64
}

type openRequest struct {
	route string
	start time.Time
	long  bool
}

// longLived are the routes whose requests last as long as someone uses
// them (WebSockets, the neko proxy) or as long as an upload takes.
func longLived(route string) bool {
	return strings.HasSuffix(route, "/ws") || strings.HasPrefix(route, "/neko/") ||
		route == "POST /api/rooms/{room}/media" || route == "POST /api/me/avatar"
}

// track keeps the requests under way, and failed and slow ones in the
// diagnostics, by their route (the pattern they matched), never their
// path: paths have usernames and invite codes in them.
func (s *Server) track(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, route := mux.Handler(r)
		if route == "" {
			route = "other"
		}
		req := &openRequest{route: route, start: time.Now(), long: longLived(route)}
		s.reqs.mu.Lock()
		if s.reqs.open == nil {
			s.reqs.open = make(map[int64]*openRequest)
		}
		s.reqs.nextID++
		id := s.reqs.nextID
		s.reqs.open[id] = req
		s.reqs.mu.Unlock()
		s.reqs.total.Add(1)

		sw := &statusWriter{ResponseWriter: w}
		defer func() {
			s.reqs.mu.Lock()
			delete(s.reqs.open, id)
			s.reqs.mu.Unlock()
			d := time.Since(req.start)
			if sw.status < 500 && (req.long || d < slowRequest) {
				return
			}
			attrs := map[string]any{"method": r.Method, "route": route, "status": sw.status, "duration_ms": d.Milliseconds()}
			level := slog.LevelInfo
			if sw.status >= 500 {
				level = slog.LevelWarn
			}
			s.diagnostics.Add(diag.Entry{Time: req.start, Kind: diag.KindRequest, Level: level,
				Room: r.PathValue("room"), Msg: route, Attrs: attrs})
		}()
		mux.ServeHTTP(sw, r)
	})
}

// Requests are the requests under way (diag.Watchdog).
func (s *Server) Requests() []diag.Request {
	now := time.Now()
	s.reqs.mu.Lock()
	defer s.reqs.mu.Unlock()
	list := make([]diag.Request, 0, len(s.reqs.open))
	for _, r := range s.reqs.open {
		list = append(list, diag.Request{Route: r.route, Age: now.Sub(r.start), Long: r.long})
	}
	return list
}

// Gauges adds the server's numbers to a health sample (diag.Watchdog).
func (s *Server) Gauges(sample map[string]any) {
	sample["connections"] = diag.OpenConns.Load()
	sample["requests_total"] = s.reqs.total.Load()
}

// statusWriter notes the status of the answer. Unwrap lets
// http.ResponseController, the reverse proxy and WebSockets reach the
// connection beneath.
type statusWriter struct {
	http.ResponseWriter
	status int // 0 until the answer starts (and for taken-over connections)
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
