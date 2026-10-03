// Package httpapi wires the HTTP surface: the JSON API, the room WebSocket,
// the neko reverse proxy and the embedded web UI.
package httpapi

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"sort"

	"cozycast/internal/room"
)

type Server struct {
	rooms map[string]*room.Room
	web   fs.FS
	log   *slog.Logger
}

func New(rooms map[string]*room.Room, web fs.FS) *Server {
	return &Server{rooms: rooms, web: web, log: slog.Default()}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/rooms", s.listRooms)
	mux.HandleFunc("GET /api/rooms/{room}/ws", s.roomSocket)
	mux.HandleFunc("/neko/{room}/{path...}", s.nekoProxy)
	mux.Handle("/", spaHandler(s.web))
	return mux
}

func (s *Server) room(r *http.Request) *room.Room {
	return s.rooms[r.PathValue("room")]
}

func (s *Server) listRooms(w http.ResponseWriter, r *http.Request) {
	type roomInfo struct {
		Name string `json:"name"`
	}
	list := make([]roomInfo, 0, len(s.rooms))
	for name := range s.rooms {
		list = append(list, roomInfo{Name: name})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	writeJSON(w, http.StatusOK, list)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
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
