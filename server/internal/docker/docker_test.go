package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func unixClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "docker.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(handler)
	if err := srv.Listener.Close(); err != nil {
		t.Fatal(err)
	}
	srv.Listener = listener
	srv.Start()
	t.Cleanup(srv.Close)
	c := New(socket)
	t.Cleanup(c.http.CloseIdleConnections)
	return c
}

func TestOwnProject(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, body, want string
	}{
		{"compose", `{"Config":{"Labels":{"com.docker.compose.project":"cozycast"}}}`, "cozycast"},
		{"no project label", `{"Config":{"Labels":{"other":"label"}}}`, ""},
		{"no labels", `{"Config":{}}`, ""},
		{"empty project", `{"Config":{"Labels":{"com.docker.compose.project":""}}}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := unixClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.EscapedPath() != "/containers/"+url.PathEscape(host)+"/json" {
					t.Errorf("unexpected inspect request: %s %s", r.Method, r.URL)
				}
				fmt.Fprint(w, tc.body)
			})
			got, err := c.OwnProject(context.Background())
			if got != tc.want || (tc.want == "" && !errors.Is(err, ErrNotFound)) || (tc.want != "" && err != nil) {
				t.Fatalf("OwnProject = (%q, %v), want %q", got, err, tc.want)
			}
		})
	}
}

func TestServiceContainer(t *testing.T) {
	for _, project := range []string{"", "cozy project"} {
		for _, tc := range []struct {
			name, body string
			want       Container
			wantErr    string
		}{
			{"zero", `[]`, Container{}, "not found"},
			{"running", `[{"Id":"room-id","State":"running"}]`, Container{ID: "room-id", Running: true}, ""},
			{"stopped", `[{"Id":"room-id","State":"exited"}]`, Container{ID: "room-id"}, ""},
			{"many", `[{"Id":"room-1"},{"Id":"room-2"}]`, Container{}, "2 containers"},
		} {
			t.Run(project+"/"+tc.name, func(t *testing.T) {
				c := unixClient(t, func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodGet || r.URL.Path != "/containers/json" || r.URL.Query().Get("all") != "1" {
						t.Errorf("unexpected list request: %s %s", r.Method, r.URL)
					}
					var filters map[string][]string
					if err := json.Unmarshal([]byte(r.URL.Query().Get("filters")), &filters); err != nil {
						t.Error(err)
					}
					labels := []string{serviceLabel + "=room-default"}
					if project != "" {
						labels = append(labels, projectLabel+"="+project)
					}
					if !reflect.DeepEqual(filters, map[string][]string{"label": labels}) {
						t.Errorf("filters = %v, want labels %v", filters, labels)
					}
					fmt.Fprint(w, tc.body)
				})
				got, err := c.ServiceContainer(context.Background(), project, "room-default")
				if got != tc.want || (tc.wantErr == "" && err != nil) || (tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr))) {
					t.Fatalf("ServiceContainer = (%+v, %v), want (%+v, %q)", got, err, tc.want, tc.wantErr)
				}
				if tc.name == "zero" && !errors.Is(err, ErrNotFound) {
					t.Fatalf("zero matches must wrap ErrNotFound: %v", err)
				}
				if tc.name == "many" && !strings.Contains(err.Error(), "COZYCAST_DOCKER_PROJECT") {
					t.Fatalf("ambiguous matches must suggest project config: %v", err)
				}
			})
		}
	}
}

func TestRestart(t *testing.T) {
	c := unixClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.EscapedPath() != "/containers/room%2Fid/restart" || r.URL.RawQuery != "t=10" {
			t.Errorf("unexpected restart request: %s %s", r.Method, r.URL)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	if err := c.Restart(context.Background(), "room/id", 10*time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestTCP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/containers/json" || r.URL.Query().Get("all") != "1" {
			t.Errorf("unexpected list request: %s %s", r.Method, r.URL)
		}
		fmt.Fprint(w, `[{"Id":"room-id","State":"exited"}]`)
	}))
	t.Cleanup(srv.Close)
	c := New("tcp://" + strings.TrimPrefix(srv.URL, "http://"))
	t.Cleanup(c.http.CloseIdleConnections)
	got, err := c.ServiceContainer(context.Background(), "project", "room-default")
	if got != (Container{ID: "room-id"}) || err != nil {
		t.Fatalf("ServiceContainer = (%+v, %v)", got, err)
	}
}

func TestStartStop(t *testing.T) {
	for _, action := range []string{"start", "stop"} {
		for _, status := range []int{http.StatusNoContent, http.StatusNotModified} {
			t.Run(fmt.Sprintf("%s/%d", action, status), func(t *testing.T) {
				c := unixClient(t, func(w http.ResponseWriter, r *http.Request) {
					query := ""
					if action == "stop" {
						query = "t=10"
					}
					if r.Method != http.MethodPost || r.URL.EscapedPath() != "/containers/room%2Fid/"+action || r.URL.RawQuery != query {
						t.Errorf("unexpected %s request: %s %s", action, r.Method, r.URL)
					}
					w.WriteHeader(status)
				})
				var err error
				if action == "start" {
					err = c.Start(context.Background(), "room/id")
				} else {
					err = c.Stop(context.Background(), "room/id", 10*time.Second)
				}
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestErrorStatuses(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusForbidden, http.StatusInternalServerError} {
		for _, operation := range []string{"inspect", "list", "restart", "start", "stop"} {
			t.Run(fmt.Sprintf("%s/%d", operation, status), func(t *testing.T) {
				c := unixClient(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(status)
					fmt.Fprint(w, "daemon error")
				})
				var err error
				switch operation {
				case "inspect":
					_, err = c.OwnProject(context.Background())
				case "list":
					_, err = c.ServiceContainer(context.Background(), "project", "service")
				case "restart":
					err = c.Restart(context.Background(), "room-id", 10*time.Second)
				case "start":
					err = c.Start(context.Background(), "room-id")
				case "stop":
					err = c.Stop(context.Background(), "room-id", 10*time.Second)
				}
				if status == http.StatusNotFound {
					if !errors.Is(err, ErrNotFound) {
						t.Fatalf("error = %v, want ErrNotFound", err)
					}
				} else if err == nil || !strings.Contains(err.Error(), fmt.Sprint(status)) || !strings.Contains(err.Error(), "daemon error") {
					t.Fatalf("error must include status and body: %v", err)
				}
			})
		}
	}
}
