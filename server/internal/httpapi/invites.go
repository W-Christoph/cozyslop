package httpapi

import (
	"errors"
	"net/http"
	"time"
	"unicode/utf8"

	"cozycast/internal/store"
)

type inviteView struct {
	*store.Invite
	Valid bool   `json:"valid"`
	Path  string `json:"path"`
}

func toInviteView(i *store.Invite, now int64) inviteView {
	path := "/invite/" + i.Code
	if i.Temporary {
		path = "/access/" + i.Code
	}
	return inviteView{Invite: i, Valid: i.Valid(now), Path: path}
}

func (s *Server) adminCreateInvite(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	var req struct {
		Room             string `json:"room"`
		Temporary        bool   `json:"temporary"`
		Name             string `json:"name"`
		Remote           bool   `json:"remote"`
		Image            bool   `json:"image"`
		Upload           bool   `json:"upload"`
		MaxUses          *int   `json:"maxUses"`
		ExpiresInMinutes *int64 `json:"expiresInMinutes"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if s.hub.Room(req.Room) == nil {
		writeError(w, http.StatusNotFound, "Unknown room.")
		return
	}
	if utf8.RuneCountInString(req.Name) > 64 {
		writeError(w, http.StatusBadRequest, "Invite names are at most 64 characters.")
		return
	}
	if req.MaxUses != nil && *req.MaxUses < 1 {
		writeError(w, http.StatusBadRequest, "Maximum uses must be at least 1.")
		return
	}
	now := time.Now().Unix()
	if req.ExpiresInMinutes != nil && (*req.ExpiresInMinutes < 1 || *req.ExpiresInMinutes > (1<<63-1-now)/60) {
		writeError(w, http.StatusBadRequest, "Expiry minutes must be a positive number.")
		return
	}
	i := &store.Invite{Room: req.Room, Temporary: req.Temporary, Name: req.Name,
		Remote: req.Remote, Image: req.Image, Upload: req.Upload, MaxUses: req.MaxUses}
	if req.ExpiresInMinutes != nil {
		until := now + *req.ExpiresInMinutes*60
		i.ExpiresAt = &until
	}
	if err := s.store.CreateInvite(r.Context(), i); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toInviteView(i, now))
}

func (s *Server) adminListInvites(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	invites, err := s.store.ListInvites(r.Context(), r.URL.Query().Get("room"))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	list := make([]inviteView, 0, len(invites))
	now := time.Now().Unix()
	for _, i := range invites {
		list = append(list, toInviteView(i, now))
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) adminDeleteInvite(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	err := s.store.DeleteInvite(r.Context(), r.PathValue("code"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "This invite is invalid or has expired.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) checkInvite(w http.ResponseWriter, r *http.Request) {
	i, err := s.store.Invite(r.Context(), r.PathValue("code"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "This invite is invalid or has expired.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !i.Valid(time.Now().Unix()) {
		writeError(w, http.StatusNotFound, "This invite is invalid or has expired.")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Room      string `json:"room"`
		Temporary bool   `json:"temporary"`
	}{Room: i.Room, Temporary: i.Temporary})
}

func (s *Server) redeemInvite(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	i, err := s.store.RedeemInvite(r.Context(), r.PathValue("code"), u.ID)
	if errors.Is(err, store.ErrInvalidInvite) {
		writeError(w, http.StatusNotFound, "This invite is invalid or has expired.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.hub.PermissionsChanged(r.Context(), i.Room, u.ID)
	w.WriteHeader(http.StatusNoContent)
}
