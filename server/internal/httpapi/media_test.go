package httpapi_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"cozycast/internal/auth"
	"cozycast/internal/config"
	"cozycast/internal/httpapi"
	"cozycast/internal/hub"
	"cozycast/internal/neko"
	"cozycast/internal/store"

	"github.com/coder/websocket/wsjson"
)

type mediaAPITest struct {
	*apiTest
	dir string
	hub *hub.Hub
}

func newMediaAPITest(t *testing.T, maxMB int64) *mediaAPITest {
	t.Helper()
	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	dir := t.TempDir()
	for _, subdir := range []string{"avatars", "chat"} {
		if err := os.Mkdir(filepath.Join(dir, subdir), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	nc, err := neko.NewClient("http://127.0.0.1:1", "x")
	if err != nil {
		t.Fatal(err)
	}
	h := hub.New(st, filepath.Join(dir, "chat"), []hub.RoomConfig{{Name: "default", Neko: nc}})
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Store: st, Auth: auth.New(st, false), Hub: h, MediaDir: dir, MaxUploadMB: maxMB,
	}).Handler())
	t.Cleanup(srv.Close)
	return &mediaAPITest{apiTest: &apiTest{t: t, st: st, srv: srv}, dir: dir, hub: h}
}

func testImage(t *testing.T, format string) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 60, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 60; x++ {
			c := color.NRGBA{R: 255, A: 255}
			if x >= 20 && x < 40 {
				c = color.NRGBA{G: 255, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	var err error
	switch format {
	case "png":
		err = png.Encode(&buf, img)
	case "jpg":
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100})
	case "gif":
		frame := image.NewPaletted(image.Rect(0, 0, 60, 20), []color.Color{color.Black, color.White})
		other := image.NewPaletted(frame.Bounds(), frame.Palette)
		other.SetColorIndex(1, 1, 1)
		err = gif.EncodeAll(&buf, &gif.GIF{Image: []*image.Paletted{frame, other}, Delay: []int{1, 1}, LoopCount: 0})
	case "webp":
		// A single lossless pixel; Go has a WebP decoder but no encoder.
		data, err := base64.StdEncoding.DecodeString("UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA==")
		if err != nil {
			t.Fatal(err)
		}
		return data
	default:
		t.Fatalf("unknown test format %s", format)
	}
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func testVideo(format string) []byte {
	if format == "mp4" {
		return []byte{0, 0, 0, 20, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm', 0, 0, 0, 0, 'i', 's', 'o', 'm'}
	}
	return []byte{0x1a, 0x45, 0xdf, 0xa3, 0x80}
}

func oversizedDimensions(t *testing.T, width, height uint32) []byte {
	t.Helper()
	data := testImage(t, "png")
	binary.BigEndian.PutUint32(data[16:20], width)
	binary.BigEndian.PutUint32(data[20:24], height)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	return data
}

func (a *mediaAPITest) upload(c *http.Client, path, field, filename string, data []byte, status int, message string) []byte {
	a.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile(field, filename)
	if err != nil {
		a.t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		a.t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		a.t.Fatal(err)
	}
	req, err := http.NewRequest("POST", a.srv.URL+path, &body)
	if err != nil {
		a.t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := c.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	result, err := io.ReadAll(res.Body)
	if err != nil {
		a.t.Fatal(err)
	}
	if res.StatusCode != status {
		a.t.Fatalf("upload %s: %d, want %d: %s", path, res.StatusCode, status, result)
	}
	if status == 204 && len(result) != 0 {
		a.t.Fatalf("204 body: %s", result)
	}
	if message != "" {
		var failure struct{ Error string }
		if err := json.Unmarshal(result, &failure); err != nil || failure.Error != message {
			a.t.Fatalf("upload error: %s (%v), want %q", result, err, message)
		}
	}
	return result
}

func (a *mediaAPITest) files(dir string, count int) {
	a.t.Helper()
	entries, err := os.ReadDir(filepath.Join(a.dir, dir))
	if err != nil || len(entries) != count {
		a.t.Fatalf("%s files: %v %v, want %d", dir, entries, err, count)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".upload-") {
			a.t.Fatalf("temporary upload leaked: %s", e.Name())
		}
	}
}

func TestAvatarUploadReplaceAndDelete(t *testing.T) {
	a := newMediaAPITest(t, 0)
	u := a.user("alice", false)
	c := a.login("alice")
	previous := filepath.Join(a.dir, "avatars", "old.png")
	if err := os.WriteFile(previous, testImage(t, "png"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := a.st.UpdateAvatar(context.Background(), u.ID, "old.png"); err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"png", "jpg", "gif", "webp"} {
		t.Run(format, func(t *testing.T) {
			data := a.upload(c, "/api/me/avatar", "avatar", "wrong.txt", testImage(t, format), 200, "")
			var out struct {
				User struct{ Username, AvatarURL string }
			}
			if err := json.Unmarshal(data, &out); err != nil || out.User.Username != "alice" {
				t.Fatalf("avatar response %s: %v", data, err)
			}
			name := strings.TrimPrefix(out.User.AvatarURL, "/media/avatars/")
			if !regexp.MustCompile(`^[0-9a-f]{32}\.png$`).MatchString(name) {
				t.Fatalf("avatar filename: %s", name)
			}
			path := filepath.Join(a.dir, "avatars", name)
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			img, err := png.Decode(f)
			f.Close()
			if err != nil || img.Bounds() != image.Rect(0, 0, 256, 256) {
				t.Fatalf("avatar image: %v %v", img, err)
			}
			if format == "png" || format == "jpg" {
				for _, p := range []image.Point{{0, 128}, {128, 128}, {255, 128}} {
					r, g, b, _ := img.At(p.X, p.Y).RGBA()
					if g < 60000 || r > 5000 || b > 5000 {
						t.Fatalf("center crop at %v: %d %d %d", p, r, g, b)
					}
				}
			}
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0o640 {
				t.Fatalf("avatar permissions: %v %v", info, err)
			}
			stored, err := a.st.UserByID(context.Background(), u.ID)
			if err != nil || stored.Avatar != name {
				t.Fatalf("stored avatar: %+v %v", stored, err)
			}
			if previous != "" {
				if _, err := os.Stat(previous); !os.IsNotExist(err) {
					t.Fatalf("previous avatar exists: %v", err)
				}
			}
			previous = path
			a.files("avatars", 1)
		})
	}
	var out struct{ User struct{ AvatarURL string } }
	a.call(c, "DELETE", "/api/me/avatar", nil, 200, &out)
	if out.User.AvatarURL != "/png/default_avatar.png" {
		t.Fatalf("cleared avatar: %+v", out)
	}
	stored, err := a.st.UserByID(context.Background(), u.ID)
	if err != nil || stored.Avatar != "" {
		t.Fatalf("cleared store avatar: %+v %v", stored, err)
	}
	a.files("avatars", 0)
	a.call(c, "DELETE", "/api/me/avatar", nil, 200, nil)
}

func TestAvatarRejectsInvalidUploads(t *testing.T) {
	a := newMediaAPITest(t, 0)
	a.user("alice", false)
	c := a.login("alice")
	a.upload(a.client(), "/api/me/avatar", "avatar", "x.png", testImage(t, "png"), 401, "Please log in.")
	a.error(a.client(), "DELETE", "/api/me/avatar", nil, 401, "Please log in.")
	for _, data := range [][]byte{
		[]byte("not an image"), testVideo("mp4"), testImage(t, "png")[:40],
		oversizedDimensions(t, 8001, 1), oversizedDimensions(t, 1, 8001), oversizedDimensions(t, 7000, 6000),
	} {
		a.upload(c, "/api/me/avatar", "avatar", "x.png", data, 415, "Use a PNG, JPEG, GIF or WebP image.")
	}
	a.upload(c, "/api/me/avatar", "avatar", "x.png", make([]byte, (5<<20)+1), 413, "File is too large.")
	a.upload(c, "/api/me/avatar", "wrong", "x.png", testImage(t, "png"), 400, "Invalid upload.")
	a.files("avatars", 0)
	data := make([]byte, 5<<20)
	copy(data, testImage(t, "png"))
	a.upload(c, "/api/me/avatar", "avatar", "exact.png", data, 200, "")
	a.files("avatars", 1)
}

func TestProfileAndAvatarNotifyRoom(t *testing.T) {
	a := newMediaAPITest(t, 0)
	a.user("alice", false)
	c := a.login("alice")
	conn, _ := a.join(c)
	readUpdate := func(nickname, nameColor, avatarURL string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var msg struct {
			Type string
			User struct{ Nickname, NameColor, AvatarURL string }
		}
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			t.Fatal(err)
		}
		if msg.Type != "user_updated" || msg.User.Nickname != nickname || msg.User.NameColor != nameColor || msg.User.AvatarURL != avatarURL {
			t.Fatalf("room update: %+v", msg)
		}
	}
	a.call(c, "PATCH", "/api/me", map[string]string{"nickname": "A Friend", "nameColor": "#123"}, 200, nil)
	readUpdate("A Friend", "#123", "/png/default_avatar.png")
	data := a.upload(c, "/api/me/avatar", "avatar", "x.png", testImage(t, "png"), 200, "")
	var out struct{ User struct{ AvatarURL string } }
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	readUpdate("A Friend", "#123", out.User.AvatarURL)
	a.call(c, "DELETE", "/api/me/avatar", nil, 200, nil)
	readUpdate("A Friend", "#123", "/png/default_avatar.png")
}

func TestAdminDeleteRemovesImportedAvatar(t *testing.T) {
	a := newMediaAPITest(t, 0)
	a.user("root", true)
	u := a.user("alice", false)
	name := strings.Repeat("a", 64) + ".png"
	if err := os.WriteFile(filepath.Join(a.dir, "avatars", name), testImage(t, "png"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := a.st.UpdateAvatar(context.Background(), u.ID, name); err != nil {
		t.Fatal(err)
	}
	a.call(a.login("root"), "DELETE", "/api/admin/users/alice", nil, 204, nil)
	a.files("avatars", 0)
}

func TestChatMediaPermissions(t *testing.T) {
	a := newMediaAPITest(t, 0)
	a.user("alice", false)
	c := a.login("alice")
	// Invalid bodies prove the presence/right checks happen before parsing.
	a.error(c, "POST", "/api/rooms/missing/media", nil, 404, "Unknown room.")
	a.error(c, "POST", "/api/rooms/default/media", nil, 403, "Join the room first.")
	a.join(c)
	a.error(c, "POST", "/api/rooms/default/media", nil, 403, "You are not allowed to post images.")
	anon := a.client()
	a.me(anon, "") // obtain the anonymous identity cookie before the upgrade
	a.join(anon)
	a.error(anon, "POST", "/api/rooms/default/media", nil, 403, "You are not allowed to post images.")
	a.files("chat", 0)
}

func TestChatMediaAccepted(t *testing.T) {
	a := newMediaAPITest(t, 0)
	u := a.user("alice", false)
	if err := a.st.SavePermission(context.Background(), store.Permission{Room: "default", UserID: u.ID, Image: true}); err != nil {
		t.Fatal(err)
	}
	c := a.login("alice")
	conn, _ := a.join(c)
	for i, format := range []string{"png", "jpg", "gif", "webp", "mp4", "webm"} {
		t.Run(format, func(t *testing.T) {
			typ := "image"
			var data []byte
			if format == "mp4" || format == "webm" {
				typ, data = "video", testVideo(format)
			} else {
				data = testImage(t, format)
			}
			a.upload(c, "/api/rooms/default/media", "file", "wrong.exe", data, 204, "")
			history, err := a.st.ChatHistory(context.Background(), "default")
			if err != nil || len(history) != i+1 {
				t.Fatalf("history: %v %v", history, err)
			}
			msg := history[i]
			if msg.Type != typ || !regexp.MustCompile(`^[0-9a-f]{32}\.`+format+`$`).MatchString(msg.Media) || msg.UserID == nil || *msg.UserID != u.ID {
				t.Fatalf("chat media: %+v", msg)
			}
			stored, err := os.ReadFile(filepath.Join(a.dir, "chat", msg.Media))
			if err != nil || !bytes.Equal(stored, data) {
				t.Fatalf("stored media changed: %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var pushed struct {
				Type    string
				Message struct{ Type, MediaURL string }
			}
			if err := wsjson.Read(ctx, conn, &pushed); err != nil || pushed.Type != "chat" || pushed.Message.Type != typ || pushed.Message.MediaURL != "/media/chat/"+msg.Media {
				t.Fatalf("chat broadcast: %+v %v", pushed, err)
			}
			a.files("chat", i+1)
		})
	}
}

func TestChatMediaRateLimitBeforeBody(t *testing.T) {
	a := newMediaAPITest(t, 0)
	a.user("root", true)
	c := a.login("root")
	_, key := a.join(c)
	a.upload(c, "/api/rooms/default/media", "file", "one.mp4", testVideo("mp4"), 204, "")
	// A completed upload consumes one token; it must not be charged twice.
	for i := 0; i < 9; i++ {
		if err := a.hub.Room("default").CanPostMedia(key); err != nil {
			t.Fatal(err)
		}
	}
	reading := make(chan struct{}, 1)
	req, err := http.NewRequest("POST", a.srv.URL+"/api/rooms/default/media", strings.NewReader("invalid upload"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Expect", "100-continue")
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
		Got100Continue: func() { reading <- struct{}{} },
	}))
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var failure struct{ Error string }
	if err := json.NewDecoder(res.Body).Decode(&failure); err != nil || res.StatusCode != 429 || failure.Error != "You are sending messages too fast." {
		t.Fatalf("rate limit: %d %+v %v", res.StatusCode, failure, err)
	}
	select {
	case <-reading:
		t.Fatal("rate-limited upload read its body")
	default:
	}
	a.files("chat", 1)
	history, err := a.st.ChatHistory(context.Background(), "default")
	if err != nil || len(history) != 1 {
		t.Fatalf("rate-limited upload posted: %v %v", history, err)
	}
}

func TestChatMediaRejectsInvalidAndOversized(t *testing.T) {
	a := newMediaAPITest(t, 1)
	a.user("root", true)
	c := a.login("root")
	a.join(c)
	pngData, gifData := testImage(t, "png"), testImage(t, "gif")
	for _, data := range [][]byte{
		[]byte("unsupported"), append([]byte("\x89PNG\r\n\x1a\n"), []byte("garbage")...),
		pngData[:len(pngData)/2], gifData[:len(gifData)-10],
		oversizedDimensions(t, 8001, 1), oversizedDimensions(t, 7000, 6000),
	} {
		a.upload(c, "/api/rooms/default/media", "file", "x.png", data, 415, "Unsupported file type.")
	}
	a.upload(c, "/api/rooms/default/media", "file", "x.mp4", make([]byte, (1<<20)+1), 413, "File is too large.")
	// An oversized unrelated part hits MaxBytesReader rather than the file cap.
	a.upload(c, "/api/rooms/default/media", "unrelated", "x.mp4", make([]byte, (1<<20)+(64<<10)+1), 413, "File is too large.")
	a.files("chat", 0)
	// Exactly the advertised file cap remains valid with multipart overhead.
	data := make([]byte, 1<<20)
	copy(data, testVideo("mp4"))
	a.upload(c, "/api/rooms/default/media", "file", "x.txt", data, 204, "")
	a.files("chat", 1)
}

func TestChatMediaRechecksRightsAfterUpload(t *testing.T) {
	a := newMediaAPITest(t, 0)
	u := a.user("alice", false)
	ctx := context.Background()
	if err := a.st.SavePermission(ctx, store.Permission{Room: "default", UserID: u.ID, Image: true}); err != nil {
		t.Fatal(err)
	}
	c := a.login("alice")
	a.join(c)
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	mw := multipart.NewWriter(writer)
	req, err := http.NewRequest("POST", a.srv.URL+"/api/rooms/default/media", reader)
	if err != nil {
		t.Fatal(err)
	}
	reading := make(chan struct{})
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Expect", "100-continue")
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
		Got100Continue: func() { close(reading) },
	}))
	finished := make(chan *http.Response, 1)
	failed := make(chan error, 1)
	go func() {
		res, err := c.Do(req)
		if err != nil {
			failed <- err
			return
		}
		finished <- res
	}()
	select {
	case <-reading:
	case err := <-failed:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("upload did not reach body reading")
	}
	// Revocation after the early check must still prevent posting and remove
	// the validated file that was renamed into its final location.
	if err := a.st.SavePermission(ctx, store.Permission{Room: "default", UserID: u.ID}); err != nil {
		t.Fatal(err)
	}
	a.hub.PermissionsChanged(ctx, "default", u.ID)
	part, err := mw.CreateFormFile("file", "x.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(testVideo("mp4")); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	select {
	case res := <-finished:
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		if err != nil || res.StatusCode != 403 || !strings.Contains(string(data), "You are not allowed to post images.") {
			t.Fatalf("revoked upload: %d %s %v", res.StatusCode, data, err)
		}
	case err := <-failed:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("upload did not finish")
	}
	a.files("chat", 0)
	history, err := a.st.ChatHistory(ctx, "default")
	if err != nil || len(history) != 0 {
		t.Fatalf("rejected upload was posted: %v %v", history, err)
	}
}

func TestServingMedia(t *testing.T) {
	a := newMediaAPITest(t, 0)
	c := a.client()
	for _, dir := range []string{"avatars", "chat"} {
		for _, ext := range []string{"png", "jpg", "jpeg", "gif", "webp", "mp4", "webm"} {
			name := strings.Repeat("a", 32) + "." + ext
			// The header must come from the allowlist even if content differs.
			data := []byte("<html>must not sniff</html>")
			if err := os.WriteFile(filepath.Join(a.dir, dir, name), data, 0o640); err != nil {
				t.Fatal(err)
			}
			res, err := c.Get(a.srv.URL + "/media/" + dir + "/" + name)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(res.Body)
			res.Body.Close()
			cache, contentType := "private, max-age=3600", "image/"+ext
			if dir == "avatars" {
				cache = "public, max-age=31536000, immutable"
			}
			if ext == "jpg" {
				contentType = "image/jpeg"
			} else if ext == "mp4" || ext == "webm" {
				contentType = "video/" + ext
			}
			if err != nil || res.StatusCode != 200 || !bytes.Equal(body, data) || res.Header.Get("Content-Type") != contentType || res.Header.Get("Cache-Control") != cache || res.Header.Get("X-Content-Type-Options") != "nosniff" || res.Header.Get("Content-Security-Policy") != "default-src 'none'; sandbox" {
				t.Fatalf("serve %s/%s: %d %v %q %v", dir, name, res.StatusCode, res.Header, body, err)
			}
		}
	}
	name := strings.Repeat("b", 64) + ".png"
	if err := os.WriteFile(filepath.Join(a.dir, "avatars", name), testImage(t, "png"), 0o640); err != nil {
		t.Fatal(err)
	}
	res, err := c.Get(a.srv.URL + "/media/avatars/" + name)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("imported avatar: %d", res.StatusCode)
	}
	videoName := strings.Repeat("c", 32) + ".mp4"
	video := testVideo("mp4")
	if err := os.WriteFile(filepath.Join(a.dir, "chat", videoName), video, 0o640); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest("GET", a.srv.URL+"/media/chat/"+videoName, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=4-7")
	res, err = c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil || res.StatusCode != 206 || string(data) != "ftyp" || res.Header.Get("Content-Range") != "bytes 4-7/20" || res.Header.Get("Content-Type") != "video/mp4" {
		t.Fatalf("video range: %d %v %s %v", res.StatusCode, res.Header, data, err)
	}
	for _, path := range []string{
		"/media/chat/missing.png", "/media/chat/" + strings.Repeat("d", 32) + ".png",
		"/media/chat/" + strings.Repeat("a", 31) + ".png", "/media/chat/" + strings.Repeat("a", 65) + ".png",
		"/media/chat/" + strings.Repeat("A", 32) + ".png", "/media/chat/" + strings.Repeat("a", 32) + ".svg",
		"/media/chat/%2e%2e%2favatars%2f" + name, "/media/chat/..%5cavatars%5c" + name,
		"/media/chat/../avatars/" + name, "/media/chat/../../api/me", "/media/chat//" + name,
		"/media/chat/" + name + "/extra", "/media/other/" + name,
	} {
		res, err := c.Get(a.srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 404 {
			t.Fatalf("bad media path %s: %d", path, res.StatusCode)
		}
	}
}

func TestMediaUploadConfig(t *testing.T) {
	t.Setenv("COZYCAST_NEKO_API_TOKEN", "x")
	for _, tt := range []struct {
		value string
		want  int64
	}{
		{"", 10}, {"1", 1}, {"25", 25}, {"0", 0}, {"-1", 0}, {"no", 0}, {"8796093022208", 0},
	} {
		t.Setenv("COZYCAST_MAX_UPLOAD_MB", tt.value)
		cfg, err := config.FromEnv()
		if tt.want == 0 {
			if err == nil {
				t.Fatalf("accepted invalid upload size %q", tt.value)
			}
		} else if err != nil || cfg.MaxUploadMB != tt.want {
			t.Fatalf("upload size %q: %d %v", tt.value, cfg.MaxUploadMB, err)
		}
	}
}

func TestSharedImportedAvatarCleanup(t *testing.T) {
	for _, action := range []string{"replace", "clear", "delete account"} {
		t.Run(action, func(t *testing.T) {
			a := newMediaAPITest(t, 0)
			a.user("root", true)
			alice, bob := a.user("alice", false), a.user("bob", false)
			name := strings.Repeat("a", 64) + ".png"
			path := filepath.Join(a.dir, "avatars", name)
			if err := os.WriteFile(path, testImage(t, "png"), 0o640); err != nil {
				t.Fatal(err)
			}
			for _, u := range []*store.User{alice, bob} {
				if err := a.st.UpdateAvatar(context.Background(), u.ID, name); err != nil {
					t.Fatal(err)
				}
			}
			switch action {
			case "replace":
				a.upload(a.login("alice"), "/api/me/avatar", "avatar", "new.png", testImage(t, "png"), 200, "")
			case "clear":
				a.call(a.login("alice"), "DELETE", "/api/me/avatar", nil, 200, nil)
			case "delete account":
				a.call(a.login("root"), "DELETE", "/api/admin/users/alice", nil, 204, nil)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("shared avatar removed: %v", err)
			}
			// Removing the last reference finally removes the imported file.
			a.call(a.login("bob"), "DELETE", "/api/me/avatar", nil, 200, nil)
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("unreferenced avatar retained: %v", err)
			}
		})
	}
}
