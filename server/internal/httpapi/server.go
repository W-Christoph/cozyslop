// Package httpapi wires the HTTP surface: the JSON API, the room WebSocket,
// the neko reverse proxy and the embedded web UI.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	"cozycast/internal/auth"
	"cozycast/internal/fwd"
	"cozycast/internal/hub"
	"cozycast/internal/pairing"
	"cozycast/internal/ratelimit"
	"cozycast/internal/relay"
	"cozycast/internal/rights"
	"cozycast/internal/store"
	"cozycast/internal/tunnel"
)

type RoomBuilder func(name, nekoURL, token string) (hub.RoomConfig, error)

type Deps struct {
	BuildRoom   RoomBuilder
	Store       *store.Store
	Auth        *auth.Service
	Hub         *hub.Hub
	Web         fs.FS
	MediaDir    string // avatars and chat subdirectories
	MaxUploadMB int64  // chat media; defaults to 10 MiB
	// SourceURL is where users get this server's source code; the AGPL
	// requires offering it to everyone using the server over the network.
	SourceURL string
	// Tunnel to rooms on other machines; nil when off.
	Tunnel *tunnel.Tunnel
	// Paired rooms' media: forwarded ports, the address viewers reach
	// them on (invalid if unknown) and the ports to give out.
	Media      *fwd.Ports
	PublicIP   netip.Addr
	MediaPorts [2]int
	// Relay sends paired rooms' media to their viewers; nil when off, and
	// viewers then connect to the forwarded ports themselves.
	Relay *relay.Relay
}

type Server struct {
	buildRoom      RoomBuilder
	tunnel         *tunnel.Tunnel
	media          *fwd.Ports
	relay          *relay.Relay
	publicIP       netip.Addr
	mediaPorts     [2]int
	pairing        *pairing.Manager // nil when the tunnel is off
	nodeMu         sync.Mutex
	nodeBoots      map[string]string // room: the boot ID its computer last checked in with
	roomMu         sync.Mutex        // registration mutations
	store          *store.Store
	auth           *auth.Service
	hub            *hub.Hub
	web            fs.FS
	sourceURL      string
	log            *slog.Logger
	mediaDir       string
	maxUploadBytes int64
	avatarMu       sync.Mutex // avatar replacement, removal and account deletion

	adminMu sync.Mutex // enabled-admin count checks and updates

	socketMu    sync.Mutex // serializes handler registration with shutdown
	socketWG    sync.WaitGroup
	socketCtx   context.Context
	stopSockets context.CancelFunc

	loginLimit    *ratelimit.Limiter // per IP: login attempts
	resetLimit    *ratelimit.Limiter // per IP: reset link checks and redemptions
	registerLimit *ratelimit.Limiter // per IP: account creations
	pairLimit     *ratelimit.Limiter // per IP: pairing requests
}

