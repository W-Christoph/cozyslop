// Package config reads server settings from the environment.
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"cozycast/internal/neko"
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
	NekoSecret    string   // each room's neko admin token is derived from it (see NekoToken)
	NekoAPIToken  string   // instead of NekoSecret: one token for every neko, as given
	DefaultScreen string   // container default desktop size for all rooms; empty leaves it alone
	Rooms         []Room
	WebDir        string // serve the UI from disk instead of the embedded build (dev)
	SourceURL     string // where users can get this server's source code (AGPL)
	MaxUploadMB   int64  // maximum chat media file size in MiB
	// Tunnels to rooms on other machines (docs/home-hosting.md); off when
	// TunnelPort is 0. The server takes TunnelNet's first address.
	TunnelPort int
	TunnelNet  netip.Prefix
	// What paired rooms' media goes through: the server's public address,
	// announced to viewers, and its ports for them, one per room.
	PublicIP   netip.Addr // invalid if not set
	MediaPorts [2]int     // first and last
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
		NekoSecret:    os.Getenv("COZYCAST_NEKO_SECRET"),
		NekoAPIToken:  os.Getenv("COZYCAST_NEKO_API_TOKEN"),
		DefaultScreen: os.Getenv("COZYCAST_DEFAULT_SCREEN"),
		WebDir:        os.Getenv("COZYCAST_WEB_DIR"),
		SourceURL:     env("COZYCAST_SOURCE_URL", "https://github.com/W-Christoph/cozyslop"),
	}
	if c.NekoSecret == "" && c.NekoAPIToken == "" {
		return c, errors.New("COZYCAST_NEKO_SECRET is required")
	}
	maxUploadMB, err := strconv.ParseInt(env("COZYCAST_MAX_UPLOAD_MB", "10"), 10, 64)
	if err != nil || maxUploadMB <= 0 || maxUploadMB > ((1<<63-1)-(64<<10))/(1<<20) {
		return c, errors.New("COZYCAST_MAX_UPLOAD_MB must be a positive integer size in MiB")
	}
	c.MaxUploadMB = maxUploadMB
	if c.DefaultScreen != "" {
		size, err := neko.ParseScreen(c.DefaultScreen)
		if err != nil || size.Width <= 0 || size.Height <= 0 || size.Rate <= 0 {
			return c, errors.New("COZYCAST_DEFAULT_SCREEN must look like 1280x720@30 with positive dimensions and frame rate")
		}
	}

	if port := os.Getenv("COZYCAST_TUNNEL_PORT"); port != "" {
		c.TunnelPort, err = strconv.Atoi(port)
		if err != nil || c.TunnelPort < 1 || c.TunnelPort > 65535 {
			return c, errors.New("COZYCAST_TUNNEL_PORT must be a UDP port number")
		}
	}
	c.TunnelNet, err = netip.ParsePrefix(env("COZYCAST_TUNNEL_NET", "10.77.0.0/24"))
	if err != nil || !c.TunnelNet.Addr().Is4() || c.TunnelNet.Bits() > 30 || c.TunnelNet != c.TunnelNet.Masked() {
		return c, errors.New("COZYCAST_TUNNEL_NET must be an IPv4 network like 10.77.0.0/24, /30 or larger")
	}

	if ip := os.Getenv("COZYCAST_PUBLIC_IP"); ip != "" {
		if c.PublicIP, err = netip.ParseAddr(ip); err != nil {
			return c, errors.New("COZYCAST_PUBLIC_IP must be an IP address")
		}
	}
	first, last, ok := strings.Cut(env("COZYCAST_MEDIA_PORTS", "52100-52109"), "-")
	c.MediaPorts[0], err = strconv.Atoi(first)
	if err == nil {
		c.MediaPorts[1], err = strconv.Atoi(last)
	}
	if !ok || err != nil || c.MediaPorts[0] < 1 || c.MediaPorts[1] > 65535 || c.MediaPorts[0] > c.MediaPorts[1] {
		return c, errors.New("COZYCAST_MEDIA_PORTS must be a port range like 52100-52109")
	}

	rooms, err := parseRooms(env("COZYCAST_ROOMS", "default=http://room-default:8080"))
	if err != nil {
		return c, err
	}
	c.Rooms = rooms
	return c, nil
}

// NekoToken is the admin token of the named room's neko. With a secret,
// every room has its own: the room's desktop user can read their neko's
// token, and it must not open the other rooms. worker/entrypoint.sh derives
// the same value.
func (c Config) NekoToken(room string) string {
	if c.NekoSecret == "" {
		return c.NekoAPIToken
	}
	sum := sha256.Sum256([]byte("cozycast-neko-token:" + room + ":" + c.NekoSecret))
	return hex.EncodeToString(sum[:])
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

var roomName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func ValidRoomName(name string) bool { return roomName.MatchString(name) }

// ValidateNekoURL accepts a server address, optionally with a path prefix.
func ValidateNekoURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") ||
		u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") {
		return errors.New("Neko URL must be an absolute http or https URL without credentials, query or fragment.")
	}
	return nil
}
