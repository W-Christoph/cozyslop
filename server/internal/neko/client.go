// Package neko is a small client for the parts of the neko v3 REST API that
// CozyCast drives: member management and login. CozyCast acts as the neko
// admin (via session.api_token); browsers only ever receive per-member
// session tokens.
package neko

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

// Profile mirrors neko's MemberProfile. Every field is always sent: neko
// treats omitted booleans as "allowed", which is the wrong default for us.
type Profile struct {
	Name                  string         `json:"name"`
	Avatar                string         `json:"avatar"`
	IsAdmin               bool           `json:"is_admin"`
	CanLogin              bool           `json:"can_login"`
	CanConnect            bool           `json:"can_connect"`
	CanWatch              bool           `json:"can_watch"`
	CanHost               bool           `json:"can_host"`
	CanShareMedia         bool           `json:"can_share_media"`
	CanAccessClipboard    bool           `json:"can_access_clipboard"`
	SendsInactiveCursor   bool           `json:"sends_inactive_cursor"`
	CanSeeInactiveCursors bool           `json:"can_see_inactive_cursors"`
	Plugins               map[string]any `json:"plugins"`
}

type Member struct {
	ID      string  `json:"id"`
	Profile Profile `json:"profile"`
}

var ErrNotFound = errors.New("neko: not found")

type Client struct {
	connected atomic.Bool
	base      *url.URL
	token     string
	http      *http.Client
	transport http.RoundTripper
	stream    *http.Client // for WebSockets: no overall timeout
}

// DialFunc opens connections to neko, e.g. through a tunnel.
type DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// NewClient talks to the neko at baseURL. With dial, every connection to
// it (API, WebSockets, file transfers, the desktop's helpers) goes through
// dial instead of the network, and never through an HTTP proxy.
func NewClient(baseURL, apiToken string, dial DialFunc) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("neko: invalid base url %q: %w", baseURL, err)
	}
	transport := http.DefaultTransport
	if dial != nil {
		transport = &http.Transport{
			DialContext:         dial,
			MaxIdleConnsPerHost: 4,
			IdleConnTimeout:     90 * time.Second,
		}
	}
	return &Client{
		base:      u,
		token:     apiToken,
		http:      &http.Client{Timeout: 10 * time.Second, Transport: transport},
		transport: transport,
		stream:    &http.Client{Transport: transport},
	}, nil
}

// BaseURL is the neko server address, used by the reverse proxy.
func (c *Client) BaseURL() *url.URL { return c.base }

// Transport reaches this neko and the desktop's helpers next to it.
func (c *Client) Transport() http.RoundTripper { return c.transport }

// StreamClient is for WebSockets to this neko.
func (c *Client) StreamClient() *http.Client { return c.stream }

func (c *Client) CreateMember(ctx context.Context, id, password string, p Profile) error {
	body := map[string]any{"username": id, "password": password, "profile": p}
	return c.do(ctx, http.MethodPost, "/api/members", body, nil, true)
}

func (c *Client) UpdateProfile(ctx context.Context, id string, p Profile) error {
	return c.do(ctx, http.MethodPost, "/api/members/"+url.PathEscape(id), p, nil, true)
}

func (c *Client) DeleteMember(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/members/"+url.PathEscape(id), nil, nil, true)
}

func (c *Client) ListMembers(ctx context.Context) ([]Member, error) {
	var members []Member
	err := c.do(ctx, http.MethodGet, "/api/members", nil, &members, true)
	return members, err
}

// Login creates a neko session for the member and returns its token.
// It is sent without the admin token so neko authenticates the member.
func (c *Client) Login(ctx context.Context, id, password string) (string, error) {
	var res struct {
		Token string `json:"token"`
	}
	body := map[string]string{"username": id, "password": password}
	if err := c.do(ctx, http.MethodPost, "/api/login", body, &res, false); err != nil {
		return "", err
	}
	if res.Token == "" {
		return "", errors.New("neko: login returned no token (are cookies disabled?)")
	}
	return res.Token, nil
}

// DeleteFile removes a file from the desktop's Downloads folder (neko's file
// transfer plugin). ErrNotFound if there is no such file.
func (c *Client) DeleteFile(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/api/filetransfer?filename="+url.QueryEscape(name), nil, nil, true)
}

func (c *Client) Healthy(ctx context.Context) bool {
	return c.do(ctx, http.MethodGet, "/health", nil, nil, false) == nil
}

func (c *Client) do(ctx context.Context, method, path string, in, out any, admin bool) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}

	path, query, _ := strings.Cut(path, "?")
	u := c.base.JoinPath(path)
	u.RawQuery = query
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if admin {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("neko: %s %s: %w", method, path, err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if res.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		message := string(bytes.TrimSpace(msg))
		if c.token != "" {
			message = strings.ReplaceAll(message, c.token, "[redacted]")
		}
		return fmt.Errorf("neko: %s %s: %s: %s", method, path, res.Status, message)
	}
	if out != nil {
		return json.NewDecoder(res.Body).Decode(out)
	}
	return nil
}

func randomPassword() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// ScreenSize is a desktop resolution and refresh rate.
type ScreenSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
	Rate   int `json:"rate"`
}

func (s ScreenSize) String() string { return fmt.Sprintf("%dx%d@%d", s.Width, s.Height, s.Rate) }

// Widescreen reports whether the resolution is exactly 16:9.
func (s ScreenSize) Widescreen() bool { return s.Width*9 == s.Height*16 }

// ParseScreen reads "1280x720@30".
func ParseScreen(str string) (ScreenSize, error) {
	var s ScreenSize
	if _, err := fmt.Sscanf(str, "%dx%d@%d", &s.Width, &s.Height, &s.Rate); err != nil || s.String() != str {
		return ScreenSize{}, fmt.Errorf("neko: invalid screen %q", str)
	}
	return s, nil
}

// SetScreen changes the desktop resolution and frame rate.
func (c *Client) SetScreen(ctx context.Context, s ScreenSize) error {
	return c.do(ctx, http.MethodPost, "/api/room/screen", s, nil, true)
}

// Screen returns the current desktop size.
func (c *Client) Screen(ctx context.Context) (ScreenSize, error) {
	var s ScreenSize
	err := c.do(ctx, http.MethodGet, "/api/room/screen", nil, &s, true)
	return s, err
}

// ScreenConfigurations lists the resolutions the desktop supports.
func (c *Client) ScreenConfigurations(ctx context.Context) ([]ScreenSize, error) {
	var list []ScreenSize
	err := c.do(ctx, http.MethodGet, "/api/room/screen/configurations", nil, &list, true)
	return list, err
}

// Connected reports whether the observer currently has a working event stream.
func (c *Client) Connected() bool { return c.connected.Load() }
