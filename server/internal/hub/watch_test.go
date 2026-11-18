package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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
			search, _ := req.Variables["q"].(string)
			// As AniList does: nothing for "im", which no title has.
			if strings.Contains(search, "Podcast") || strings.HasPrefix(search, "im ") {
				w.Write([]byte(`{"data":{"Page":{"media":[]}}}`))
				return
			}
			if strings.Contains(search, "summoner") {
				w.Write([]byte(`{"data":{"Page":{"media":[{"id":8,"title":{"romaji":"Summoner Shorts","english":"I’m Dating a Dark Summoner Shorts"}},{"id":7,"title":{"romaji":"Dark Summoner to Dekiteiru","english":"I'm Dating a Dark Summoner"},"episodes":12,"coverImage":{"medium":""}}]}}}`))
				return
			}
			w.Write([]byte(`{"data":{"Page":{"media":[{"id":1,"title":{"romaji":"Sousou no Frieren"},"episodes":28,"coverImage":{"medium":"f.jpg"}}]}}}`))
			return
		}
		if req.Variables["id"] == float64(2) {
			w.Write([]byte(`{"data":{"Media":{"id":2,"title":{"romaji":"Frieren 2"},"episodes":null,"coverImage":{"medium":""}}}}`))
			return
		}
		if req.Variables["id"] == float64(4) {
			w.Write([]byte(`{"data":{"Media":{"id":4,"title":{"romaji":"Frieren Mini"},"episodes":3,"coverImage":{"medium":""}}}}`))
			return
		}
		w.Write([]byte(`{"errors":[{"message":"Not Found.","status":404}]}`))
	}))
	t.Cleanup(ani.Close)

	f := newFixture(t, store.RoomSettings{})
	f.r.statusURL, f.r.playToken = helper.URL+"/status", "secret"
	f.h.AniList = anilist.New()
	f.h.AniList.URL = ani.URL
	wl, err := store.OpenWatchlog(f.ctx, ":memory:")
	requireOK(t, err)
	t.Cleanup(func() { wl.Close() })
	f.h.Watchlog = wl
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

// progress is the episodes each account has watched, as "3 6" or "1-4".
func (f *fixture) progress() map[string]string {
	f.t.Helper()
	list, err := f.h.Watchlog.ListProgress(f.ctx)
	requireOK(f.t, err)
	out := map[string]string{}
	for _, p := range list {
		u, err := f.s.UserByID(f.ctx, p.UserID)
		requireOK(f.t, err)
		var parts []string
		for _, r := range p.Watched {
			if r[0] == r[1] {
				parts = append(parts, strconv.Itoa(r[0]))
			} else {
				parts = append(parts, strconv.Itoa(r[0])+"-"+strconv.Itoa(r[1]))
			}
		}
		out[u.Username] = strings.Join(parts, " ")
	}
	return out
}

func lastProposal(t *testing.T, rec *recording) *WatchProposal {
	t.Helper()
	return rec.wait(t, "watch").(watchMsg).Proposal
}

// unconsumed is what rec was sent of a type and not yet waited for.
func unconsumed(rec *recording, typ string) []any {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	var out []any
	for i, m := range rec.messages {
		if !rec.consumed[i] && messageType(m) == typ {
			out = append(out, m)
		}
	}
	return out
}

// newestProposal is the last of the suggestions rec was sent, passing over
// those before.
func newestProposal(t *testing.T, rec *recording) *WatchProposal {
	t.Helper()
	p := lastProposal(t, rec)
	for len(unconsumed(rec, "watch")) > 0 {
		p = lastProposal(t, rec)
	}
	return p
}

