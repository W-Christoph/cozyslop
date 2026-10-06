package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"cozycast/internal/auth"
	"cozycast/internal/config"
	"cozycast/internal/docker"
	"cozycast/internal/httpapi"
	"cozycast/internal/hub"
	"cozycast/internal/legacy"
	"cozycast/internal/neko"
	"cozycast/internal/store"
	"cozycast/internal/tunnel"
	"cozycast/webui"

	"golang.org/x/crypto/acme/autocert"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		if len(os.Args) > 1 {
			fmt.Fprintln(os.Stderr, err)
		} else {
			slog.Error("fatal", "err", err)
		}
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) > 0 && args[0] != "reset-admin" {
		return fmt.Errorf("unknown subcommand %q\nUsage: cozycast [reset-admin]", args[0])
	}
	if len(args) > 1 {
		return errors.New("Usage: cozycast [reset-admin]")
	}
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	var adminHash string
	if len(args) > 0 {
		if err := auth.ValidatePassword(cfg.InitAdminPass); err != nil {
			return fmt.Errorf("COZYCAST_INIT_ADMIN_PASSWORD: %w", err)
		}
		adminHash, err = auth.HashPassword(cfg.InitAdminPass)
		if err != nil {
			return err
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return err
	}
	db, err := store.Open(ctx, filepath.Join(cfg.DataDir, "cozycast.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	if len(args) > 0 {
		created, err := db.ResetAdmin(ctx, adminHash)
		if err != nil {
			return err
		}
		action := "Reset admin password"
		if created {
			action = "Created admin account"
		}
		_, err = fmt.Fprintln(out, action+"; enabled admin rights and cleared sessions.")
		return err
	}
	mediaDir := filepath.Join(cfg.DataDir, "media")
	for _, dir := range []string{"chat", "avatars"} {
		if err := os.MkdirAll(filepath.Join(mediaDir, dir), 0o750); err != nil {
			return err
		}
	}
	if cfg.ImportPath != "" {
		if _, err := os.Stat(cfg.ImportPath); err == nil {
			summary, err := legacy.Import(ctx, db, cfg.ImportPath, filepath.Join(mediaDir, "avatars"))
			switch {
			case errors.Is(err, legacy.ErrNotEmpty):
				slog.Info("legacy import skipped because the database is not empty")
			case err != nil:
				return err
			default:
				slog.Info("legacy import complete", "users", summary.Users, "rooms", summary.Rooms,
					"permissions", summary.Permissions, "invites", summary.Invites,
					"logins", summary.Logins, "avatars", summary.Avatars, "skipped", summary.Skipped)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := ensureAdmin(ctx, db, cfg.InitAdminPass); err != nil {
		return err
	}
	go sweepExpired(ctx, db)

	var dc *docker.Client
	var project string
	if cfg.Docker {
		dc = docker.New(cfg.DockerSocket)
		project, err = dc.OwnProject(ctx)
		if err != nil {
			project = cfg.DockerProject
			if project == "" {
				slog.Warn("Docker compose project unavailable; matching containers by service name only", "err", err)
			}
		}
		slog.Info("room container control enabled", "socket", cfg.DockerSocket, "project", project)
	} else {
		slog.Info("room container control disabled")
	}

	tun, err := openTunnel(ctx, db, cfg)
	if err != nil {
		return err
	}
	if tun != nil {
		defer tun.Close()
		go rememberEndpoints(ctx, db, tun)
	}
	buildRoom := roomBuilder(cfg, tun)
	rooms, err := loadRooms(ctx, db, cfg, buildRoom)
	if err != nil {
		return err
	}
	for i := range rooms {
		room := &rooms[i]
		if dc != nil && room.Source == "configured" {
			service := room.Neko.BaseURL().Hostname()
			room.Restart = func(ctx context.Context) error {
				id, err := dc.ServiceContainer(ctx, project, service)
				if err != nil {
					return err
				}
				return dc.Restart(ctx, id, 10*time.Second)
			}
		}
	}
	h := hub.New(db, filepath.Join(mediaDir, "chat"), rooms)
	if err := h.Start(ctx); err != nil {
		return err
	}

	var web fs.FS = webui.FS()
	if cfg.WebDir != "" {
		web = os.DirFS(cfg.WebDir)
	}

	api := httpapi.New(httpapi.Deps{
		BuildRoom:   buildRoom,
		Store:       db,
		Tunnel:      tun,
		Auth:        auth.New(db, cfg.TrustProxy),
		Hub:         h,
		Web:         web,
		MediaDir:    mediaDir,
		MaxUploadMB: cfg.MaxUploadMB,
		SourceURL:   cfg.SourceURL,
	})
	handler := api.Handler()

	var servers []*http.Server
	errc := make(chan error, 2)
	serve := func(srv *http.Server, tls bool) {
		servers = append(servers, srv)
		go func() {
			slog.Info("listening", "addr", srv.Addr, "tls", tls, "rooms", len(rooms))
			if tls {
				errc <- srv.ListenAndServeTLS("", "")
			} else {
				errc <- srv.ListenAndServe()
			}
		}()
	}

	if len(cfg.Domains) == 0 {
		serve(newServer(cfg.Listen, handler), false)
	} else {
		// Automatic HTTPS: certificates from Let's Encrypt, cached in the
		// data dir. Plain HTTP only answers ACME challenges and redirects.
		m := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(cfg.Domains...),
			Cache:      autocert.DirCache(filepath.Join(cfg.DataDir, "certs")),
			Email:      cfg.ACMEEmail,
		}
		tlsSrv := newServer(cfg.TLSListen, hsts(handler))
		tlsSrv.TLSConfig = m.TLSConfig()
		serve(tlsSrv, true)
		serve(newServer(cfg.Listen, m.HTTPHandler(nil)), false)
	}

	var serveErr error
	select {
	case serveErr = <-errc:
		stop()
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return errors.Join(serveErr, shutdown(shutdownCtx, servers, api))
}

func shutdown(ctx context.Context, servers []*http.Server, api *httpapi.Server) error {
	var err error
	for _, srv := range servers {
		if e := srv.Shutdown(ctx); e != nil && !errors.Is(e, http.ErrServerClosed) {
			err = errors.Join(err, e, srv.Close())
		}
	}
	return errors.Join(err, api.Shutdown(ctx))
}

func newServer(addr string, h http.Handler) *http.Server {
	// Request bodies get their deadline per route (httpapi); WebSockets and
	// uploads must not be cut off by a server-wide one.
	return &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute}
}

// hsts tells browsers to use HTTPS for this site from now on.
func hsts(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		next.ServeHTTP(w, r)
	})
}

// ensureAdmin creates the "admin" account on first start (CozyCast did the
// same). An existing account is left alone, including its password.
func ensureAdmin(ctx context.Context, db *store.Store, password string) error {
	if password == "" {
		return nil
	}
	_, err := db.UserByUsername(ctx, "admin")
	if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if err := auth.ValidatePassword(password); err != nil {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	slog.Info("creating admin account")
	return db.CreateUser(ctx, &store.User{Username: "admin", PasswordHash: hash, Nickname: "admin", Admin: true, Verified: true})
}

func sweepExpired(ctx context.Context, db *store.Store) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		deleteExpired(ctx, db)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func deleteExpired(ctx context.Context, db *store.Store) {
	if err := db.DeleteExpiredSessions(ctx); err != nil && ctx.Err() == nil {
		slog.Warn("delete expired sessions", "err", err)
	}
	if err := db.DeleteExpiredAnonBans(ctx); err != nil && ctx.Err() == nil {
		slog.Warn("delete expired anonymous bans", "err", err)
	}
}

// loadRooms gives configured rooms precedence without deleting registrations.
func loadRooms(ctx context.Context, db *store.Store, cfg config.Config, build httpapi.RoomBuilder) ([]hub.RoomConfig, error) {
	rooms := []hub.RoomConfig{}
	configured := make(map[string]bool)
	for _, rc := range cfg.Rooms {
		room, err := build(rc.Name, rc.NekoURL, cfg.NekoToken(rc.Name))
		if err != nil {
			return nil, err
		}
		room.Source = "configured"
		rooms = append(rooms, room)
		configured[rc.Name] = true
	}
	registered, err := db.RegisteredRooms(ctx)
	if err != nil {
		return nil, err
	}
	for _, rc := range registered {
		if configured[rc.Name] {
			slog.Warn("configured room overrides registered room", "room", rc.Name)
			continue
		}
		room, err := build(rc.Name, rc.NekoURL, rc.NekoToken)
		if err != nil {
			return nil, err
		}
		room.Source = "registered"
		if rc.Paired() {
			room.Source = "paired"
		}
		rooms = append(rooms, room)
	}
	return rooms, nil
}

// roomBuilder builds rooms whose neko is reached through the tunnel when its
// address is inside it, and directly otherwise.
func roomBuilder(cfg config.Config, tun *tunnel.Tunnel) httpapi.RoomBuilder {
	return func(name, nekoURL, token string) (hub.RoomConfig, error) {
		var dial neko.DialFunc
		if tun.Owns(nekoURL) {
			dial = tun.DialContext
		}
		return hub.BuildRoomConfig(name, nekoURL, token, cfg.DefaultScreen, dial)
	}
}

// openTunnel starts the server's end of the tunnels to paired rooms, with
// every paired room as a peer. nil when COZYCAST_TUNNEL_PORT is not set.
func openTunnel(ctx context.Context, db *store.Store, cfg config.Config) (*tunnel.Tunnel, error) {
	registered, err := db.RegisteredRooms(ctx)
	if err != nil {
		return nil, err
	}
	if cfg.TunnelPort == 0 {
		for _, rc := range registered {
			if rc.Paired() {
				slog.Warn("paired room unreachable: set COZYCAST_TUNNEL_PORT", "room", rc.Name)
			}
		}
		return nil, nil
	}
	key, err := tunnel.LoadKey(filepath.Join(cfg.DataDir, "wireguard.key"))
	if err != nil {
		return nil, fmt.Errorf("tunnel key: %w", err)
	}
	tun, err := tunnel.Open(tunnel.Config{PrivateKey: key, Port: cfg.TunnelPort,
		Address: cfg.TunnelNet.Addr().Next(), Network: cfg.TunnelNet})
	if err != nil {
		return nil, err
	}
	for _, rc := range registered {
		if !rc.Paired() {
			continue
		}
		if err := tun.SetPeer(nodePeer(rc)); err != nil {
			// Its room stays offline; the others still work.
			slog.Error("paired room unusable", "room", rc.Name, "err", err)
		}
	}
	slog.Info("tunnel listening", "port", tun.Port(), "address", tun.Address(), "network", tun.Network())
	return tun, nil
}

// nodePeer is a paired room's node as a tunnel peer. A node that was seen
// before is reachable at once, without waiting for it to reconnect.
func nodePeer(rc store.RegisteredRoom) tunnel.Peer {
	key, _ := tunnel.ParseKey(rc.NodeKey)
	addr, _ := netip.ParseAddr(rc.TunnelAddress)
	endpoint, _ := netip.ParseAddrPort(rc.NodeEndpoint)
	return tunnel.Peer{Key: key, Address: addr, Endpoint: endpoint}
}

// rememberEndpoints stores where each node was last seen, for nodePeer after
// a restart.
func rememberEndpoints(ctx context.Context, db *store.Store, tun *tunnel.Tunnel) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		saveEndpoints(ctx, db, tun)
	}
}

func saveEndpoints(ctx context.Context, db *store.Store, tun *tunnel.Tunnel) {
	registered, err := db.RegisteredRooms(ctx)
	if err != nil {
		return
	}
	for _, rc := range registered {
		key, err := tunnel.ParseKey(rc.NodeKey)
		if !rc.Paired() || err != nil {
			continue
		}
		if ep := tun.Endpoint(key); ep.IsValid() && ep.String() != rc.NodeEndpoint {
			if err := db.SetNodeEndpoint(ctx, rc.Name, ep.String()); err != nil && ctx.Err() == nil {
				slog.Warn("remember node endpoint", "room", rc.Name, "err", err)
			}
		}
	}
}
