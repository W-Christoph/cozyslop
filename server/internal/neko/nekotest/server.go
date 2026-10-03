// Package nekotest implements the neko API used by CozyCast, without a desktop.
package nekotest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"cozycast/internal/neko"
	"github.com/coder/websocket"
)

type member struct {
	password string
	profile  neko.Profile
}
type observer struct {
	conn   *websocket.Conn
	events chan any
}

// Call records a request in arrival order (including unsuccessful requests).
type Call struct{ Method, Path, MemberID string }

// Server is a concurrency-safe, in-memory neko server. New registers cleanup.
type Server struct {
	http              *httptest.Server
	apiToken          string
	mu                sync.Mutex
	healthy, implicit bool
	screen            neko.ScreenSize
	host              string
	members           map[string]member
	tokens            map[string]string
	observers         map[*observer]bool
	nextToken         uint64
	calls             []Call
	changed           chan struct{}
}

func New(t testing.TB, apiToken string) *Server {
	t.Helper()
	s := &Server{apiToken: apiToken, healthy: true, members: make(map[string]member), tokens: make(map[string]string), observers: make(map[*observer]bool), changed: make(chan struct{})}
	s.http = httptest.NewServer(http.HandlerFunc(s.serveHTTP))
	t.Cleanup(func() { s.Restart(); s.http.Close() })
	return s
}
func (s *Server) URL() string { return s.http.URL }
func cloneProfile(p neko.Profile) neko.Profile {
	// A JSON round trip also detaches nested plugin values from the caller.
	b, _ := json.Marshal(p)
	var out neko.Profile
	_ = json.Unmarshal(b, &out)
	return out
}
func (s *Server) Member(id string) (neko.Profile, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.members[id]
	return cloneProfile(m.profile), ok
}
func (s *Server) Members() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.members))
	for id := range s.members {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
func (s *Server) ImplicitHosting() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.implicit }

// Screen is the last screen size set through the API (zero if never).
func (s *Server) Screen() neko.ScreenSize { s.mu.Lock(); defer s.mu.Unlock(); return s.screen }

// Screens are the resolutions the fake desktop supports.
var Screens = []neko.ScreenSize{{Width: 1920, Height: 1080, Rate: 30}, {Width: 1280, Height: 720, Rate: 30}, {Width: 800, Height: 600, Rate: 30}}

func (s *Server) SetHealthy(healthy bool) { s.mu.Lock(); defer s.mu.Unlock(); s.healthy = healthy }
func (s *Server) Calls() []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Call(nil), s.calls...)
}
func (s *Server) signalLocked() { close(s.changed); s.changed = make(chan struct{}) }

