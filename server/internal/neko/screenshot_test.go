package neko_test

import (
	"bytes"
	"context"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cozycast/internal/neko"
	"cozycast/internal/neko/nekotest"
)

func TestScreenshot(t *testing.T) {
	fake := nekotest.New(t, "admin-secret")
	c, err := neko.NewClient(fake.URL(), "admin-secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	screen := nekotest.Screens[2]
	if err := c.SetScreen(ctx, screen); err != nil {
		t.Fatal(err)
	}
	data, err := c.Screenshot(ctx, 80)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != screen.Width || img.Bounds().Dy() != screen.Height {
		t.Fatalf("screenshot bounds = %v", img.Bounds())
	}
	// The fake requires the admin bearer token, even for a logged-in member.
	wrong, err := neko.NewClient(fake.URL(), "wrong", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrong.Screenshot(ctx, 80); err == nil {
		t.Fatal("screenshot accepted a non-admin token")
	}
}

func TestScreenshotRawResponse(t *testing.T) {
	want := []byte{0xff, 0xd8, 0x00, 0x80, 0xff, 0xd9}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/prefix/api/room/screen/shot.jpg" || r.URL.Query().Get("quality") != "80" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("incorrect screenshot request: %s %s, auth %q", r.Method, r.URL, r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(want)
	}))
	defer srv.Close()
	c, err := neko.NewClient(srv.URL+"/prefix", "secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Screenshot(context.Background(), 80)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("Screenshot = %x, %v; want %x", got, err, want)
	}
}

func TestScreenshotBoundsAndRedaction(t *testing.T) {
	for _, mode := range []string{"large", "error"} {
		t.Run(mode, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "error" {
					http.Error(w, "secret", http.StatusBadGateway)
					return
				}
				chunk := make([]byte, 1<<20)
				for range 17 {
					if _, err := w.Write(chunk); err != nil {
						return
					}
				}
			}))
			defer srv.Close()
			c, err := neko.NewClient(srv.URL, "secret", nil)
			if err != nil {
				t.Fatal(err)
			}
			got, err := c.Screenshot(context.Background(), 80)
			if err == nil || got != nil || strings.Contains(err.Error(), "secret") {
				t.Fatalf("Screenshot = %d bytes, %v", len(got), err)
			}
		})
	}
}

func TestScreenshotCancellation(t *testing.T) {
	for _, reconnect := range []bool{false, true} {
		t.Run(map[bool]string{false: "context", true: "generation"}[reconnect], func(t *testing.T) {
			started := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				<-r.Context().Done()
			}))
			defer srv.Close()
			c, err := neko.NewClient(srv.URL, "secret", nil)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := c.Screenshot(ctx, 80); done <- err }()
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("screenshot did not start")
			}
			if reconnect {
				c.Reconnect()
			} else {
				cancel()
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("canceled screenshot succeeded")
				}
			case <-time.After(time.Second):
				t.Fatal("screenshot did not cancel promptly")
			}
		})
	}
}
