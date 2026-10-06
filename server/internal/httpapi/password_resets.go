package httpapi

import (
	"errors"
	"net/http"

	"cozycast/internal/auth"
	"cozycast/internal/store"
)

const invalidPasswordReset = "This reset link is invalid or has expired. Ask a moderator for a new link."

func (s *Server) adminCreatePasswordReset(w http.ResponseWriter, r *http.Request) {
	actor := s.requireAdmin(w, r)
	if actor == nil {
		return
	}
	u := s.userByPath(w, r)
	if u == nil {
		return
	}
	token, expires, err := s.store.CreatePasswordReset(r.Context(), u.ID, actor.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": token, "path": "/reset/" + token, "expiresAt": expires})
}

func (s *Server) allowPasswordReset(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Referrer-Policy", "no-referrer")
	if !s.resetLimit.Allow(s.auth.ClientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "Too many reset link attempts. Try again in a minute.")
		return false
	}
	return true
}

func (s *Server) checkPasswordReset(w http.ResponseWriter, r *http.Request) {
	if !s.allowPasswordReset(w, r) {
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	u, err := s.store.PasswordResetUser(r.Context(), req.Token)
	if errors.Is(err, store.ErrInvalidPasswordReset) {
		writeError(w, http.StatusNotFound, invalidPasswordReset)
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": true, "username": u.Username})
}

func (s *Server) redeemPasswordReset(w http.ResponseWriter, r *http.Request) {
	if !s.allowPasswordReset(w, r) {
		return
	}
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	// Check before hashing; redemption checks again under the write lock.
	if _, err := s.store.PasswordResetUser(r.Context(), req.Token); err != nil {
		if errors.Is(err, store.ErrInvalidPasswordReset) {
			writeError(w, http.StatusNotFound, invalidPasswordReset)
		} else {
			s.internalError(w, r, err)
		}
		return
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	userID, err := s.store.RedeemPasswordReset(r.Context(), req.Token, hash)
	if errors.Is(err, store.ErrInvalidPasswordReset) {
		writeError(w, http.StatusNotFound, invalidPasswordReset)
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.hub.EndSessions(userID, nil)
	w.WriteHeader(http.StatusNoContent)
}
