package httpapi

import (
	"errors"
	"net/http"
	"regexp"
	"unicode/utf8"

	"cozycast/internal/auth"
	"cozycast/internal/store"
)

// Validation rules carried over from CozyCast so imported accounts stay valid.
var (
	usernameRe  = regexp.MustCompile(`^[a-zA-Z0-9](?:[-_.]?[a-zA-Z0-9])*$`)
	nicknameRe  = regexp.MustCompile(`^[\x{21}-\x{7E}\x{A1}-\x{AC}\x{AE}-\x{FF}](?: ?[\x{21}-\x{7E}\x{A1}-\x{AC}\x{AE}-\x{FF}])*$`)
	nameColorRe = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}){1,2}$`)
)

func validUsername(s string) bool {
	return len(s) >= 2 && len(s) <= 12 && usernameRe.MatchString(s)
}

func validPassword(s string) bool {
	n := utf8.RuneCountInString(s)
	return n >= 8 && n <= 100
}

func validNickname(s string) bool {
	n := utf8.RuneCountInString(s)
	return n >= 1 && n <= 12 && nicknameRe.MatchString(s)
}

// me is the account as the owner sees it.
type me struct {
	Username  string `json:"username"`
	Nickname  string `json:"nickname"`
	NameColor string `json:"nameColor"`
	AvatarURL string `json:"avatarUrl"`
	Admin     bool   `json:"admin"`
	Verified  bool   `json:"verified"`
}

func toMe(u *store.User) me {
	return me{
		Username:  u.Username,
		Nickname:  u.Nickname,
		NameColor: u.NameColor,
		AvatarURL: avatarURL(u),
		Admin:     u.Admin,
		Verified:  u.Verified,
	}
}

func avatarURL(u *store.User) string {
	if u == nil || u.Avatar == "" {
		return "/png/default_avatar.png"
	}
	return "/media/avatars/" + u.Avatar
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	set, err := s.store.Settings(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, set)
}

func (s *Server) getMe(w http.ResponseWriter, r *http.Request) {
	id, err := s.auth.Identify(w, r)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if id.User == nil {
		writeJSON(w, http.StatusOK, map[string]any{"user": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": toMe(id.User)})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if !s.loginLimit.Allow(s.auth.ClientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "Too many login attempts. Try again in a minute.")
		return
	}
	u, err := s.auth.Login(w, r, req.Username, req.Password)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		writeError(w, http.StatusUnauthorized, "Wrong username or password.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": toMe(u)})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if err := s.auth.Logout(w, r); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		InviteCode string `json:"inviteCode"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if !validUsername(req.Username) {
		writeError(w, http.StatusBadRequest,
			"Usernames are 2-12 letters or digits, optionally separated by single '-', '_' or '.'.")
		return
	}
	if !validPassword(req.Password) {
		writeError(w, http.StatusBadRequest, "Passwords are 8-100 characters.")
		return
	}

	set, err := s.store.Settings(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	// Invite handling (checking and redeeming req.InviteCode) is added with
	// the invites API; until then invite-only registration is closed.
	if set.Registration != "open" {
		writeError(w, http.StatusForbidden, "Registration requires an invite.")
		return
	}
	if !s.registerLimit.Allow(s.auth.ClientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "Too many new accounts from your address. Try again later.")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	u := &store.User{Username: req.Username, PasswordHash: hash, Nickname: req.Username}
	err = s.store.CreateUser(r.Context(), u)
	if errors.Is(err, store.ErrUsernameTaken) {
		writeError(w, http.StatusConflict, "That username is taken.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := s.auth.StartSession(w, r, u); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": toMe(u)})
}

func (s *Server) updateMe(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	var req struct {
		Nickname  string `json:"nickname"`
		NameColor string `json:"nameColor"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if !validNickname(req.Nickname) {
		writeError(w, http.StatusBadRequest,
			"Nicknames are 1-12 printable characters without leading, trailing or double spaces.")
		return
	}
	if !nameColorRe.MatchString(req.NameColor) {
		writeError(w, http.StatusBadRequest, "Name colour must be a hex colour like #f90 or #ff9900.")
		return
	}
	// A nickname may not impersonate another account's username.
	if other, err := s.store.UserByUsername(r.Context(), req.Nickname); err == nil && other.ID != u.ID {
		writeError(w, http.StatusConflict, "That nickname is another account's username.")
		return
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.internalError(w, r, err)
		return
	}

	if err := s.store.UpdateProfile(r.Context(), u.ID, req.Nickname, req.NameColor); err != nil {
		s.internalError(w, r, err)
		return
	}
	u.Nickname, u.NameColor = req.Nickname, req.NameColor
	writeJSON(w, http.StatusOK, map[string]any{"user": toMe(u)})
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	var req struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if !s.loginLimit.Allow(s.auth.ClientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "Too many attempts. Try again in a minute.")
		return
	}
	if !auth.CheckPassword(u, req.Current) {
		writeError(w, http.StatusForbidden, "Your current password is wrong.")
		return
	}
	if !validPassword(req.New) {
		writeError(w, http.StatusBadRequest, "Passwords are 8-100 characters.")
		return
	}
	hash, err := auth.HashPassword(req.New)
	if err == nil {
		err = s.store.UpdatePassword(r.Context(), u.ID, hash)
	}
	if err == nil {
		// Anyone who knew the old password is logged out.
		err = s.auth.LogoutOthers(r.Context(), r, u)
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// requireUser returns the logged-in user, or answers 401 and returns nil.
func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) *store.User {
	id, err := s.auth.Identify(w, r)
	if err != nil {
		s.internalError(w, r, err)
		return nil
	}
	if id.User == nil {
		writeError(w, http.StatusUnauthorized, "Please log in.")
		return nil
	}
	return id.User
}

// requireAdmin returns the logged-in admin, or answers 401/403 and returns nil.
func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) *store.User {
	u := s.requireUser(w, r)
	if u != nil && !u.Admin {
		writeError(w, http.StatusForbidden, "Admins only.")
		return nil
	}
	return u
}
