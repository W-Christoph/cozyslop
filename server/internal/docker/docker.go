// Package docker restarts room containers through the Docker Engine API.
// It is only used when the operator opts in by mounting the Docker socket,
// and it only ever restarts containers of this compose project.
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
	"time"
)

const (
	projectLabel = "com.docker.compose.project"
	serviceLabel = "com.docker.compose.service"
)

var ErrNotFound = errors.New("docker: not found")

type Client struct {
	http *http.Client
}

// New talks to the Docker daemon on the given Unix socket.
func New(socket string) *Client {
	return &Client{http: &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socket)
			},
		},
		Timeout: time.Minute,
	}}
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

// ServiceContainer returns the id of the running container of a compose
// service. With project "", any project's service of that name matches, as
// long as there is exactly one.
func (c *Client) ServiceContainer(ctx context.Context, project, service string) (string, error) {
	labels := []string{serviceLabel + "=" + service}
	if project != "" {
		labels = append(labels, projectLabel+"="+project)
	}
	filters, err := json.Marshal(map[string][]string{"label": labels})
	if err != nil {
		return "", err
	}
	var list []struct {
		ID string `json:"Id"`
	}
	if err := c.do(ctx, http.MethodGet, "/containers/json?filters="+url.QueryEscape(string(filters)), &list); err != nil {
		return "", err
	}
	switch len(list) {
	case 0:
		return "", fmt.Errorf("%w: container for service %q", ErrNotFound, service)
	case 1:
		return list[0].ID, nil
	default:
		return "", fmt.Errorf("docker: %d containers for service %q; set COZYCAST_DOCKER_PROJECT", len(list), service)
	}
}

// Restart restarts a container, giving it timeout to stop.
func (c *Client) Restart(ctx context.Context, id string, timeout time.Duration) error {
	path := fmt.Sprintf("/containers/%s/restart?t=%d", url.PathEscape(id), int(timeout.Seconds()))
	return c.do(ctx, http.MethodPost, path, nil)
}

func (c *Client) do(ctx context.Context, method, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, "http://docker"+path, nil)
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
	if res.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("docker: %s %s: %s: %s", method, path, res.Status, msg)
	}
	if out != nil {
		return json.NewDecoder(res.Body).Decode(out)
	}
	return nil
}
