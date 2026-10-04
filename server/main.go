package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
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
	"cozycast/webui"

	"golang.org/x/crypto/acme/autocert"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
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
					"avatars", summary.Avatars, "skipped", summary.Skipped)
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

	rooms := make([]hub.RoomConfig, 0, len(cfg.Rooms))
	for _, rc := range cfg.Rooms {
		nc, err := neko.NewClient(rc.NekoURL, cfg.NekoToken(rc.Name))
		if err != nil {
			return err
		}
		room := hub.RoomConfig{Name: rc.Name, Neko: nc}
		if dc != nil {
			u, err := url.Parse(rc.NekoURL)
			if err != nil {
				return err
			}
			service := u.Hostname()
			room.Restart = func(ctx context.Context) error {
				id, err := dc.ServiceContainer(ctx, project, service)
				if err != nil {
					return err
				}
				return dc.Restart(ctx, id, 10*time.Second)
			}
		}
		rooms = append(rooms, room)
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
		Store:       db,
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
