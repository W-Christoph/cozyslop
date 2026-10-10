// Command node is the agent on a computer that runs a room for a CozyCast
// server elsewhere: see docs/home-hosting.md and compose.node.yaml.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"cozycast/internal/node"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	forward, err := node.PortsFromEnv(env("COZYCAST_FORWARD", "8080=room:8080,8081=room:8081,8082=room:8082,8083=room:8083"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err = node.Run(ctx, node.Config{
		Hub:      os.Getenv("COZYCAST_HUB"),
		Name:     env("COZYCAST_ROOM_NAME", "home"),
		StateDir: env("COZYCAST_STATE_DIR", "/state"),
		EnvFile:  env("COZYCAST_ENV_FILE", "/run/cozycast/room.env"),
		Forward:  forward,
		// The room's only way out (compose.node.yaml).
		ProxyListen: env("COZYCAST_PROXY_LISTEN", ":3128"),
		RoomHost:    env("COZYCAST_ROOM_HOST", "room"),
		Out:         os.Stdout,
	})
	if errors.Is(err, node.ErrRejected) {
		// Stay up: Docker would only restart it.
		fmt.Println(err)
		<-ctx.Done()
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Stopped:", err)
		os.Exit(1)
	}
}
