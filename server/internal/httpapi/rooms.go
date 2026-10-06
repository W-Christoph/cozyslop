package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"

	"cozycast/internal/config"
	"cozycast/internal/hub"
	"cozycast/internal/store"
)

// adminRoom leaves out the neko address: it can be a home machine's, and
// admins set it without needing to see it again (like the token).
type adminRoom struct {
	Name      string `json:"name"`
	Source    string `json:"source"`
	Connected bool   `json:"connected"`
	UserCount int    `json:"userCount"`
}

func toAdminRoom(r *hub.Room) adminRoom {
	return adminRoom{r.Name, r.Source, r.Neko().Connected(), r.UserCount()}
}

func newRoomToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func (s *Server) adminListRooms(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	list := []adminRoom{}
	for _, room := range s.hub.Rooms() {
		list = append(list, toAdminRoom(room))
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) adminCreateRoom(w http.ResponseWriter, r *http.Request) {
	actor := s.requireAdmin(w, r)
	if actor == nil {
		return
	}
	var req struct {
		Name    string `json:"name"`
		NekoURL string `json:"nekoUrl"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if !config.ValidRoomName(req.Name) {
		writeError(w, http.StatusBadRequest, "Room names must contain only letters, digits, underscores or hyphens.")
		return
	}
	if err := config.ValidateNekoURL(req.NekoURL); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.roomMu.Lock()
	defer s.roomMu.Unlock()
	if s.hub.Room(req.Name) != nil {
		writeError(w, http.StatusConflict, "That room already exists.")
		return
	}
	token, err := newRoomToken()
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	rc, err := s.buildRoom(req.Name, req.NekoURL, token)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	rc.Source = "registered"
	room := store.RegisteredRoom{Name: req.Name, NekoURL: req.NekoURL, NekoToken: token, CreatedBy: &actor.ID}
	// Complete DB/runtime changes even if the requester disconnects midway.
	ctx := context.WithoutCancel(r.Context())
	if err := s.store.CreateRegisteredRoom(ctx, &room); err != nil {
		if errors.Is(err, store.ErrRoomExists) {
			writeError(w, http.StatusConflict, "That room already exists.")
		} else {
			s.internalError(w, r, err)
		}
		return
	}
	if err := s.hub.Add(ctx, rc); err != nil {
		_ = s.store.DeleteRegisteredRoom(ctx, req.Name)
		s.internalError(w, r, err)
		return
	}
	s.writeRoomToken(w, http.StatusCreated, req.Name, token)
}

// registeredRoom rejects changes to configuration-owned rooms, including
// registrations shadowed by COZYCAST_ROOMS at startup. Caller holds roomMu.
func (s *Server) registeredRoom(w http.ResponseWriter, r *http.Request) (store.RegisteredRoom, bool) {
	name := r.PathValue("room")
	if room := s.hub.Room(name); room != nil && room.Source == "configured" {
		writeError(w, http.StatusConflict, "This room is managed in COZYCAST_ROOMS.")
		return store.RegisteredRoom{}, false
	}
	room, err := s.store.RegisteredRoom(r.Context(), name)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "Unknown room.")
		return room, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return room, false
	}
	return room, true
}

func (s *Server) adminChangeRoom(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	s.roomMu.Lock()
	defer s.roomMu.Unlock()
	room, ok := s.registeredRoom(w, r)
	if !ok {
		return
	}
	old := room
	rotating := r.Method == http.MethodPost
	if rotating {
		token, err := newRoomToken()
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		room.NekoToken = token
	} else {
		var req struct {
			NekoURL string `json:"nekoUrl"`
		}
		if !readJSON(w, r, &req) {
			return
		}
		if err := config.ValidateNekoURL(req.NekoURL); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		room.NekoURL = req.NekoURL
	}
	rc, err := s.buildRoom(room.Name, room.NekoURL, room.NekoToken)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	rc.Source = "registered"
	ctx := context.WithoutCancel(r.Context())
	if err := s.store.UpdateRegisteredRoom(ctx, room); err != nil {
		s.internalError(w, r, err)
		return
	}
	// The old runtime must finish before the replacement cleans neko members.
	s.hub.Remove(room.Name, "room_changed")
	if err := s.hub.Add(ctx, rc); err != nil {
		_ = s.store.UpdateRegisteredRoom(ctx, old)
		oldRC, buildErr := s.buildRoom(old.Name, old.NekoURL, old.NekoToken)
		if buildErr == nil {
			oldRC.Source = "registered"
			_ = s.hub.Add(ctx, oldRC)
		}
		s.internalError(w, r, err)
		return
	}
	if rotating {
		s.writeRoomToken(w, http.StatusOK, room.Name, room.NekoToken)
	} else {
		writeJSON(w, http.StatusOK, toAdminRoom(s.hub.Room(room.Name)))
	}
}

func (s *Server) writeRoomToken(w http.ResponseWriter, status int, name, token string) {
	writeJSON(w, status, struct {
		adminRoom
		NekoToken string `json:"nekoToken"`
	}{toAdminRoom(s.hub.Room(name)), token})
}

func (s *Server) adminDeleteRoom(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	s.roomMu.Lock()
	defer s.roomMu.Unlock()
	room, ok := s.registeredRoom(w, r)
	if !ok {
		return
	}
	ctx := context.WithoutCancel(r.Context())
	if err := s.store.DeleteRegisteredRoom(ctx, room.Name); err != nil {
		s.internalError(w, r, err)
		return
	}
	s.hub.Remove(room.Name, "not_found")
	w.WriteHeader(http.StatusNoContent)
}
