// Package httpapi wires the HTTP surface: the JSON API, the room WebSocket,
// the neko reverse proxy and the embedded web UI.
package httpapi

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"cozycast/internal/auth"
	"cozycast/internal/hub"
	"cozycast/internal/ratelimit"
	"cozycast/internal/rights"
	"cozycast/internal/store"
)

type Deps struct {
	Store *store.Store
	Auth  *auth.Service
	Hub   *hub.Hub
	Web   fs.FS
}

type Server struct {
	store *store.Store
	auth  *auth.Service
	hub   *hub.Hub
	web   fs.FS
	log   *slog.Logger

	loginLimit    *ratelimit.Limiter // per IP: login attempts
	registerLimit *ratelimit.Limiter // per IP: account creations
}

func New(d Deps) *Server {
	return &Server{
		store:         d.Store,
		auth:          d.Auth,
		hub:           d.Hub,
		web:           d.Web,
		log:           slog.Default(),
		loginLimit:    ratelimit.New(10, 30*time.Second),
		registerLimit: ratelimit.New(3, 10*time.Minute),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("POST /api/auth/register", s.register)
	mux.HandleFunc("GET /api/me", s.getMe)
	mux.HandleFunc("PATCH /api/me", s.updateMe)
	mux.HandleFunc("POST /api/me/password", s.changePassword)
	mux.HandleFunc("GET /api/rooms", s.listRooms)
	mux.HandleFunc("GET /api/rooms/{room}/ws", s.roomSocket)
	mux.HandleFunc("/neko/{room}/{path...}", s.nekoProxy)
	mux.Handle("/", spaHandler(s.web))
	return sameOrigin(mux)
}

// sameOrigin rejects state-changing requests sent by other sites. Session
// cookies are SameSite=Lax already; this also covers same-site subdomains
// and old browsers.
func sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || u.Host != r.Host {
					writeError(w, http.StatusForbidden, "cross-origin request rejected")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// listRooms returns the rooms the requester may see. Hidden rooms are only
// listed for people who can join them.
func (s *Server) listRooms(w http.ResponseWriter, r *http.Request) {
	id, err := s.auth.Identify(w, r)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	type roomInfo struct {
		Name      string `json:"name"`
		Access    string `json:"access"`
		UserCount int    `json:"userCount"`
		Open      bool   `json:"open"` // the requester may join
	}
	list := []roomInfo{}
	now := time.Now().Unix()
	for _, rm := range s.hub.Rooms() {
		in := rights.Input{Room: rm.Settings(), User: id.User, Now: now}
		if id.User != nil {
			if in.Perm, err = s.store.Permission(r.Context(), rm.Name, id.User.ID); err != nil {
				s.internalError(w, r, err)
				return
			}
		} else if ban, err := s.store.AnonBan(r.Context(), rm.Name, id.AnonID, id.IP); err == nil {
			in.AnonBan = ban
		} else if !errors.Is(err, store.ErrNotFound) {
			s.internalError(w, r, err)
			return
		}
		open := rights.Admit(in) == nil
		if in.Room.Hidden && !open {
			continue
		}
		list = append(list, roomInfo{Name: rm.Name, Access: in.Room.Access, UserCount: rm.UserCount(), Open: open})
	}
	writeJSON(w, http.StatusOK, list)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError sends {"error": msg}. Messages are shown to users as-is.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// internalError logs err and sends a generic 500.
func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	writeError(w, http.StatusInternalServerError, "Something went wrong.")
}

const maxJSONBody = 64 << 10

// readJSON decodes a JSON request body into v, answering 400 itself on
// failure.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "Request too large.")
		} else {
			writeError(w, http.StatusBadRequest, "Invalid request.")
		}
		return false
	}
	return true
}

// spaHandler serves static files and falls back to index.html so client-side
// routes like /room/default work on reload.
func spaHandler(web fs.FS) http.Handler {
	files := http.FileServerFS(web)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Path[1:]
		if name == "" {
			name = "index.html"
		}
		if _, err := fs.Stat(web, name); err != nil {
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
}
