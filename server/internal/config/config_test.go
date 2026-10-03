package config

import "testing"

func TestDockerConfig(t *testing.T) {
	t.Setenv("COZYCAST_NEKO_API_TOKEN", "test-secret")
	t.Setenv("COZYCAST_ROOMS", "default=http://room-default:8080")
	t.Setenv("COZYCAST_MAX_UPLOAD_MB", "10")
	for _, tc := range []struct {
		name, enabled, socket, project string
		wantEnabled                    bool
		wantSocket                     string
	}{
		{name: "default", wantSocket: "/var/run/docker.sock"},
		{name: "disabled", enabled: "false", wantSocket: "/var/run/docker.sock"},
		{name: "enabled", enabled: "true", wantEnabled: true, wantSocket: "/var/run/docker.sock"},
		{name: "custom", enabled: "true", socket: "/run/custom.sock", project: "cozycast", wantEnabled: true, wantSocket: "/run/custom.sock"},
		{name: "socket alone stays off", socket: "/run/custom.sock", project: "cozycast", wantSocket: "/run/custom.sock"},
		{name: "invalid stays off", enabled: "invalid", wantSocket: "/var/run/docker.sock"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("COZYCAST_DOCKER", tc.enabled)
			t.Setenv("COZYCAST_DOCKER_SOCKET", tc.socket)
			t.Setenv("COZYCAST_DOCKER_PROJECT", tc.project)
			cfg, err := FromEnv()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Docker != tc.wantEnabled || cfg.DockerSocket != tc.wantSocket || cfg.DockerProject != tc.project {
				t.Fatalf("Docker config = (%v, %q, %q), want (%v, %q, %q)", cfg.Docker, cfg.DockerSocket, cfg.DockerProject, tc.wantEnabled, tc.wantSocket, tc.project)
			}
		})
	}
}
