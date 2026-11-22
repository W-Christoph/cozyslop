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
	opened   []string // the pages /open was asked to open
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
		if r.URL.Path == "/open" && r.Method == http.MethodPost {
			d.opened = append(d.opened, r.URL.Query().Get("url"))
			w.WriteHeader(http.StatusNoContent)
			return
		}
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
	f.r.statusURL, f.r.openURL, f.r.playToken = helper.URL+"/status", helper.URL+"/open", "secret"
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
	requireEqual(t, p.Viewers[0], WatchViewer{Key: alice.m.key, Nickname: "alice nick", Coverage: 1, Checked: true, Earlier: 4})
	pb := lastProposal(t, b)
	requireEqual(t, len(pb.Viewers), 1)
	requireEqual(t, pb.Viewers[0], WatchViewer{Key: bob.m.key, Nickname: "bob nick", Coverage: 0.5, Checked: false, Earlier: 4})
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
	requireEqual(t, p.Viewers[0], WatchViewer{Key: alice.m.key, Nickname: "alice nick", Coverage: 1, Checked: true, Earlier: 4})
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

func TestWatchMarksEarlierEpisodes(t *testing.T) {
	f, d := newWatchFixture(t)
	alice, a := f.join(f.user("alice"))
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watching", Watching: true})
	bob, b := f.join(f.user("bob"))
	f.r.Handle(f.ctx, bob, ClientMsg{Type: "watching", Watching: true})
	// Bob marked 2 and 3 before; alice nothing.
	twentyEight := 28
	requireOK(t, f.h.Watchlog.SaveAnime(f.ctx, &store.Anime{ID: 1, Title: "Sousou no Frieren", Episodes: &twentyEight}))
	requireOK(t, f.h.Watchlog.SetWatched(f.ctx, bob.m.user.ID, 1, 2, 3, true, "manual"))

	f.play(d, "[SubsPlease] Sousou no Frieren - 05 (1080p).mkv", nil)
	p := lastProposal(t, a)
	requireEqual(t, p.Viewers[0].Earlier, 4)
	requireEqual(t, lastProposal(t, b).Viewers[0].Earlier, 2)
	// Another episode counts again.
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_episode", ID: p.ID, Episode: 6})
	requireEqual(t, newestProposal(t, a).Viewers[0].Earlier, 5)
	requireEqual(t, newestProposal(t, b).Viewers[0].Earlier, 3)

	// Asking for them marks nothing by itself, and can be taken back.
	f.r.Handle(f.ctx, bob, ClientMsg{Type: "watch_earlier", ID: p.ID, Earlier: true})
	requireEqual(t, newestProposal(t, b).Viewers[0].EarlierToo, true)
	f.r.Handle(f.ctx, bob, ClientMsg{Type: "watch_earlier", ID: p.ID, Earlier: false})
	requireEqual(t, newestProposal(t, b).Viewers[0].EarlierToo, false)
	// Alice saw them elsewhere: they are marked with her yes. Bob says yes
	// to this one only.
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_earlier", ID: p.ID, Earlier: true})
	requireEqual(t, newestProposal(t, a).Viewers[0].EarlierToo, true)
	requireEqual(t, f.progress()["alice"], "")
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_answer", ID: p.ID, Checked: true})
	f.r.Handle(f.ctx, bob, ClientMsg{Type: "watch_answer", ID: p.ID, Checked: true})
	requireEqual(t, f.progress()["alice"], "1-6")
	requireEqual(t, f.progress()["bob"], "2-3 6")
}

func TestWatchOpensEpisodeWhereItPlayed(t *testing.T) {
	f, d := newWatchFixture(t)
	viewer := f.user("alice")
	f.permission(viewer, store.Permission{Remote: true})
	alice, a := f.join(viewer)
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watching", Watching: true})
	bob, b := f.join(f.user("bob"))

	// AniList's guess learns nothing until alice says yes.
	page := playStatus{State: "playing", Source: "firefox", Length: 100, Title: "Some title", URL: "https://site.example/sousou-no-frieren-episode-5"}
	for page.Time = 0; page.Time < 100; page.Time += watchBucket {
		d.set(page)
		f.r.checkPlayback(f.ctx)
	}
	page.State = "stopped"
	d.set(page)
	f.r.checkPlayback(f.ctx)
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_open", AnimeID: 1, Episode: 6})
	requireEqual(t, a.wait(t, "error").(errorMsg).Message, "That show is not on the watchlog.")
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_answer", ID: lastProposal(t, a).ID, Checked: true})
	got, err := f.h.Watchlog.Anime(f.ctx, 1)
	requireOK(t, err)
	requireEqual(t, got.WatchURL, "https://site.example/sousou-no-frieren-episode-{ep}")

	// The next episode opens there, for who may use the remote.
	f.r.Handle(f.ctx, bob, ClientMsg{Type: "watch_open", AnimeID: 1, Episode: 6})
	requireEqual(t, b.wait(t, "error").(errorMsg).Message, "You are not allowed to do that.")
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_open", AnimeID: 1, Episode: 6})
	opened := func() string { d.mu.Lock(); defer d.mu.Unlock(); return strings.Join(d.opened, " ") }
	for i := 0; i < 100 && opened() == ""; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	requireEqual(t, opened(), "https://site.example/sousou-no-frieren-episode-6")

	// A show nobody watched in Firefox has no address.
	requireOK(t, f.h.Watchlog.SaveAnime(f.ctx, &store.Anime{ID: 2, Title: "Frieren 2"}))
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_open", AnimeID: 2, Episode: 1})
	requireEqual(t, a.wait(t, "error").(errorMsg).Message, "Nobody has said where Frieren 2 is watched yet. Set its address in the show's window.")

	// A page linked to a show is that show's: its address is learned at once.
	page = playStatus{State: "playing", Source: "firefox", Length: 100, Title: "Some Podcast Episode 5 - Site", URL: "https://other.example/watch?v=abc&ep=5"}
	d.set(page)
	f.r.checkPlayback(f.ctx)
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_link", AnimeID: 2})
	a.wait(t, "playing")
	for i := 0; i < 100 && got.WatchURL != "https://other.example/watch?v=abc&ep={ep}"; i++ {
		time.Sleep(10 * time.Millisecond)
		got, err = f.h.Watchlog.Anime(f.ctx, 2)
		requireOK(t, err)
	}
	requireEqual(t, got.WatchURL, "https://other.example/watch?v=abc&ep={ep}")
}