func TestWatchSuggestsAndSaves(t *testing.T) {
	f, d := newWatchFixture(t)
	alice, a := f.join(f.user("alice"))
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watching", Watching: true})
	bob, b := f.join(f.user("bob"))
	f.r.Handle(f.ctx, bob, ClientMsg{Type: "watching", Watching: true})
	carol, c := f.join(f.user("carol"))
	guest, g := f.join(anon("guest"))
	admin, ad := f.join(f.flags(f.user("root"), true, false))

	// Bob pauses the stream for the second half, carol sees only its start;
	// the anonymous guest is not tracked.
	const file = "[SubsPlease] Sousou no Frieren - 05 (1080p) [ABCD1234].mkv"
	f.play(d, file, func(s int) {
		f.r.Handle(f.ctx, carol, ClientMsg{Type: "watching", Watching: s < 10})
		if s == 50 {
			f.r.Handle(f.ctx, bob, ClientMsg{Type: "watching", Watching: false})
		}
	})
	playing := a.wait(t, "playing").(playingMsg).Playing
	requireEqual(t, *playing, WatchPlaying{AnimeID: 1, Title: "Sousou no Frieren", Episode: 5})
	requireEqual(t, a.wait(t, "playing").(playingMsg).Playing == nil, true)
	requireEqual(t, g.wait(t, "playing").(playingMsg).Playing != nil, true)

	// Each is asked about themselves alone: yes unless they say no from
	// 60% on, no unless they say yes from 20%. Who saw less is not asked,
	// and nobody, admins too, sees the others.
	p := lastProposal(t, a)
	requireEqual(t, p.AnimeID, int64(1))
	requireEqual(t, p.Episode, 5)
	requireEqual(t, *p.Episodes, 28)
	requireEqual(t, len(p.Viewers), 1)
	requireEqual(t, p.Viewers[0], WatchViewer{Key: alice.m.key, Nickname: "alice nick", Coverage: 1, Checked: true})
	pb := lastProposal(t, b)
	requireEqual(t, len(pb.Viewers), 1)
	requireEqual(t, pb.Viewers[0], WatchViewer{Key: bob.m.key, Nickname: "bob nick", Coverage: 0.5, Checked: false})
	requireEqual(t, lastProposal(t, c) == nil, true)
	requireEqual(t, lastProposal(t, g) == nil, true)
	requireEqual(t, lastProposal(t, ad) == nil, true)
	if p.ApplyAt < time.Now().Add(watchSaveAfter-time.Minute).UnixMilli() {
		t.Fatalf("applyAt %d", p.ApplyAt)
	}

	// Only those asked may change it.
	for _, other := range []struct {
		c   *Client
		rec *recording
	}{{guest, g}, {carol, c}, {admin, ad}} {
		other.c.m.rights.Admin = other.c == admin // an admin is no exception
		f.r.Handle(f.ctx, other.c, ClientMsg{Type: "watch_answer", ID: p.ID, Checked: true})
		requireEqual(t, other.rec.wait(t, "error").(errorMsg).Message, "You are not asked about that episode.")
		f.r.Handle(f.ctx, other.c, ClientMsg{Type: "watch_episode", ID: p.ID, Episode: 6})
		requireEqual(t, other.rec.wait(t, "error").(errorMsg).Message, "You are not asked about that episode.")
	}
	f.r.Handle(f.ctx, admin, ClientMsg{Type: "watch_save", ID: p.ID})
	requireEqual(t, ad.wait(t, "error").(errorMsg).Message, "Unknown message type.")

	f.r.Handle(f.ctx, bob, ClientMsg{Type: "watch_episode", ID: p.ID, Episode: 29})
	requireEqual(t, b.wait(t, "error").(errorMsg).Message, "The show has no such episode.")
	f.r.Handle(f.ctx, bob, ClientMsg{Type: "watch_episode", ID: p.ID, Episode: 6})
	requireEqual(t, lastProposal(t, a).Episode, 6)
	// A show without episode 6: not saved by itself, and nobody can say yes.
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_anime", ID: p.ID, AnimeID: 4})
	requireEqual(t, lastProposal(t, a).ApplyAt, int64(0))
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_answer", ID: p.ID, Checked: true})
	requireEqual(t, a.wait(t, "error").(errorMsg).Message, "Pick the show first.")
	// Another show, from AniList.
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_anime", ID: p.ID, AnimeID: 2})
	changed := lastProposal(t, a)
	requireEqual(t, changed.AnimeID, int64(2))
	requireEqual(t, changed.Title, "Frieren 2")
	requireEqual(t, changed.Episodes == nil, true)
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_anime", ID: p.ID, AnimeID: 3})
	requireEqual(t, a.wait(t, "error").(errorMsg).Message, "AniList has no such anime.")

	// Bob says yes, alice no: with everyone's answer it is saved.
	f.r.Handle(f.ctx, bob, ClientMsg{Type: "watch_answer", ID: p.ID, Checked: true})
	answered := newestProposal(t, b).Viewers[0]
	requireEqual(t, answered.Checked && answered.Answered, true)
	requireEqual(t, lastProposal(t, a).Viewers[0].Answered, false)
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_answer", ID: p.ID, Checked: false})
	requireEqual(t, lastProposal(t, a) == nil, true)
	requireEqual(t, f.progress()["alice"], "")
	requireEqual(t, f.progress()["bob"], "6")
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_answer", ID: p.ID, Checked: true})
	requireEqual(t, a.wait(t, "error").(errorMsg).Message, "That suggestion is gone.")

	// The file's title is now known as the show picked: AniList is not
	// asked again. Saved by itself after the wait, as suggested, never
	// lowering progress.
	searches := d.searched()
	watchSaveAfter = 50 * time.Millisecond
	t.Cleanup(func() { watchSaveAfter = 2 * time.Minute })
	f.r.Handle(f.ctx, bob, ClientMsg{Type: "watching", Watching: true})
	f.play(d, "[SubsPlease] Sousou no Frieren - 03 (1080p).mkv", nil)
	requireEqual(t, d.searched(), searches)
	p = lastProposal(t, a)
	requireEqual(t, p.AnimeID, int64(2))
	requireEqual(t, lastProposal(t, a) == nil, true) // being saved
	for i := 0; i < 100 && f.progress()["alice"] == ""; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	requireEqual(t, f.progress()["alice"], "3")
	requireEqual(t, f.progress()["bob"], "3 6")
}

