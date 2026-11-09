package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"cozycast/internal/anilist"
	"cozycast/internal/store"
)

// fakeDesktop is the play helper's /status and AniList, both programmable.
type fakeDesktop struct {
	mu       sync.Mutex
	status   playStatus
	searches int
}

func (d *fakeDesktop) set(st playStatus) { d.mu.Lock(); d.status = st; d.mu.Unlock() }

func (d *fakeDesktop) searched() int { d.mu.Lock(); defer d.mu.Unlock(); return d.searches }

func newWatchFixture(t *testing.T) (*fixture, *fakeDesktop) {
	t.Helper()
	d := &fakeDesktop{}
	helper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		d.mu.Lock()
		defer d.mu.Unlock()
		json.NewEncoder(w).Encode(d.status)
	}))
	t.Cleanup(helper.Close)
	ani := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if strings.Contains(req.Query, "Page") {
			d.mu.Lock()
			d.searches++
			d.mu.Unlock()
			w.Write([]byte(`{"data":{"Page":{"media":[{"id":1,"title":{"romaji":"Sousou no Frieren"},"episodes":28,"coverImage":{"medium":"f.jpg"}}]}}}`))
			return
		}
		if req.Variables["id"] == float64(2) {
			w.Write([]byte(`{"data":{"Media":{"id":2,"title":{"romaji":"Frieren 2"},"episodes":null,"coverImage":{"medium":""}}}}`))
			return
		}
		w.Write([]byte(`{"errors":[{"message":"Not Found.","status":404}]}`))
	}))
	t.Cleanup(ani.Close)

	f := newFixture(t, store.RoomSettings{})
	f.r.statusURL, f.r.playToken = helper.URL+"/status", "secret"
	f.h.AniList = anilist.New()
	f.h.AniList.URL = ani.URL
	f.h.SetWatchlog(true)
	return f, d
}

// play has VLC play file from start to end (100 seconds), polling at every
// part; during(t) runs before each poll.
func (f *fixture) play(d *fakeDesktop, file string, during func(second int)) {
	for s := 0; s < 100; s += watchBucket {
		if during != nil {
			during(s)
		}
		d.set(playStatus{State: "playing", Time: s, Length: 100, File: file})
		f.r.checkPlayback(f.ctx)
	}
	d.set(playStatus{State: "stopped"})
	f.r.checkPlayback(f.ctx)
}

func (f *fixture) progress() map[string]int {
	f.t.Helper()
	list, err := f.s.ListProgress(f.ctx)
	requireOK(f.t, err)
	out := map[string]int{}
	for _, p := range list {
		out[p.Username] = p.Episode
	}
	return out
}

func lastProposal(t *testing.T, rec *recording) *WatchProposal {
	t.Helper()
	return rec.wait(t, "watch").(watchMsg).Proposal
}