// A site whose addresses do not say the episode: only the page of the
// latest one is known, for opening that episode again.
func TestWatchRemembersPageWithoutEpisode(t *testing.T) {
	f, d := newWatchFixture(t)
	viewer := f.user("alice")
	f.permission(viewer, store.Permission{Remote: true})
	alice, a := f.join(viewer)
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watching", Watching: true})

	page := playStatus{State: "playing", Source: "firefox", Length: 100, Title: "Sousou no Frieren Episode 11 - Site", URL: "https://site.example/watch/G4VUQ1/the-journeys-end"}
	for page.Time = 0; page.Time < 100; page.Time += watchBucket {
		d.set(page)
		f.r.checkPlayback(f.ctx)
	}
	page.State = "stopped"
	d.set(page)
	f.r.checkPlayback(f.ctx)
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_answer", ID: lastProposal(t, a).ID, Checked: true})
	got, err := f.h.Watchlog.Anime(f.ctx, 1)
	requireOK(t, err)
	requireEqual(t, got.WatchURL, "")
	requireEqual(t, got.PageURL, page.URL)
	requireEqual(t, got.PageEpisode, 11)

	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_open", AnimeID: 1, Episode: 12})
	requireEqual(t, a.wait(t, "error").(errorMsg).Message, "Only the page of episode 11 of Sousou no Frieren is known.")
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_open", AnimeID: 1, Episode: 11})
	opened := func() string { d.mu.Lock(); defer d.mu.Unlock(); return strings.Join(d.opened, " ") }
	for i := 0; i < 100 && opened() == ""; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	requireEqual(t, opened(), page.URL)

	// The next episode, the name linked now, is remembered as it starts.
	page = playStatus{State: "playing", Source: "firefox", Length: 100, Title: "Sousou no Frieren Episode 12 - Site", URL: "https://site.example/watch/G4VUQ2/a-new-start"}
	d.set(page)
	f.r.checkPlayback(f.ctx)
	got, err = f.h.Watchlog.Anime(f.ctx, 1)
	requireOK(t, err)
	requireEqual(t, got.PageURL, page.URL)
	requireEqual(t, got.PageEpisode, 12)
}

// A page whose title and address say different episodes: the address
// follows the episode as corrected.
func TestWatchAddressFollowsCorrectedEpisode(t *testing.T) {
	f, d := newWatchFixture(t)
	alice, a := f.join(f.user("alice"))
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watching", Watching: true})

	page := playStatus{State: "playing", Source: "firefox", Length: 100, Title: "Sousou no Frieren Episode 10 - Site", URL: "https://site.example/watch/AbC/sousou-no-frieren?ep=9"}
	for page.Time = 0; page.Time < 100; page.Time += watchBucket {
		d.set(page)
		f.r.checkPlayback(f.ctx)
	}
	page.State = "stopped"
	d.set(page)
	f.r.checkPlayback(f.ctx)
	p := lastProposal(t, a)
	requireEqual(t, p.Episode, 10)
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_episode", ID: p.ID, Episode: 9})
	f.r.Handle(f.ctx, alice, ClientMsg{Type: "watch_answer", ID: newestProposal(t, a).ID, Checked: true})
	got, err := f.h.Watchlog.Anime(f.ctx, 1)
	requireOK(t, err)
	requireEqual(t, got.WatchURL, "https://site.example/watch/AbC/sousou-no-frieren?ep={ep}")
	requireEqual(t, got.PageEpisode, 9)
	requireEqual(t, f.progress()["alice"], "9")
}

func TestWatchlogFollowsAiring(t *testing.T) {
	f, _ := newWatchFixture(t)
	// Airing, its length not known: AniList knows it now. One that has
	// finished is not asked about.
	requireOK(t, f.h.Watchlog.SaveAnime(f.ctx, &store.Anime{ID: 1, Title: "Frieren"}))
	twelve := 12
	requireOK(t, f.h.Watchlog.SaveAnime(f.ctx, &store.Anime{ID: 5, Title: "Done", Episodes: &twelve}))
	requireEqual(t, f.h.updateAiring(f.ctx).IsZero(), true) // nothing more announced
	got, err := f.h.Watchlog.Anime(f.ctx, 1)
	requireOK(t, err)
	requireEqual(t, *got.Episodes, 28)
	ids, err := f.h.Watchlog.AiringAnime(f.ctx)
	requireOK(t, err)
	requireEqual(t, len(ids), 0)
}
