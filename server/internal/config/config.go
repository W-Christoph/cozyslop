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
	Listen        string   // HTTP; with Domains set it only redirects and answers ACME
	TLSListen     string   // HTTPS, used when Domains is set
	Domains       []string // enables automatic HTTPS for these host names
	ACMEEmail     string   // optional contact for Let's Encrypt
	DataDir       string   // database and uploaded files
	ImportPath    string   // optional legacy export archive; imported into an empty database
	TrustProxy    bool     // take client IP/scheme from X-Forwarded-* headers
	Docker        bool     // opt in to room container restarts
	DockerSocket  string   // Docker Engine Unix socket
	DockerProject string   // fallback when the server's compose project cannot be detected
	InitAdminPass string   // creates the "admin" account if it does not exist
	NekoAPIToken  string
	Rooms         []Room
	WebDir        string // serve the UI from disk instead of the embedded build (dev)
	SourceURL     string // where users can get this server's source code (AGPL)
	MaxUploadMB   int64  // maximum chat media file size in MiB
}

func FromEnv() (Config, error) {
	c := Config{
		Listen:        env("COZYCAST_LISTEN", ":8080"),
		TLSListen:     env("COZYCAST_TLS_LISTEN", ":8443"),
		Domains:       splitList(os.Getenv("COZYCAST_DOMAIN")),
		ACMEEmail:     os.Getenv("COZYCAST_ACME_EMAIL"),
		DataDir:       env("COZYCAST_DATA_DIR", "data"),
		ImportPath:    os.Getenv("COZYCAST_IMPORT"),
		TrustProxy:    envBool("COZYCAST_TRUST_PROXY", false),
		Docker:        envBool("COZYCAST_DOCKER", false),
		DockerSocket:  env("COZYCAST_DOCKER_SOCKET", "/var/run/docker.sock"),
		DockerProject: os.Getenv("COZYCAST_DOCKER_PROJECT"),
		InitAdminPass: os.Getenv("COZYCAST_INIT_ADMIN_PASSWORD"),
		NekoAPIToken:  os.Getenv("COZYCAST_NEKO_API_TOKEN"),
		WebDir:        os.Getenv("COZYCAST_WEB_DIR"),
		SourceURL:     env("COZYCAST_SOURCE_URL", "https://github.com/W-Christoph/cozyslop"),
	}
	if c.NekoAPIToken == "" {
		return c, errors.New("COZYCAST_NEKO_API_TOKEN is required")
	}
	maxUploadMB, err := strconv.ParseInt(env("COZYCAST_MAX_UPLOAD_MB", "10"), 10, 64)
	if err != nil || maxUploadMB <= 0 || maxUploadMB > ((1<<63-1)-(64<<10))/(1<<20) {
		return c, errors.New("COZYCAST_MAX_UPLOAD_MB must be a positive integer size in MiB")
	}
	c.MaxUploadMB = maxUploadMB

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

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
