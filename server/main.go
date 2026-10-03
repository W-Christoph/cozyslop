package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"cozycast/internal/auth"
	"cozycast/internal/config"
	"cozycast/internal/httpapi"
	"cozycast/internal/hub"
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
	if err := ensureAdmin(ctx, db, cfg.InitAdminPass); err != nil {
		return err
	}
	go sweepSessions(ctx, db)

	mediaDir := filepath.Join(cfg.DataDir, "media")
	for _, dir := range []string{"chat", "avatars"} {
		if err := os.MkdirAll(filepath.Join(mediaDir, dir), 0o750); err != nil {
			return err
		}
	}

	rooms := make([]hub.RoomConfig, 0, len(cfg.Rooms))
	for _, rc := range cfg.Rooms {
		nc, err := neko.NewClient(rc.NekoURL, cfg.NekoAPIToken)
		if err != nil {
			return err
		}
		rooms = append(rooms, hub.RoomConfig{Name: rc.Name, Neko: nc})
	}
	h := hub.New(db, filepath.Join(mediaDir, "chat"), rooms)
	h.Start(ctx)

	var web fs.FS = webui.FS()
	if cfg.WebDir != "" {
		web = os.DirFS(cfg.WebDir)
	}

	handler := httpapi.New(httpapi.Deps{
		Store: db,
		Auth:  auth.New(db, cfg.TrustProxy),
		Hub:   h,
		Web:   web,
	}).Handler()

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

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, srv := range servers {
		if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	return nil
}

func newServer(addr string, h http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 10 * time.Second}
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
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	slog.Info("creating admin account")
	return db.CreateUser(ctx, &store.User{Username: "admin", PasswordHash: hash, Nickname: "admin", Admin: true, Verified: true})
}

func sweepSessions(ctx context.Context, db *store.Store) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if err := db.DeleteExpiredSessions(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("delete expired sessions", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
