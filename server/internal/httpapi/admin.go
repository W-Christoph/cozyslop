package httpapi

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"

	"cozycast/internal/auth"
	"cozycast/internal/hub"
	"cozycast/internal/neko"
	"cozycast/internal/store"
)

type adminUser struct {
	me
	Disabled  bool  `json:"disabled"`
	CreatedAt int64 `json:"createdAt"`
}

func toAdminUser(u *store.User) adminUser {
	return adminUser{me: toMe(u), Disabled: u.Disabled, CreatedAt: u.CreatedAt}
}

func (s *Server) adminListUsers(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	list := make([]adminUser, 0, len(users))
	for _, u := range users {
		list = append(list, toAdminUser(u))
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) adminUpdateUser(w http.ResponseWriter, r *http.Request) {
	actor := s.requireAdmin(w, r)
	if actor == nil {
		return
	}
	var req struct {
		Admin    *bool `json:"admin"`
		Verified *bool `json:"verified"`
		Disabled *bool `json:"disabled"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	// Keep the enabled-admin count check and flag update together, including
	// when two admins demote or disable one another at the same time.
	s.adminMu.Lock()
	defer s.adminMu.Unlock()
	u := s.userByPath(w, r)
	if u == nil {
		return
	}
	admin, verified, disabled := u.Admin, u.Verified, u.Disabled
	if req.Admin != nil {
		admin = *req.Admin
	}
	if req.Verified != nil {
		verified = *req.Verified
	}
	if req.Disabled != nil {
		disabled = *req.Disabled
	}
	if actor.ID == u.ID && (admin != u.Admin || disabled != u.Disabled) {
		writeError(w, http.StatusForbidden, "You can't remove your own admin rights or disable yourself.")
		return
	}
	n, err := s.store.CountAdmins(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if u.Admin && !u.Disabled {
		n--
	}
	if admin && !disabled {
		n++
	}
	if n == 0 {
		writeError(w, http.StatusConflict, "There must be at least one admin.")
		return
	}
	if err := s.store.UpdateFlags(r.Context(), u.ID, admin, verified, disabled); err != nil {
		s.internalError(w, r, err)
		return
	}
	if disabled && !u.Disabled {
		if err := s.store.DeleteUserSessions(r.Context(), u.ID, nil); err != nil {
			s.hub.UserChanged(r.Context(), u.ID)
			s.internalError(w, r, err)
			return
		}
	}
	u.Admin, u.Verified, u.Disabled = admin, verified, disabled
	s.hub.UserChanged(r.Context(), u.ID)
	writeJSON(w, http.StatusOK, toAdminUser(u))
}

func (s *Server) adminDeleteUser(w http.ResponseWriter, r *http.Request) {
	actor := s.requireAdmin(w, r)
	if actor == nil {
		return
	}
	s.adminMu.Lock()
	defer s.adminMu.Unlock()
	s.avatarMu.Lock()
	defer s.avatarMu.Unlock()
	u := s.userByPath(w, r)
	if u == nil {
		return
	}
	if actor.ID == u.ID {
		writeError(w, http.StatusForbidden, "You can't delete yourself.")
		return
	}
	if u.Admin {
		writeError(w, http.StatusConflict, "Remove admin rights first.")
		return
	}
	if err := s.store.DeleteUser(r.Context(), u.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	s.removeMediaFile("avatars", u.Avatar)
	s.hub.UserChanged(r.Context(), u.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminSetPassword(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	u := s.userByPath(w, r)
	if u == nil {
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if !validPassword(req.Password) {
		writeError(w, http.StatusBadRequest, "Passwords are 8-100 characters.")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err == nil {
		err = s.store.UpdatePassword(r.Context(), u.ID, hash)
	}
	if err == nil {
		err = s.store.DeleteUserSessions(r.Context(), u.ID, nil)
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminListPermissions(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	list, err := s.store.ListPermissions(r.Context(), r.URL.Query().Get("room"))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if list == nil {
		list = []store.Permission{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) adminSavePermission(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	room := r.PathValue("room")
	if s.hub.Room(room) == nil {
		writeError(w, http.StatusNotFound, "Unknown room.")
		return
	}
	u := s.userByPath(w, r)
	if u == nil {
		return
	}
	var req struct {
		Remote      bool   `json:"remote"`
		Image       bool   `json:"image"`
		Upload      bool   `json:"upload"`
		Trusted     bool   `json:"trusted"`
		Invited     bool   `json:"invited"`
		Banned      bool   `json:"banned"`
		InviteName  string `json:"inviteName"`
		BannedUntil *int64 `json:"bannedUntil"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if utf8.RuneCountInString(req.InviteName) > 64 {
		writeError(w, http.StatusBadRequest, "Invite names are at most 64 characters.")
		return
	}
	p := store.Permission{Room: room, UserID: u.ID, Remote: req.Remote, Image: req.Image,
		Upload: req.Upload, Trusted: req.Trusted, Invited: req.Invited, Banned: req.Banned,
		InviteName: req.InviteName, BannedUntil: req.BannedUntil}
	if err := s.store.SavePermission(r.Context(), p); err != nil {
		s.internalError(w, r, err)
		return
	}
	s.hub.PermissionsChanged(r.Context(), room, u.ID)
	list, err := s.store.ListPermissions(r.Context(), room)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	for _, saved := range list {
		if saved.UserID == u.ID {
			writeJSON(w, http.StatusOK, saved)
			return
		}
	}
	s.internalError(w, r, store.ErrNotFound)
}

func (s *Server) adminDeletePermission(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	u := s.userByPath(w, r)
	if u == nil {
		return
	}
	room := r.PathValue("room")
	err := s.store.DeletePermission(r.Context(), room, u.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "Unknown permission.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.hub.PermissionsChanged(r.Context(), room, u.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminGetRoomSettings(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	room := r.PathValue("room")
	if s.hub.Room(room) == nil {
		writeError(w, http.StatusNotFound, "Unknown room.")
		return
	}
	set, err := s.store.RoomSettings(r.Context(), room)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, set)
}

func (s *Server) adminSaveRoomSettings(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	room := r.PathValue("room")
	if s.hub.Room(room) == nil {
		writeError(w, http.StatusNotFound, "Unknown room.")
		return
	}
	var req struct {
		Name            string  `json:"name"` // ignored; the path decides
		Access          string  `json:"access"`
		Hidden          bool    `json:"hidden"`
		RemoteOwnership bool    `json:"remoteOwnership"`
		CenterRemote    bool    `json:"centerRemote"`
		DefaultRemote   bool    `json:"defaultRemote"`
		DefaultImage    bool    `json:"defaultImage"`
		DefaultUpload   bool    `json:"defaultUpload"`
		Screen          *string `json:"screen"` // omitted = unchanged
		Stream          *string `json:"stream"` // omitted = unchanged
	}
	if !readJSON(w, r, &req) {
		return
	}
	if !store.ValidAccess(req.Access) {
		writeError(w, http.StatusBadRequest, "Access must be public, account, verified or invite.")
		return
	}
	current, err := s.store.RoomSettings(r.Context(), room)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	set := store.RoomSettings{Name: room, Access: req.Access, Hidden: req.Hidden,
		RemoteOwnership: req.RemoteOwnership, CenterRemote: req.CenterRemote,
		DefaultRemote: req.DefaultRemote, DefaultImage: req.DefaultImage, DefaultUpload: req.DefaultUpload,
		Screen: current.Screen, Stream: current.Stream}
	if req.Stream != nil && *req.Stream != current.Stream {
		if *req.Stream != "" {
			streams := s.hub.Room(room).Streams()
			if streams == nil {
				writeError(w, http.StatusServiceUnavailable, "The room's desktop is not reachable right now.")
				return
			}
			if !slices.Contains(streams, *req.Stream) {
				writeError(w, http.StatusBadRequest, "The room does not offer that stream.")
				return
			}
		}
		set.Stream = *req.Stream
	}
	if req.Screen != nil && *req.Screen != current.Screen {
		if *req.Screen != "" && !s.screenSupported(w, r, room, *req.Screen) {
			return
		}
		set.Screen = *req.Screen
	}
	if err := s.store.SaveRoomSettings(r.Context(), set); err != nil {
		s.internalError(w, r, err)
		return
	}
	s.hub.RoomSettingsChanged(r.Context(), room)
	writeJSON(w, http.StatusOK, set)
}

// adminStreamOptions lists what the room can be set to: 16:9 screen sizes
// ("1280x720@30") and capture pipelines ("b2500-s100-veryfast", the default first).
func (s *Server) adminStreamOptions(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	rm := s.hub.Room(r.PathValue("room"))
	if rm == nil {
		writeError(w, http.StatusNotFound, "Unknown room.")
		return
	}
	list, err := offeredScreens(r, rm)
	streams := rm.Streams()
	if err != nil || streams == nil {
		if err != nil {
			s.log.Warn("list neko screens", "room", rm.Name, "err", err)
		}
		writeError(w, http.StatusServiceUnavailable, "The room's desktop is not reachable right now.")
		return
	}
	screens := make([]string, 0, len(list))
	for _, size := range list {
		screens = append(screens, size.String())
	}
	writeJSON(w, http.StatusOK, map[string][]string{"screens": screens, "streams": streams})
}

// offeredScreens lists the resolutions a room can be set to: the 16:9 ones
// among what its desktop supports.
func offeredScreens(r *http.Request, rm *hub.Room) ([]neko.ScreenSize, error) {
	list, err := rm.Neko().ScreenConfigurations(r.Context())
	return slices.DeleteFunc(list, func(s neko.ScreenSize) bool { return !s.Widescreen() }), err
}

// screenSupported checks a requested resolution against the room's desktop,
// answering the request itself if it is not supported.
func (s *Server) screenSupported(w http.ResponseWriter, r *http.Request, room, screen string) bool {
	if _, err := neko.ParseScreen(screen); err != nil {
		writeError(w, http.StatusBadRequest, "Screen must look like 1280x720@30.")
		return false
	}
	list, err := offeredScreens(r, s.hub.Room(room))
	if err != nil {
		s.log.Warn("list neko screens", "room", room, "err", err)
		writeError(w, http.StatusServiceUnavailable, "The room's desktop is not reachable right now.")
		return false
	}
	for _, size := range list {
		if size.String() == screen {
			return true
		}
	}
	writeError(w, http.StatusBadRequest, "The room's desktop does not support that resolution.")
	return false
}

func (s *Server) adminBan(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	rm := s.hub.Room(r.PathValue("room"))
	if rm == nil {
		writeError(w, http.StatusNotFound, "Unknown room.")
		return
	}
	var req struct {
		Key     string `json:"key"`
		Minutes *int64 `json:"minutes"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Minutes != nil && (*req.Minutes < 0 || *req.Minutes > (1<<63-1-time.Now().Unix())/60) {
		writeError(w, http.StatusBadRequest, "Minutes must be zero or a positive number, or null for forever.")
		return
	}
	var err error
	if req.Minutes != nil && *req.Minutes == 0 {
		err = rm.Kick(req.Key)
	} else {
		var until *int64
		if req.Minutes != nil {
			n := time.Now().Unix() + *req.Minutes*60
			until = &n
		}
		err = rm.Ban(r.Context(), req.Key, until)
	}
	if errors.Is(err, hub.ErrNotPresent) {
		writeError(w, http.StatusNotFound, "That user is not in the room.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminListBans(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	list, err := s.store.ListAnonBans(r.Context(), r.URL.Query().Get("room"))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if list == nil {
		list = []store.AnonBan{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) adminDeleteBan(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid ban ID.")
		return
	}
	err = s.store.DeleteAnonBan(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "Unknown ban.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminSaveSettings(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	var set store.Settings
	if !readJSON(w, r, &set) {
		return
	}
	if utf8.RuneCountInString(set.Message) > 4096 {
		writeError(w, http.StatusBadRequest, "The message is at most 4096 characters.")
		return
	}
	if set.Registration != "open" && set.Registration != "invite" {
		writeError(w, http.StatusBadRequest, "Registration must be open or invite.")
		return
	}
	if err := s.store.SaveSettings(r.Context(), set); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, set)
}

// userByPath answers 404 if the username path parameter is unknown.
func (s *Server) userByPath(w http.ResponseWriter, r *http.Request) *store.User {
	u, err := s.store.UserByUsername(r.Context(), r.PathValue("username"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "Unknown user.")
		return nil
	}
	if err != nil {
		s.internalError(w, r, err)
		return nil
	}
	return u
}
