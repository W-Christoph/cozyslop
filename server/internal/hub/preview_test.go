package hub

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cozycast/internal/neko"
	"cozycast/internal/neko/nekotest"
	"cozycast/internal/store"
)

func screenshotCalls(fake *nekotest.Server) (n int) {
	for _, call := range fake.Calls() {
		if call.Path == "/api/room/screen/shot.jpg" {
			n++
		}
	}
	return n
}

func TestPreviewSizeAndCache(t *testing.T) {
	for _, screen := range []neko.ScreenSize{nekotest.Screens[0], nekotest.Screens[2]} {
		t.Run(screen.String(), func(t *testing.T) {
			f := newFixture(t, store.RoomSettings{})
			requireOK(t, f.r.neko.SetScreen(f.ctx, screen))
			c, rec := f.join(anon("viewer"))
			other, otherRec := f.join(anon("other"))
			before := time.Now().UnixMilli()
			f.r.Handle(f.ctx, c, ClientMsg{Type: "preview"})
			first := rec.wait(t, "preview").(previewMsg)
			if !strings.HasPrefix(first.Image, "data:image/jpeg;base64,") || first.Time < before || first.Time > time.Now().UnixMilli() {
				t.Fatalf("invalid preview time/data URL: time=%d", first.Time)
			}
			data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(first.Image, "data:image/jpeg;base64,"))
			requireOK(t, err)
			img, err := jpeg.Decode(bytes.NewReader(data))
			requireOK(t, err)
			w, h := img.Bounds().Dx(), img.Bounds().Dy()
			if w > 320 || h > 180 || w*screen.Height != h*screen.Width {
				t.Fatalf("preview %dx%d does not fit screen %s", w, h, screen)
			}
			requireEqual(t, otherRec.count("preview"), 0)
			f.r.Handle(f.ctx, other, ClientMsg{Type: "preview"})
			requireEqual(t, otherRec.wait(t, "preview").(previewMsg), first)
			requireEqual(t, screenshotCalls(f.fake), 1)
			// Age the shot rather than making every test sleep for the TTL.
			f.r.mu.Lock()
			f.r.preview.taken = time.Now().Add(-previewCacheAge - time.Millisecond)
			f.r.mu.Unlock()
			f.r.Handle(f.ctx, c, ClientMsg{Type: "preview"})
			rec.wait(t, "preview")
			requireEqual(t, screenshotCalls(f.fake), 2)
		})
	}
}

// previewLinkFixture intercepts screenshot calls while forwarding every other
// request to nekotest, just like a slow or broken link to a room at home.
func previewLinkFixture(t *testing.T, shot http.HandlerFunc) *fixture {
	t.Helper()
	f := newFixture(t, store.RoomSettings{})
	fake := nekotest.New(t, "secret")
	target, err := url.Parse(fake.URL())
	requireOK(t, err)
	forward := httputil.NewSingleHostReverseProxy(target)
	link := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/room/screen/shot.jpg" {
			shot(w, r)
			return
		}
		forward.ServeHTTP(w, r)
	}))
	t.Cleanup(link.Close)
	nc, err := neko.NewClient(link.URL, "secret", nil)
	requireOK(t, err)
	requireOK(t, f.h.Add(f.ctx, RoomConfig{Name: "preview", Source: "registered", Neko: nc}))
	r := f.h.Room("preview")
	t.Cleanup(func() { f.h.Remove("preview", "not_found") })
	select {
	case <-r.ready:
	case <-time.After(testTimeout):
		t.Fatal("preview room did not become ready")
	}
	return &fixture{t, f.ctx, f.s, f.h, r, fake}
}

func waitPreviewState(t *testing.T, r *Room, ready func() bool) {
	t.Helper()
	deadline := time.NewTimer(testTimeout)
	defer deadline.Stop()
	for {
		r.mu.Lock()
		ok := ready()
		r.mu.Unlock()
		if ok {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("preview did not reach expected state")
		case <-time.After(time.Millisecond):
		}
	}
}

