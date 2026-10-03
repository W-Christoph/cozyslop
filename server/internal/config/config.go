// Package config reads server settings from the environment.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Room struct {
	Name    string
	NekoURL string
}

type Config struct {
	Listen        string
	NekoAPIToken  string
	Rooms         []Room
	WebDir        string // serve the UI from disk instead of the embedded build (dev)
	DefaultRemote bool
	DefaultUpload bool
}

func FromEnv() (Config, error) {
	c := Config{
		Listen:        env("COZYCAST_LISTEN", ":8080"),
		NekoAPIToken:  os.Getenv("COZYCAST_NEKO_API_TOKEN"),
		WebDir:        os.Getenv("COZYCAST_WEB_DIR"),
		DefaultRemote: envBool("COZYCAST_DEFAULT_REMOTE", true),
		DefaultUpload: envBool("COZYCAST_DEFAULT_UPLOAD", false),
	}
	if c.NekoAPIToken == "" {
		return c, errors.New("COZYCAST_NEKO_API_TOKEN is required")
	}

	rooms, err := parseRooms(env("COZYCAST_ROOMS", "default=http://room-default:8080"))
	if err != nil {
		return c, err
	}
	c.Rooms = rooms
	return c, nil
}

// parseRooms reads "name=url,name2=url2".
func parseRooms(s string) ([]Room, error) {
	var rooms []Room
	seen := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, url, ok := strings.Cut(part, "=")
		if !ok || name == "" || url == "" {
			return nil, fmt.Errorf("COZYCAST_ROOMS: expected name=url, got %q", part)
		}
		if seen[name] {
			return nil, fmt.Errorf("COZYCAST_ROOMS: duplicate room %q", name)
		}
		seen[name] = true
		rooms = append(rooms, Room{Name: name, NekoURL: url})
	}
	if len(rooms) == 0 {
		return nil, errors.New("COZYCAST_ROOMS: no rooms configured")
	}
	return rooms, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v, err := strconv.ParseBool(os.Getenv(key))
	if err != nil {
		return def
	}
	return v
}
