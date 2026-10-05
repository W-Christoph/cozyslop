package hub

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"cozycast/internal/store"
)

func TestFileActions(t *testing.T) {
	var mu sync.Mutex
	var asked []string
	status := http.StatusNoContent
	desktop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		asked = append(asked, r.Method+" "+r.URL.Path+" "+r.URL.Query().Get("name")+" "+r.Header.Get("Authorization"))
		w.WriteHeader(status)
	}))
	defer desktop.Close()
	calls := func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), asked...) }
	result := func(rec *recording, n int) fileResultMsg {
		t.Helper()
		rec.wait(t, "file_result")
		var all []fileResultMsg
		for _, m := range rec.snapshot() {
			if r, ok := m.(fileResultMsg); ok {
				all = append(all, r)
			}
		}
		requireEqual(t, len(all), n)
		return all[n-1]
	}

	f := newFixture(t, store.RoomSettings{RemoteOwnership: true})
	f.r.playURL, f.r.playToken = desktop.URL+"/play", "secret"
	f.fake.AddFile("a b&c.mp4")
	f.fake.AddFile("kept.mp4")
	guest, g := f.join(anon("guest"))
	uploader := f.user("uploader")
	f.permission(uploader, store.Permission{Upload: true})
	up, u := f.join(uploader)
	both := f.user("both")
	f.permission(both, store.Permission{Upload: true, Remote: true})
	bo, b := f.join(both)

	// Neither without the upload right; playing needs the remote right too.
	f.r.Handle(f.ctx, guest, ClientMsg{Type: "file_delete", Name: "kept.mp4"})
	requireEqual(t, result(g, 1), fileResultMsg{Type: "file_result", Action: "delete", Name: "kept.mp4", Error: "You are not allowed to do that."})
	requireEqual(t, f.fake.HasFile("kept.mp4"), true)
	f.r.Handle(f.ctx, up, ClientMsg{Type: "file_play", Name: "a.mp4"})
	requireEqual(t, result(u, 1).Error, "You are not allowed to do that.")
	requireEqual(t, len(calls()), 0)

	// Deleting is neko's own, as its admin; playing is the helper's.
	f.r.Handle(f.ctx, up, ClientMsg{Type: "file_delete", Name: "a b&c.mp4"})
	requireEqual(t, result(u, 2), fileResultMsg{Type: "file_result", Action: "delete", Name: "a b&c.mp4"})
	requireEqual(t, f.fake.HasFile("a b&c.mp4"), false)
	f.r.Handle(f.ctx, bo, ClientMsg{Type: "file_play", Name: "movie.mkv"})
	requireEqual(t, result(b, 1), fileResultMsg{Type: "file_result", Action: "play", Name: "movie.mkv"})
	got := calls()
	requireEqual(t, len(got), 1)
	requireEqual(t, got[0], "POST /play movie.mkv Bearer secret")
	// Only the tab that asked hears about it.
	requireEqual(t, g.count("file_result"), 1)

	// Names that are not a file of the folder never reach the desktop.
	for _, name := range []string{"", "../etc/passwd", "a/b"} {
		f.r.Handle(f.ctx, up, ClientMsg{Type: "file_delete", Name: name})
	}
	requireEqual(t, result(u, 5).Error, "That file is not in Downloads.")
	f.r.Handle(f.ctx, up, ClientMsg{Type: "file_delete", Name: "gone.mp4"})
	requireEqual(t, result(u, 6).Error, "That file is not in Downloads any more.")
	requireEqual(t, len(calls()), 1)

	// With remote ownership, a remote someone else holds is not played over.
	f.r.setHost(up.ID)
	f.r.Handle(f.ctx, bo, ClientMsg{Type: "file_play", Name: "movie.mkv"})
	requireEqual(t, result(b, 2).Error, "uploader nick owns the remote.")
	f.r.setHost(bo.ID)
	f.r.Handle(f.ctx, bo, ClientMsg{Type: "file_play", Name: "movie.mkv"})
	requireEqual(t, result(b, 3).Error, "")
	requireEqual(t, len(calls()), 2)

	mu.Lock()
	status = http.StatusNotFound
	mu.Unlock()
	f.r.Handle(f.ctx, bo, ClientMsg{Type: "file_play", Name: "gone.mp4"})
	requireEqual(t, result(b, 4).Error, "That file is not in Downloads any more.")

	// A desktop without the helper, or one that does not answer.
	desktop.Close()
	f.r.Handle(f.ctx, bo, ClientMsg{Type: "file_play", Name: "movie.mkv"})
	requireEqual(t, result(b, 5).Error, "The room's desktop did not answer. It may need a newer room image.")
	f.r.playURL = ""
	f.r.Handle(f.ctx, bo, ClientMsg{Type: "file_play", Name: "movie.mkv"})
	requireEqual(t, result(b, 6).Error, "This room's desktop cannot play files.")
}