// An account that left the watchlog is not followed, not told what plays
// and not asked; leaving takes it out of what is open.
func TestWatchLeavesOutWhoLeft(t *testing.T) {
	f, d := newWatchFixture(t)
	alice, a := f.join(f.user("alice"))
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watching", Watching: true})
	bob, b := f.join(f.user("bob"))
	f.r.Handle(f.ctx, bob, ClientMsg{Type: "watching", Watching: true})
	leave := func(c *Client, off bool) {
		requireOK(t, f.s.SetWatchlogOff(f.ctx, c.m.user.ID, off))
		f.h.UserChanged(f.ctx, c.m.user.ID)
	}

	leave(bob, true)
	requireEqual(t, b.wait(t, "user_updated").(userMsg).User.WatchlogOff, true)
	f.play(d, "Show - 01.mkv", nil)
	requireEqual(t, a.count("playing"), 2)
	requireEqual(t, b.count("playing"), 3) // the one for leaving, and nothing twice
	for range 3 {
		requireEqual(t, b.wait(t, "playing").(playingMsg).Playing == nil, true)
	}
	requireEqual(t, len(lastProposal(t, a).Viewers), 1)
	requireEqual(t, newestProposal(t, b) == nil, true)
	f.r.mu.Lock()
	requireEqual(t, f.r.proposal.viewer(bob.m.key), -1)
	f.r.mu.Unlock()
	f.r.Handle(f.ctx, bob, ClientMsg{Type: "watch_link", AnimeID: 1})
	requireEqual(t, b.wait(t, "error").(errorMsg).Message, "You are not allowed to do that.")

	// Back in, and asked; leaving again, the suggestion goes with the last
	// one asked.
	leave(bob, false)
	requireEqual(t, b.wait(t, "playing").(playingMsg).Playing == nil, true)
	f.play(d, "Show - 02.mkv", nil)
	requireEqual(t, b.wait(t, "playing").(playingMsg).Playing.Episode, 2)
	requireEqual(t, len(newestProposal(t, b).Viewers), 1)
	leave(alice, true)
	requireEqual(t, newestProposal(t, a) == nil, true)
	requireEqual(t, len(unconsumed(b, "watch")), 0) // nothing changes for bob
	leave(bob, true)
	requireEqual(t, newestProposal(t, b) == nil, true)
	f.r.mu.Lock()
	requireEqual(t, f.r.proposal == nil, true)
	f.r.mu.Unlock()
}

