// Package docker controls room containers through the Docker Engine API.
// It is only used when the operator opts in, and it only controls containers
// selected by their compose service and project labels.
package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"
)

const (
	projectLabel = "com.docker.compose.project"
	serviceLabel = "com.docker.compose.service"
)

var ErrNotFound = errors.New("docker: not found")

type Client struct {
	http     *http.Client
	endpoint string
}

type Container struct {
	ID      string
	Running bool
}

// New talks to Docker on a Unix socket or a tcp://host:port HTTP endpoint.
func New(endpoint string) *Client {
	transport := &http.Transport{}
	base := "http://docker"
	if strings.HasPrefix(endpoint, "tcp://") {
		base = "http://" + strings.TrimPrefix(endpoint, "tcp://")
	} else {
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", endpoint)
		}
	}
	return &Client{http: &http.Client{
		Transport: transport,
		Timeout:   time.Minute,
	}, endpoint: base}
}

// OwnProject returns the compose project of the container this process runs
// in, or ErrNotFound when it does not run in a (compose) container.
func (c *Client) OwnProject(ctx context.Context) (string, error) {
	host, err := os.Hostname()
	if err != nil {
		return "", err
	}
	var info struct {
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if err := c.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(host)+"/json", &info); err != nil {
		return "", err
	}
	project := info.Config.Labels[projectLabel]
	if project == "" {
		return "", ErrNotFound
	}
	return project, nil
}

// ServiceContainer returns the container of a compose service, including
// stopped containers, and whether it is running. With project "", any
// project's service of that name matches, as long as there is exactly one.
func (c *Client) ServiceContainer(ctx context.Context, project, service string) (Container, error) {
	labels := []string{serviceLabel + "=" + service}
	if project != "" {
		labels = append(labels, projectLabel+"="+project)
	}
	filters, err := json.Marshal(map[string][]string{"label": labels})
	if err != nil {
		return Container{}, err
	}
	var list []struct {
		ID    string `json:"Id"`
		State string `json:"State"`
	}
	if err := c.do(ctx, http.MethodGet, "/containers/json?all=1&filters="+url.QueryEscape(string(filters)), &list); err != nil {
		return Container{}, err
	}
	switch len(list) {
	case 0:
		return Container{}, fmt.Errorf("%w: container for service %q", ErrNotFound, service)
	case 1:
		return Container{ID: list[0].ID, Running: list[0].State == "running"}, nil
	default:
		return Container{}, fmt.Errorf("docker: %d containers for service %q; set COZYCAST_DOCKER_PROJECT", len(list), service)
	}
}

// Restart restarts a container, giving it timeout to stop.
func (c *Client) Restart(ctx context.Context, id string, timeout time.Duration) error {
	path := fmt.Sprintf("/containers/%s/restart?t=%d", url.PathEscape(id), int(timeout.Seconds()))
	return c.do(ctx, http.MethodPost, path, nil)
}

// Start starts a container; an already running container is a success.
func (c *Client) Start(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/start", nil, http.StatusNotModified)
}

// Stop stops a container, giving it timeout; an already stopped container is a success.
func (c *Client) Stop(ctx context.Context, id string, timeout time.Duration) error {
	path := fmt.Sprintf("/containers/%s/stop?t=%d", url.PathEscape(id), int(timeout.Seconds()))
	return c.do(ctx, http.MethodPost, path, nil, http.StatusNotModified)
}

func (c *Client) do(ctx context.Context, method, path string, out any, success ...int) error {
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, nil)
	if err != nil {
		return err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("docker: %s %s: %w", method, path, err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if res.StatusCode >= 300 && !slices.Contains(success, res.StatusCode) {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("docker: %s %s: %s: %s", method, path, res.Status, msg)
	}
	if out != nil {
		return json.NewDecoder(res.Body).Decode(out)
	}
	return nil
}
