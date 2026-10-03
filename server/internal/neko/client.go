// Package neko is a small client for the parts of the neko v3 REST API that
// CozyCast drives: member management and login. CozyCast acts as the neko
// admin (via session.api_token); browsers only ever receive per-member
// session tokens.
package neko

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	base  *url.URL
	token string
	http  *http.Client
}

func NewClient(baseURL, apiToken string) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("neko: invalid base url %q: %w", baseURL, err)
	}
	return &Client{
		base:  u,
		token: apiToken,
		http:  &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// BaseURL is the neko server address, used by the reverse proxy.
func (c *Client) BaseURL() *url.URL { return c.base }

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

	req, err := http.NewRequestWithContext(ctx, method, c.base.JoinPath(path).String(), body)
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
		return fmt.Errorf("neko: %s %s: %s: %s", method, path, res.Status, bytes.TrimSpace(msg))
	}
	if out != nil {
		return json.NewDecoder(res.Body).Decode(out)
	}
	return nil
}
