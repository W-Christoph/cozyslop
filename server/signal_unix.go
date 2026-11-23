//go:build unix

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"cozycast/internal/diag"
)

// dumpOnSignal keeps a goroutine dump on SIGUSR1, for when the server does
// not answer: docker compose kill -s USR1 server.
func dumpOnSignal(ctx context.Context, w *diag.Watchdog) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGUSR1)
	go func() {
		defer signal.Stop(c)
		for {
			select {
			case <-ctx.Done():
				return
			case <-c:
				w.Dump("SIGUSR1")
			}
		}
	}()
}
