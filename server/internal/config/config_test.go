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

func TestNekoToken(t *testing.T) {
	t.Setenv("COZYCAST_NEKO_API_TOKEN", "")
	t.Setenv("COZYCAST_NEKO_SECRET", "")
	if _, err := FromEnv(); err == nil {
		t.Fatal("no neko secret or token accepted")
	}
	t.Setenv("COZYCAST_NEKO_SECRET", "secret")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	// worker/entrypoint.sh: printf 'cozycast-neko-token:%s:%s' default secret | sha256sum
	if got := cfg.NekoToken("default"); got != "b6647a3bddfd3258ed8d4fa8a9f3e4bbb5d8f0c99383b7a847419a9f7d2bc18e" {
		t.Fatal("derived token:", got)
	}
	if cfg.NekoToken("second") == cfg.NekoToken("default") {
		t.Fatal("two rooms share a token")
	}
	t.Setenv("COZYCAST_NEKO_SECRET", "")
	t.Setenv("COZYCAST_NEKO_API_TOKEN", "plain")
	if cfg, err = FromEnv(); err != nil || cfg.NekoToken("default") != "plain" || cfg.NekoToken("second") != "plain" {
		t.Fatal("plain token:", cfg.NekoToken("default"), err)
	}
}

func TestDefaultScreenConfig(t *testing.T) {
	t.Setenv("COZYCAST_NEKO_API_TOKEN", "test-secret")
	t.Setenv("COZYCAST_MAX_UPLOAD_MB", "10")
	for _, tc := range []struct {
		screen string
		valid  bool
	}{
		{"", true}, {"1280x720@30", true}, {"1920x1080@60", true},
		{"invalid", false}, {"1280x720", false}, {"1280x720@30junk", false},
		{"0x720@30", false}, {"1280x-720@30", false}, {"1280x720@0", false},
	} {
		t.Run(tc.screen, func(t *testing.T) {
			t.Setenv("COZYCAST_DEFAULT_SCREEN", tc.screen)
			cfg, err := FromEnv()
			if (err == nil) != tc.valid || (tc.valid && cfg.DefaultScreen != tc.screen) {
				t.Fatalf("default screen %q: %q, %v", tc.screen, cfg.DefaultScreen, err)
			}
		})
	}
}

func TestRegisteredRoomValidation(t *testing.T) {
	for _, name := range []string{"default", "room-second", "Room_2", "123"} {
		if !ValidRoomName(name) {
			t.Errorf("name rejected: %q", name)
		}
	}
	for _, name := range []string{"", "a/b", "..", "a b", "a?b", "a=b", "a,b"} {
		if ValidRoomName(name) {
			t.Errorf("unsafe name accepted: %q", name)
		}
	}
	for _, raw := range []string{"http://neko:8080", "https://neko/prefix/", "http://[::1]:8080"} {
		if err := ValidateNekoURL(raw); err != nil {
			t.Errorf("URL %s: %v", raw, err)
		}
	}
	for _, raw := range []string{"neko", "//neko", "ftp://neko", "http:///path", "http://user:pass@neko", "http://neko?", "http://neko?x=1", "http://neko#", "http://neko#fragment", "http://neko:bad"} {
		if err := ValidateNekoURL(raw); err == nil {
			t.Errorf("unsafe URL accepted: %q", raw)
		}
	}
	if _, err := parseRooms(""); err == nil {
		t.Fatal("empty configured room list accepted")
	}
}

func TestTunnelConfig(t *testing.T) {
	t.Setenv("COZYCAST_NEKO_API_TOKEN", "test-secret")
	for _, tc := range []struct {
		port, network string
		wantPort      int
		wantNet       string
		ok            bool
	}{
		{wantNet: "10.77.0.0/24", ok: true},
		{port: "51820", wantPort: 51820, wantNet: "10.77.0.0/24", ok: true},
		{port: "51820", network: "10.200.0.0/16", wantPort: 51820, wantNet: "10.200.0.0/16", ok: true},
		{port: "0"},
		{port: "70000"},
		{port: "udp"},
		{network: "10.77.0.1/24"},
		{network: "10.77.0.0/31"},
		{network: "fd00::/64"},
		{network: "nonsense"},
	} {
		t.Setenv("COZYCAST_TUNNEL_PORT", tc.port)
		t.Setenv("COZYCAST_TUNNEL_NET", tc.network)
		cfg, err := FromEnv()
		if (err == nil) != tc.ok {
			t.Errorf("%+v: err=%v", tc, err)
			continue
		}
		if tc.ok && (cfg.TunnelPort != tc.wantPort || cfg.TunnelNet.String() != tc.wantNet) {
			t.Errorf("%+v: got %d %s", tc, cfg.TunnelPort, cfg.TunnelNet)
		}
	}
}