func TestWatchIgnoresWhatNobodySaw(t *testing.T) {
	f, d := newWatchFixture(t)
	alice, a := f.join(f.user("alice"))

	// In the room with the stream not running: not watching.
	f.play(d, "Show - 09.mkv", nil)
	requireEqual(t, a.count("watch"), 0)
	requireEqual(t, a.count("playing"), 2)
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watching", Watching: true})

	// Skipped through: one part only.
	for _, s := range []int{0} {
		d.set(playStatus{State: "playing", Time: s, Length: 100, File: "Show - 01.mkv"})
		f.r.checkPlayback(f.ctx)
	}
	d.set(playStatus{State: "stopped"})
	f.r.checkPlayback(f.ctx)
	// Not an episode.
	f.play(d, "holiday.mp4", nil)
	requireEqual(t, a.count("watch"), 0)
	requireEqual(t, a.count("playing"), 4) // Show 01 started and ended, too

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

func TestWatchFollowsFirefox(t *testing.T) {
	f, d := newWatchFixture(t)
	alice, a := f.join(f.user("alice"))
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watching", Watching: true})

	// A video with an episode but no show AniList knows is followed, for
	// someone to say which show it is; it suggests nothing by itself.
	page := playStatus{State: "playing", Source: "firefox", Length: 100, Title: "Some Podcast Episode 5 - Site", URL: "https://site.example/watch?v=abc"}
	for page.Time = 0; page.Time < 100; page.Time += watchBucket {
		d.set(page)
		f.r.checkPlayback(f.ctx)
	}
	requireEqual(t, *a.wait(t, "playing").(playingMsg).Playing, WatchPlaying{Title: "Some Podcast", Episode: 5})

	// The show and the episode are in the address; the title, the page's
	// to change, does not start the episode over.
	page.URL = "https://site.example/watch/AbCdEf_123/sousou-no-frieren?ep=5"
	for page.Time = 0; page.Time < 100; page.Time += watchBucket {
		page.Title = "An Episode's Title"
		if page.Time >= 50 {
			page.Title = "(1) An Episode's Title"
		}
		d.set(page)
		f.r.checkPlayback(f.ctx)
	}
	requireEqual(t, a.count("watch"), 0) // the podcast was no show's episode
	requireEqual(t, a.wait(t, "playing").(playingMsg).Playing == nil, true)
	requireEqual(t, *a.wait(t, "playing").(playingMsg).Playing, WatchPlaying{AnimeID: 1, Title: "Sousou no Frieren", Episode: 5})
	// Paused in the middle it waits; paused at its end it is over.
	page.State, page.Time = "paused", 50
	d.set(page)
	f.r.checkPlayback(f.ctx)
	requireEqual(t, a.count("watch"), 0)
	page.Time = 95
	d.set(page)
	f.r.checkPlayback(f.ctx)
	p := lastProposal(t, a)
	requireEqual(t, p.Episode, 5)
	requireEqual(t, p.File, page.URL)
	requireEqual(t, p.Viewers[0], WatchViewer{Key: alice.m.key, Nickname: "alice nick", Coverage: 1, Checked: true})
	f.r.checkPlayback(f.ctx) // still paused there: not another one
	requireEqual(t, a.count("watch"), 1)

	// Saved, the address's title is known for the show: the next episode
	// needs no AniList. Alice is the only one asked: her answer saves it.
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_answer", ID: p.ID, Checked: true})
	searches := d.searched()
	page.State, page.Time, page.URL = "playing", 0, "https://site.example/watch/AbCdEf_123/sousou-no-frieren?ep=6"
	d.set(page)
	f.r.checkPlayback(f.ctx)
	requireEqual(t, d.searched(), searches)
	requireEqual(t, f.progress()["alice"], "5")
}

func TestWatchFindsShowSpelledWithoutSigns(t *testing.T) {
	f, d := newWatchFixture(t)
	alice, a := f.join(f.user("alice"))
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watching", Watching: true})

	// The address has "im" for "I'm": AniList knows the title's last words.
	d.set(playStatus{State: "playing", Source: "firefox", Length: 100, Title: "An Episode's Title",
		URL: "https://site.example/watch/AbCdEf_123/im-dating-a-dark-summoner?ep=6"})
	f.r.checkPlayback(f.ctx)
	requireEqual(t, *a.wait(t, "playing").(playingMsg).Playing, WatchPlaying{AnimeID: 7, Title: "Dark Summoner to Dekiteiru", Episode: 6})

	// Only a show with the same letters counts: not whatever else is found.
	d.set(playStatus{State: "playing", Source: "firefox", Length: 100, Title: "Another",
		URL: "https://site.example/watch/AbCdEf_123/im-a-summoner-fan?ep=2"})
	searches := d.searched()
	f.r.checkPlayback(f.ctx)
	requireEqual(t, f.r.watch.anime == nil, true)
	requireEqual(t, d.searched(), searches+4)
}

func TestWatchLinksWhatPlaysToAShow(t *testing.T) {
	f, d := newWatchFixture(t)
	alice, a := f.join(f.user("alice"))
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watching", Watching: true})
	guest, g := f.join(anon("guest"))

	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_link", AnimeID: 2})
	requireEqual(t, a.wait(t, "error").(errorMsg).Message, "No episode is playing.")

	// Not recognised: alice says which show it is. Guests may not.
	page := playStatus{State: "playing", Source: "firefox", Length: 100, Title: "Some Podcast Episode 5 - Site", URL: "https://site.example/watch?v=abc"}
	d.set(page)
	f.r.checkPlayback(f.ctx)
	requireEqual(t, a.wait(t, "playing").(playingMsg).Playing.AnimeID, int64(0))
	f.r.Handle(f.ctx, guest, ClientMsg{Type: "watch_link", AnimeID: 2})
	requireEqual(t, g.wait(t, "error").(errorMsg).Message, "You are not allowed to do that.")
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_link", AnimeID: 2})
	requireEqual(t, *a.wait(t, "playing").(playingMsg).Playing, WatchPlaying{AnimeID: 2, Title: "Frieren 2", Episode: 5, Linked: true})

	// What was seen before counts, and the episode is suggested for the show.
	for page.Time = 0; page.Time < 100; page.Time += watchBucket {
		d.set(page)
		f.r.checkPlayback(f.ctx)
	}
	page.State = "stopped"
	d.set(page)
	f.r.checkPlayback(f.ctx)
	p := lastProposal(t, a)
	requireEqual(t, p.AnimeID, int64(2))
	requireEqual(t, p.Viewers[0].Checked, true)

	// The name is the show's from now on: the next episode needs nobody,
	// and no AniList.
	searches := d.searched()
	page.State, page.Time, page.Title = "playing", 0, "Some Podcast Episode 6 - Site"
	d.set(page)
	f.r.checkPlayback(f.ctx)
	requireEqual(t, a.wait(t, "playing").(playingMsg).Playing == nil, true)
	requireEqual(t, *a.wait(t, "playing").(playingMsg).Playing, WatchPlaying{AnimeID: 2, Title: "Frieren 2", Episode: 6, Linked: true})
	requireEqual(t, d.searched(), searches)
}