func New(d Deps) *Server {
	if d.MaxUploadMB <= 0 {
		d.MaxUploadMB = 10
	}
	if d.BuildRoom == nil {
		d.BuildRoom = func(name, nekoURL, token string) (hub.RoomConfig, error) {
			return hub.BuildRoomConfig(name, nekoURL, token, "", nil)
		}
	}
	socketCtx, stopSockets := context.WithCancel(context.Background())
	var pm *pairing.Manager
	if d.Tunnel != nil {
		pm = pairing.NewManager(d.Tunnel.PublicKey())
	}
	return &Server{
		pairing:        pm,
		nodeBoots:      make(map[string]string),
		pairLimit:      ratelimit.New(10, time.Minute),
		buildRoom:      d.BuildRoom,
		tunnel:         d.Tunnel,
		media:          d.Media,
		relay:          d.Relay,
		publicIP:       d.PublicIP,
		mediaPorts:     d.MediaPorts,
		store:          d.Store,
		auth:           d.Auth,
		hub:            d.Hub,
		web:            d.Web,
		sourceURL:      d.SourceURL,
		log:            slog.Default(),
		mediaDir:       d.MediaDir,
		maxUploadBytes: d.MaxUploadMB << 20,
		loginLimit:     ratelimit.New(10, 30*time.Second),
		resetLimit:     ratelimit.New(10, 30*time.Second),
		registerLimit:  ratelimit.New(3, 10*time.Minute),
		socketCtx:      socketCtx,
		stopSockets:    stopSockets,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/legacy", s.legacyLogin)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("POST /api/auth/register", s.register)
	mux.HandleFunc("POST /api/auth/password-reset/check", s.checkPasswordReset)
	mux.HandleFunc("POST /api/auth/password-reset/redeem", s.redeemPasswordReset)
	mux.HandleFunc("GET /api/me", s.getMe)
	mux.HandleFunc("PATCH /api/me", s.updateMe)
	mux.HandleFunc("POST /api/me/password", s.changePassword)
	mux.HandleFunc("POST /api/me/avatar", s.uploadAvatar)
	mux.HandleFunc("DELETE /api/me/avatar", s.deleteAvatar)

	mux.HandleFunc("GET /api/admin/users", s.adminListUsers)
	mux.HandleFunc("PATCH /api/admin/users/{username}", s.adminUpdateUser)
	mux.HandleFunc("DELETE /api/admin/users/{username}", s.adminDeleteUser)
	mux.HandleFunc("POST /api/admin/users/{username}/password", s.adminSetPassword)
	mux.HandleFunc("POST /api/admin/users/{username}/password-reset", s.adminCreatePasswordReset)

	mux.HandleFunc("GET /api/admin/permissions", s.adminListPermissions)
	mux.HandleFunc("PUT /api/admin/permissions/{room}/{username}", s.adminSavePermission)
	mux.HandleFunc("DELETE /api/admin/permissions/{room}/{username}", s.adminDeletePermission)

	mux.HandleFunc("GET /api/admin/rooms", s.adminListRooms)
	mux.HandleFunc("POST /api/admin/rooms", s.adminCreateRoom)
	mux.HandleFunc("PATCH /api/admin/rooms/{room}", s.adminChangeRoom)
	mux.HandleFunc("POST /api/admin/rooms/{room}/token", s.adminChangeRoom)
	mux.HandleFunc("POST /api/admin/rooms/{room}/start", s.adminStartRoom)
	mux.HandleFunc("POST /api/admin/rooms/{room}/stop", s.adminStopRoom)
	mux.HandleFunc("DELETE /api/admin/rooms/{room}", s.adminDeleteRoom)

	mux.HandleFunc("POST /api/nodes/pair", s.createPairing)
	mux.HandleFunc("GET /api/nodes/pair/{id}", s.pairingStatus)
	mux.HandleFunc("GET /api/admin/pairing", s.adminListPairing)
	mux.HandleFunc("POST /api/admin/pairing/{id}/accept", s.adminAcceptPairing)
	mux.HandleFunc("DELETE /api/admin/pairing/{id}", s.adminRejectPairing)

	mux.HandleFunc("GET /api/admin/rooms/{room}/settings", s.adminGetRoomSettings)
	mux.HandleFunc("PUT /api/admin/rooms/{room}/settings", s.adminSaveRoomSettings)
	mux.HandleFunc("GET /api/admin/rooms/{room}/stream-options", s.adminStreamOptions)

	mux.HandleFunc("POST /api/admin/rooms/{room}/bans", s.adminBan)
	mux.HandleFunc("GET /api/admin/rooms/{room}/grants", s.adminListGrants)
	mux.HandleFunc("PUT /api/admin/rooms/{room}/grants", s.adminGrant)
	mux.HandleFunc("GET /api/admin/bans", s.adminListBans)
	mux.HandleFunc("DELETE /api/admin/bans/{id}", s.adminDeleteBan)

	mux.HandleFunc("PUT /api/admin/settings", s.adminSaveSettings)

	mux.HandleFunc("POST /api/admin/invites", s.adminCreateInvite)
	mux.HandleFunc("GET /api/admin/invites", s.adminListInvites)
	mux.HandleFunc("DELETE /api/admin/invites/{code}", s.adminDeleteInvite)
	mux.HandleFunc("GET /api/invites/{code}", s.checkInvite)
	mux.HandleFunc("POST /api/invites/{code}/redeem", s.redeemInvite)

	mux.HandleFunc("GET /api/rooms", s.listRooms)
	mux.HandleFunc("GET /api/rooms/{room}/ws", s.roomSocket)
	mux.HandleFunc("POST /api/rooms/{room}/media", s.uploadChatMedia)
	mux.HandleFunc("GET /media/avatars/{file}", s.serveAvatar)
	mux.HandleFunc("GET /media/chat/{file}", s.serveChatMedia)
	mux.HandleFunc("/media/{path...}", http.NotFound)
	mux.HandleFunc("/media", http.NotFound)
	mux.HandleFunc("/neko/{room}/{path...}", s.nekoProxy)
	mux.Handle("/", spaHandler(s.web))
	return bodyDeadline(sameOrigin(validateMediaPath(mux)))
}

// How long a request body may take to arrive. Uploads to the room desktop
// (through the neko proxy) can be large.
var (
	bodyTimeout       = 30 * time.Second
	uploadBodyTimeout = 10 * time.Minute
	nekoBodyTimeout   = 2 * time.Hour
)

// bodyDeadline stops a client from holding a connection open by never
// finishing its request body. WebSocket upgrades have no body and lose the
// deadline when the connection is taken over.
func bodyDeadline(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength != 0 {
			d := bodyTimeout
			switch p := r.URL.Path; {
			case strings.HasPrefix(p, "/neko/"):
				d = nekoBodyTimeout
			case p == "/api/me/avatar", strings.HasPrefix(p, "/api/rooms/") && strings.HasSuffix(p, "/media"):
				d = uploadBodyTimeout
			}
			_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(d))
		}
		next.ServeHTTP(w, r)
	})
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
		Open      bool   `json:"open"`   // the requester may join
		Online    bool   `json:"online"` // the room's desktop is connected
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
		list = append(list, roomInfo{Name: rm.Name, Access: in.Room.Access, UserCount: rm.UserCount(), Open: open, Online: rm.Neko().Connected()})
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
	if err := dec.Decode(new(any)); err != io.EOF {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return false
	}
	return true
}

// spaHandler serves static files and falls back to index.html so client-side
// routes like /room/default work on reload.
func spaHandler(web fs.FS) http.Handler {
	files := http.FileServerFS(web)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/reset/") {
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; form-action 'self'; base-uri 'self'")
		}
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
