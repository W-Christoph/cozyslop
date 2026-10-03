package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cozycast/internal/config"
	"cozycast/internal/httpapi"
	"cozycast/internal/neko"
	"cozycast/internal/room"
	"cozycast/webui"
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

	defaults := room.Permissions{Remote: cfg.DefaultRemote, Upload: cfg.DefaultUpload}
	rooms := make(map[string]*room.Room, len(cfg.Rooms))
	for _, rc := range cfg.Rooms {
		nc, err := neko.NewClient(rc.NekoURL, cfg.NekoAPIToken)
		if err != nil {
			return err
		}
		rm := room.New(rc.Name, nc, defaults)
		rooms[rc.Name] = rm
		go rm.Start(ctx)
	}

	var web fs.FS = webui.FS()
	if cfg.WebDir != "" {
		web = os.DirFS(cfg.WebDir)
	}

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           httpapi.New(rooms, web).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.Listen, "rooms", len(rooms))
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