func TestWatchSuggestsAndSaves(t *testing.T) {
	f, d := newWatchFixture(t)
	alice, a := f.join(f.user("alice"))
	bob, _ := f.join(f.user("bob"))
	guest, g := f.join(anon("guest"))

	// Bob looks away for the second half; the anonymous guest is not tracked.
	const file = "[SubsPlease] Sousou no Frieren - 05 (1080p) [ABCD1234].mkv"
	f.play(d, file, func(s int) {
		if s == 50 {
			f.r.Handle(f.ctx, bob, ClientMsg{Type: "activity", Active: false})
		}
	})
	playing := a.wait(t, "playing").(playingMsg).Playing
	requireEqual(t, *playing, WatchPlaying{AnimeID: 1, Title: "Sousou no Frieren", Episode: 5})
	requireEqual(t, a.wait(t, "playing").(playingMsg).Playing == nil, true)

	p := lastProposal(t, a)
	requireEqual(t, p.AnimeID, int64(1))
	requireEqual(t, p.Episode, 5)
	requireEqual(t, *p.Episodes, 28)
	requireEqual(t, len(p.Viewers), 2)
	requireEqual(t, p.Viewers[0], WatchViewer{Key: alice.m.key, Nickname: "alice nick", Coverage: 1, Checked: true})
	requireEqual(t, p.Viewers[1], WatchViewer{Key: bob.m.key, Nickname: "bob nick", Coverage: 0.5, Checked: false})
	if p.ApplyAt < time.Now().Add(watchSaveAfter-time.Minute).UnixMilli() {
		t.Fatalf("applyAt %d", p.ApplyAt)
	}

	// Only accounts may change it.
	f.r.Handle(f.ctx, guest, ClientMsg{Type: "watch_check", ID: p.ID, Key: bob.m.key, Checked: true})
	requireEqual(t, g.wait(t, "error").(errorMsg).Message, "You are not allowed to do that.")

	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_check", ID: p.ID, Key: bob.m.key, Checked: true})
	requireEqual(t, lastProposal(t, a).Viewers[1].Checked, true)
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_episode", ID: p.ID, Episode: 29})
	requireEqual(t, a.wait(t, "error").(errorMsg).Message, "The show has no such episode.")
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_episode", ID: p.ID, Episode: 6})
	requireEqual(t, lastProposal(t, a).Episode, 6)
	// Another show, from AniList.
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_anime", ID: p.ID, AnimeID: 2})
	changed := lastProposal(t, a)
	requireEqual(t, changed.AnimeID, int64(2))
	requireEqual(t, changed.Title, "Frieren 2")
	requireEqual(t, changed.Episodes == nil, true)
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_anime", ID: p.ID, AnimeID: 3})
	requireEqual(t, a.wait(t, "error").(errorMsg).Message, "AniList has no such anime.")

	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_save", ID: p.ID})
	requireEqual(t, lastProposal(t, a) == nil, true)
	requireEqual(t, f.progress()["alice"], 6)
	requireEqual(t, f.progress()["bob"], 6)
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_save", ID: p.ID})
	requireEqual(t, a.wait(t, "error").(errorMsg).Message, "That suggestion is gone.")

	// The file's title is now known as the show picked: AniList is not
	// asked again. Saved by itself after the wait, never lowering progress.
	searches := d.searched()
	watchSaveAfter = 50 * time.Millisecond
	t.Cleanup(func() { watchSaveAfter = 2 * time.Minute })
	f.r.Handle(f.ctx, bob, ClientMsg{Type: "activity", Active: true})
	f.play(d, "[SubsPlease] Sousou no Frieren - 03 (1080p).mkv", nil)
	requireEqual(t, d.searched(), searches)
	p = lastProposal(t, a)
	requireEqual(t, p.AnimeID, int64(2))
	requireEqual(t, len(p.Viewers), 2)
	requireEqual(t, lastProposal(t, a) == nil, true) // saved
	requireEqual(t, f.progress()["alice"], 6)
}

func TestWatchIgnoresWhatNobodySaw(t *testing.T) {
	f, d := newWatchFixture(t)
	_, a := f.join(f.user("alice"))

	// Skipped through: a few parts only.
	for _, s := range []int{0, 90} {
		d.set(playStatus{State: "playing", Time: s, Length: 100, File: "Show - 01.mkv"})
		f.r.checkPlayback(f.ctx)
	}
	d.set(playStatus{State: "stopped"})
	f.r.checkPlayback(f.ctx)
	// Not an episode.
	f.play(d, "holiday.mp4", nil)
	requireEqual(t, a.count("watch"), 0)
	requireEqual(t, a.count("playing"), 2) // Show 01 started and ended

	// Turned off, the open suggestion and the episode are forgotten.
	f.play(d, "Show - 02.mkv", nil)
	requireEqual(t, lastProposal(t, a) != nil, true)
	d.set(playStatus{State: "playing", Time: 0, Length: 100, File: "Show - 03.mkv"})
	f.r.checkPlayback(f.ctx)
	f.h.SetWatchlog(false)
	f.r.checkPlayback(f.ctx)
	requireEqual(t, lastProposal(t, a) == nil, true)
	f.r.mu.Lock()
	requireEqual(t, f.r.watch == nil && f.r.proposal == nil, true)
	f.r.mu.Unlock()
	requireEqual(t, len(f.progress()), 0)
}