// WaitObservers waits for exactly n active observer sockets, or ctx cancellation.
func (s *Server) WaitObservers(ctx context.Context, n int) error {
	for {
		s.mu.Lock()
		count, changed := len(s.observers), s.changed
		s.mu.Unlock()
		if count == n {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}
func hostPayload(id string) any { return map[string]any{"has_host": id != "", "host_id": id} }
func (s *Server) setHostLocked(id string) {
	s.host = id
	for o := range s.observers {
		// A stalled observer is disconnected rather than silently losing events.
		select {
		case o.events <- map[string]any{"event": "control/host", "payload": hostPayload(id)}:
		default:
			_ = o.conn.CloseNow()
			delete(s.observers, o)
			s.signalLocked()
		}
	}
}
func (s *Server) SetHost(id string) { s.mu.Lock(); defer s.mu.Unlock(); s.setHostLocked(id) }
func (s *Server) ClearHost()        { s.SetHost("") }

// Restart invalidates every session and closes observer sockets; the HTTP URL
// stays stable so clients can reconnect just as after a container restart.
func (s *Server) Restart() {
	s.mu.Lock()
	observers := s.observers
	s.observers = make(map[*observer]bool)
	s.members = make(map[string]member)
	s.tokens = make(map[string]string)
	s.host = ""
	s.implicit = false
	s.signalLocked()
	s.mu.Unlock()
	for o := range observers {
		_ = o.conn.CloseNow()
	}
}
func respond(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	if json.NewDecoder(r.Body).Decode(value) != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return false
	}
	return true
}
func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/ws" && r.Method == http.MethodGet {
		s.serveWS(w, r)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, Call{Method: r.Method, Path: r.URL.Path})
	if r.URL.Path == "/health" && r.Method == http.MethodGet {
		if !s.healthy {
			http.Error(w, "unhealthy", http.StatusServiceUnavailable)
		}
		return
	}
	if r.URL.Path == "/api/login" && r.Method == http.MethodPost {
		var in struct{ Username, Password string }
		if !decode(w, r, &in) {
			return
		}
		s.calls[len(s.calls)-1].MemberID = in.Username
		m, ok := s.members[in.Username]
		if !ok || m.password != in.Password || !m.profile.CanLogin {
			http.Error(w, "bad credentials", http.StatusUnauthorized)
			return
		}
		s.nextToken++
		token := fmt.Sprintf("session-%d", s.nextToken)
		s.tokens[token] = in.Username
		respond(w, map[string]string{"token": token})
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+s.apiToken {
		http.Error(w, "admin token required", http.StatusUnauthorized)
		return
	}
	switch {
	case r.URL.Path == "/api/members" && r.Method == http.MethodGet:
		members := make([]neko.Member, 0, len(s.members))
		for id, m := range s.members {
			members = append(members, neko.Member{ID: id, Profile: m.profile})
		}
		sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
		respond(w, members)
	case r.URL.Path == "/api/members" && r.Method == http.MethodPost:
		var in struct {
			Username, Password string
			Profile            neko.Profile
		}
		if !decode(w, r, &in) {
			return
		}
		s.calls[len(s.calls)-1].MemberID = in.Username
		if _, exists := s.members[in.Username]; exists {
			http.Error(w, "member exists", http.StatusConflict)
			return
		}
		s.members[in.Username] = member{in.Password, in.Profile}
	case strings.HasPrefix(r.URL.Path, "/api/members/") && (r.Method == http.MethodPost || r.Method == http.MethodDelete):
		id := strings.TrimPrefix(r.URL.Path, "/api/members/")
		s.calls[len(s.calls)-1].MemberID = id
		m, ok := s.members[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			var p neko.Profile
			if !decode(w, r, &p) {
				return
			}
			m.profile = p
			s.members[id] = m
			return
		}
		delete(s.members, id)
		for token, memberID := range s.tokens {
			if memberID == id {
				delete(s.tokens, token)
			}
		}
		if s.host == id {
			s.setHostLocked("")
		}
	case r.URL.Path == "/api/room/settings" && r.Method == http.MethodPost:
		var in struct {
			Implicit bool `json:"implicit_hosting"`
		}
		if decode(w, r, &in) {
			s.implicit = in.Implicit
		}
	case r.URL.Path == "/api/room/screen" && r.Method == http.MethodPost:
		var in neko.ScreenSize
		if decode(w, r, &in) {
			if !slices.Contains(Screens, in) {
				http.Error(w, "invalid screen configuration", http.StatusUnprocessableEntity)
				return
			}
			s.screen = in
		}
	case r.URL.Path == "/api/room/screen/configurations" && r.Method == http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Screens)
	case r.URL.Path == "/api/room/control/reset" && r.Method == http.MethodPost:
		s.setHostLocked("")
	default:
		http.NotFound(w, r)
	}
}
func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.calls = append(s.calls, Call{Method: r.Method, Path: r.URL.Path})
	id, valid := s.tokens[r.URL.Query().Get("token")]
	m, exists := s.members[id]
	if !valid || !exists || id != neko.ObserverID || !m.profile.IsAdmin {
		s.mu.Unlock()
		http.Error(w, "observer token required", http.StatusUnauthorized)
		return
	}
	// Accept and register under the lock so Restart cannot retain an old token
	// connection and system/init always precedes concurrent host changes.
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		s.mu.Unlock()
		return
	}
	o := &observer{conn: conn, events: make(chan any, 1024)}
	o.events <- map[string]any{"event": "system/init", "payload": map[string]any{"control_host": hostPayload(s.host)}}
	s.observers[o] = true
	s.signalLocked()
	s.mu.Unlock()
	defer func() { _ = conn.CloseNow(); s.mu.Lock(); delete(s.observers, o); s.signalLocked(); s.mu.Unlock() }()
	ctx := conn.CloseRead(r.Context())
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-o.events:
			data, _ := json.Marshal(event)
			writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Write(writeCtx, websocket.MessageText, data)
			cancel()
			if err != nil {
				return
			}
		}
	}
}