func TestPreviewConcurrentTabsAndBusyTab(t *testing.T) {
	release := make(chan struct{})
	answer := sync.OnceFunc(func() { close(release) })
	var requests atomic.Int32
	var forward http.Handler
	f := previewLinkFixture(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		select {
		case <-release:
			forward.ServeHTTP(w, r)
		case <-r.Context().Done():
		}
	})
	target, err := url.Parse(f.fake.URL())
	requireOK(t, err)
	forward = httputil.NewSingleHostReverseProxy(target)
	t.Cleanup(answer)
	var tabs []*Client
	var recs []*recording
	for i := range 6 {
		c, rec := f.join(anon(fmt.Sprint(i)))
		tabs, recs = append(tabs, c), append(recs, rec)
		f.r.Handle(f.ctx, c, ClientMsg{Type: "preview"})
	}
	waitPreviewState(t, f.r, func() bool {
		for _, c := range tabs {
			if !c.previewBusy {
				return false
			}
		}
		return true
	})
	// These calls return immediately because the tab already has a waiter.
	for range 3 {
		f.r.sendPreview(f.ctx, tabs[0])
	}
	f.r.Handle(f.ctx, tabs[0], ClientMsg{Type: "chat_send", Body: "still here"})
	requireEqual(t, recs[0].wait(t, "chat").(chatMsg).Message.Body, "still here")
	// Admission can be revoked while a tab is waiting for the shared shot.
	kicked, kickedRec := f.join(anon("kicked"))
	f.r.Handle(f.ctx, kicked, ClientMsg{Type: "preview"})
	waitPreviewState(t, f.r, func() bool { return kicked.previewBusy })
	requireOK(t, f.r.Kick(kicked.m.key))
	answer()
	var first previewMsg
	for i, rec := range recs {
		msg := rec.wait(t, "preview").(previewMsg)
		if i == 0 {
			first = msg
			if msg.Image == "" {
				t.Fatal("empty concurrent preview")
			}
		}
		requireEqual(t, msg, first)
		requireEqual(t, rec.count("preview"), 1)
	}
	requireEqual(t, requests.Load(), 1)
	requireEqual(t, screenshotCalls(f.fake), 1)
	waitPreviewState(t, f.r, func() bool { return !kicked.previewBusy })
	requireEqual(t, kickedRec.count("preview"), 0)
}

func TestPreviewOffline(t *testing.T) {
	f := newFixture(t, store.RoomSettings{})
	c, rec := f.join(anon("viewer"))
	f.r.Handle(f.ctx, c, ClientMsg{Type: "preview"})
	rec.wait(t, "preview") // even a fresh cached image is hidden while offline
	f.r.mu.Lock()
	f.r.offline = true
	f.r.mu.Unlock()
	f.r.Handle(f.ctx, c, ClientMsg{Type: "preview"})
	msg := rec.wait(t, "preview").(previewMsg)
	requireEqual(t, msg.Image, "")
	data, err := json.Marshal(msg)
	requireOK(t, err)
	requireEqual(t, string(data), `{"type":"preview","image":""}`)
	requireEqual(t, screenshotCalls(f.fake), 1)
}

func TestPreviewFailure(t *testing.T) {
	for _, mode := range []string{"neko error", "bad JPEG", "truncated JPEG"} {
		t.Run(mode, func(t *testing.T) {
			f := previewLinkFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if mode == "neko error" {
					http.Error(w, "unreachable", http.StatusBadGateway)
					return
				}
				if mode == "truncated JPEG" {
					var buf bytes.Buffer
					_ = jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 20, 20)), nil)
					_, _ = w.Write(buf.Bytes()[:buf.Len()-20])
					return
				}
				_, _ = w.Write([]byte("not a JPEG"))
			})
			c, rec := f.join(anon("viewer"))
			f.r.Handle(f.ctx, c, ClientMsg{Type: "preview"})
			requireEqual(t, rec.wait(t, "preview").(previewMsg), previewMsg{Type: "preview"})
		})
	}
}

func TestPreviewCanceledWaiterAndRoomRemoval(t *testing.T) {
	started, canceled := make(chan struct{}), make(chan struct{})
	f := previewLinkFixture(t, func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(canceled)
	})
	c, rec := f.join(anon("viewer"))
	other, otherRec := f.join(anon("other"))
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); f.r.sendPreview(ctx, c) }()
	select {
	case <-started:
	case <-time.After(testTimeout):
		t.Fatal("screenshot did not start")
	}
	f.r.Handle(f.ctx, other, ClientMsg{Type: "preview"})
	waitPreviewState(t, f.r, func() bool { return other.previewBusy })
	cancel()
	select {
	case <-done:
	case <-time.After(testTimeout):
		t.Fatal("canceled tab kept waiting")
	}
	select {
	case <-canceled:
		t.Fatal("one tab canceled the shared capture")
	default:
	}
	f.h.Remove(f.r.Name, "not_found")
	select {
	case <-canceled:
	case <-time.After(testTimeout):
		t.Fatal("room removal did not cancel the capture")
	}
	waitPreviewState(t, f.r, func() bool { return !other.previewBusy && f.r.previewFlight == nil })
	requireEqual(t, rec.count("preview"), 0)
	requireEqual(t, otherRec.count("preview"), 0)
}

func TestShrinkPreviewBounds(t *testing.T) {
	for _, size := range []image.Point{{80, 60}, {400, 800}, {1920, 1080}, {1001, 1000}, {1, 400}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			var source bytes.Buffer
			requireOK(t, jpeg.Encode(&source, image.NewRGBA(image.Rectangle{Max: size}), nil))
			data, err := shrinkPreview(source.Bytes())
			requireOK(t, err)
			img, err := jpeg.Decode(bytes.NewReader(data))
			requireOK(t, err)
			w, h := img.Bounds().Dx(), img.Bounds().Dy()
			if w > 320 || h > 180 || w > size.X || h > size.Y || w < 1 || h < 1 {
				t.Fatalf("invalid preview bounds: %v", img.Bounds())
			}
			if size.X <= 320 && size.Y <= 180 {
				requireEqual(t, img.Bounds().Size(), size)
			}
		})
	}
}
