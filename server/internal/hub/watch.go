package hub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"cozycast/internal/anilist"
	"cozycast/internal/diag"
	"cozycast/internal/episode"
	"cozycast/internal/store"
)

// The watchlog (docs/architecture.md, "Watchlog"). While it is on, the room
// follows what the VLC started by the play helper plays, or else Firefox,
// and notes which part of an episode each person in the room saw. When the
// episode ends, everyone who saw some of it is asked whether they watched
// it, and sees only their own part. Accounts that left the watchlog are
// neither followed nor told what plays. A page's address is remembered for
// its show, to open its next episode there (openEpisode).

const (
	watchInterval = 5 * time.Second
	watchBucket   = 10 // seconds: an episode is seen in parts this long
	watchMisses   = 3  // unanswered polls before the episode counts as ended
	// Shares of an episode someone saw: to be asked about it, and for the
	// answer to be yes unless they say no.
	watchListed  = 0.2
	watchChecked = 0.6
	// A web page keeps a video that is over, paused at its end; one paused
	// this far in, during the credits, counts as over too.
	watchEnding = 0.9
	maxEpisode  = 9999
)

// watchSaveAfter is how long a suggestion waits before it is saved by
// itself. A variable for tests.
var watchSaveAfter = 2 * time.Minute

// playStatus is the play helper's answer to GET /status.
type playStatus struct {
	State  string `json:"state"` // playing | paused | stopped | unknown
	Time   int    `json:"time"`  // seconds
	Length int    `json:"length"`
	File   string `json:"file"`
	// Firefox instead of VLC ("firefox"): the page's title and address
	// instead of a file.
	Source string `json:"source"`
	Title  string `json:"title"`
	URL    string `json:"url"`
}

// played is what the status names: the file, or for a web page its address
// and what that and its title say. key tells one episode from the next. A
// page's texts may change while it plays, so its key is the episode they
// name; pages that name none are all one.
func (st playStatus) played() (file, key string, name episode.Name, known bool) {
	if st.Source != "firefox" {
		name, known = episode.Parse(st.File)
		return st.File, "file:" + st.File, name, known
	}
	file = st.URL
	if file == "" {
		file = st.Title
	}
	if name, known = episode.ParsePage(st.Title, st.URL); !known {
		return file, "page", name, false
	}
	return file, "page:" + name.Alias() + "#" + strconv.Itoa(name.Episode), name, true
}

// watching is the file VLC is playing, or the page Firefox is.
type watching struct {
	file   string
	key    string
	name   episode.Name
	known  bool         // the name has an episode number
	page   bool         // a page of Firefox's, not a file
	anime  *store.Anime // nil if not recognised
	linked bool         // the name was linked to the show before: not AniList's guess
	// template is a page's address for the show's other episodes
	// (episode.Template); "" if it does not say the episode.
	template string
	pageURL  string // the page's address, if it is a web address
	length   int
	seen     map[string]*viewer // by identity key
}

// viewer is the parts of an episode one account saw.
type viewer struct {
	userID   int64
	nickname string
	parts    map[int]bool
}

// proposal is a suggestion to mark an episode watched.
type proposal struct {
	WatchProposal
	alias   string           // the file name's title, remembered for its show on save
	pageURL string           // the page's address, remembered for its show on save
	anime   *store.Anime     // nil while no show is picked
	users   map[string]int64 // account ids of the viewers, by identity key
	timer   *time.Timer
}

func (w *watching) playing() *WatchPlaying {
	if w == nil || !w.known {
		return nil
	}
	p := &WatchPlaying{Title: w.name.Title, Episode: w.name.Episode}
	if w.anime != nil {
		p.AnimeID, p.Title, p.Linked = w.anime.ID, w.anime.Title, w.linked
	}
	return p
}

// proposal suggests the episode for those who saw enough of it; nil if
// nobody did. A page without a show suggests nothing: other videos
// have episodes too, and nobody said that this one is a show's.
func (w *watching) proposal() *proposal {
	if !w.known || w.length <= 0 || w.page && w.anime == nil {
		return nil
	}
	parts := float64((w.length + watchBucket - 1) / watchBucket)
	p := &proposal{alias: w.name.Alias(), pageURL: w.pageURL, users: make(map[string]int64)}
	p.Episode, p.File = w.name.Episode, w.file
	p.setAnime(w.anime, w.name.Title)
	for key, v := range w.seen {
		share := min(1, float64(len(v.parts))/parts)
		if share < watchListed {
			continue
		}
		p.Viewers = append(p.Viewers, WatchViewer{Key: key, Nickname: v.nickname,
			Coverage: math.Round(share*100) / 100, Checked: share >= watchChecked})
		p.users[key] = v.userID
	}
	if len(p.Viewers) == 0 {
		return nil
	}
	slices.SortFunc(p.Viewers, func(a, b WatchViewer) int {
		return strings.Compare(strings.ToLower(a.Nickname), strings.ToLower(b.Nickname))
	})
	return p
}

func (p *proposal) setAnime(a *store.Anime, title string) {
	p.anime = a
	if a == nil {
		p.AnimeID, p.Title, p.CoverURL, p.Episodes = 0, title, "", nil
		return
	}
	p.AnimeID, p.Title, p.CoverURL, p.Episodes = a.ID, a.Title, a.CoverURL, a.Episodes
}

// countEarlierLocked notes for each viewer how many episodes of the show
// before the one suggested they have not marked: someone who watched those
// elsewhere may say so with their answer. None while there is no show.
func (r *Room) countEarlierLocked(ctx context.Context, p *proposal) {
	for i := range p.Viewers {
		v := &p.Viewers[i]
		v.Earlier = 0
		if p.anime == nil || p.Episode <= 1 {
			continue
		}
		watched, err := r.hub.Watchlog.Watched(ctx, p.users[v.Key], p.anime.ID)
		if err != nil {
			r.log.Warn("watchlog progress", "err", err)
			continue
		}
		v.Earlier = p.Episode - 1
		for _, w := range watched {
			v.Earlier -= max(0, min(w[1], p.Episode-1)-w[0]+1)
		}
	}
	for i := range p.Viewers {
		p.Viewers[i].EarlierToo = p.Viewers[i].EarlierToo && p.Viewers[i].Earlier > 0
	}
}

// saveable: p has a show, and the episode is one it has. A file numbered
// past the show's end (or a show picked later) waits to be corrected.
func (p *proposal) saveable() bool {
	return p.anime != nil && (p.Episodes == nil || p.Episode <= *p.Episodes)
}

// viewer is the index of a viewer in p; -1 if they are not asked.
func (p *proposal) viewer(key string) int {
	return slices.IndexFunc(p.Viewers, func(v WatchViewer) bool { return v.Key == key })
}

// viewFor is the proposal as m is sent it: with m as its only viewer, and
// nil for those who are not asked. A copy, since sending may outlast the
// lock.
func (p *proposal) viewFor(m *member) *WatchProposal {
	if p == nil || !tracked(m) {
		return nil
	}
	i := p.viewer(m.key)
	if i < 0 {
		return nil
	}
	v := p.WatchProposal
	v.Viewers = []WatchViewer{p.Viewers[i]}
	return &v
}

// tracked: the watchlog follows m, an account that did not leave it.
func tracked(m *member) bool {
	return m.user != nil && !m.user.WatchlogOff
}

// playingFor is what plays as m is told it: nothing for an account that
// left the watchlog.
func (w *watching) playingFor(m *member) *WatchPlaying {
	if m.user != nil && m.user.WatchlogOff {
		return nil
	}
	return w.playing()
}

// sendPlayingLocked tells everyone what plays now; nil w is nothing.
func (r *Room) sendPlayingLocked(w *watching) {
	for _, c := range r.clients {
		c.send(playingMsg{Type: "playing", Playing: w.playingFor(c.m)})
	}
}

// sendProposalLocked tells everyone about the open suggestion, as viewFor.
func (r *Room) sendProposalLocked() {
	for _, c := range r.clients {
		c.send(watchMsg{Type: "watch", Proposal: r.proposal.viewFor(c.m)})
	}
}

// watchlogLeftLocked forgets what m saw of the episode and their part of the
// suggestion, which goes if nobody else is asked: m left the watchlog.
func (r *Room) watchlogLeftLocked(m *member) {
	if r.watch != nil {
		delete(r.watch.seen, m.key)
	}
	if p := r.proposal; p != nil {
		if i := p.viewer(m.key); i >= 0 {
			p.Viewers = slices.Delete(p.Viewers, i, i+1)
			delete(p.users, m.key)
		}
		if len(p.Viewers) == 0 {
			r.takeProposalLocked()
		}
	}
}

// seesStreamLocked: a tab of m shows the stream, playing. The tab says so
// itself ("watching"): being in the room with the stream paused, without
// its picture or not connected is not watching, whether or not the tab
// tells the others that it is paused.
func seesStreamLocked(m *member) bool {
	for _, c := range m.clients {
		if c.active && c.watching {
			return true
		}
	}
	return false
}

func (r *Room) watchPlayback(ctx context.Context) {
	t := time.NewTicker(watchInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.checkPlayback(ctx)
		}
	}
}

// checkPlayback asks the desktop what VLC plays, while someone is in the
// room, and notes who sees it. Only this goroutine changes r.watch.
func (r *Room) checkPlayback(ctx context.Context) {
	if !r.hub.watchlog.Load() {
		r.dropWatching()
		return
	}
	r.mu.Lock()
	watched := len(r.clients) > 0
	r.mu.Unlock()
	if !watched {
		return
	}
	st, err := fetchPlayStatus(ctx, r.helperTransport(true), r.statusURL, r.playToken)
	if err != nil || st.State == "unknown" {
		if r.watchMisses++; r.watchMisses >= watchMisses {
			r.endWatching(ctx)
		}
		return
	}
	r.watchMisses = 0
	file, key, name, known := st.played()
	r.mu.Lock()
	w := r.watch
	r.mu.Unlock()
	switch {
	case st.State == "paused":
		if w != nil && w.key == key && st.Source == "firefox" && st.Length > 0 &&
			float64(st.Time) >= watchEnding*float64(st.Length) {
			r.endWatching(ctx)
		}
		return
	case st.State != "playing" || file == "":
		r.endWatching(ctx)
		return
	}
	if w == nil || w.key != key {
		r.endWatching(ctx)
		w = r.startWatching(ctx, file, key, name, known, st.Source == "firefox")
	}
	if !w.known {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if st.Length > 0 {
		w.length = st.Length
	}
	part := st.Time / watchBucket
	for key, m := range r.members {
		if !tracked(m) || !seesStreamLocked(m) {
			continue
		}
		v := w.seen[key]
		if v == nil {
			v = &viewer{userID: m.user.ID, parts: make(map[int]bool)}
			w.seen[key] = v
		}
		v.nickname = m.user.Nickname
		v.parts[part] = true
	}
}

// startWatching follows a new file or page. One whose show neither AniList
// nor an earlier episode knows is followed all the same: someone may say
// which show it is (linkPlaying).
func (r *Room) startWatching(ctx context.Context, file, key string, name episode.Name, known, page bool) *watching {
	w := &watching{file: file, key: key, name: name, known: known, page: page, seen: make(map[string]*viewer)}
	if w.known {
		w.anime, w.linked = r.findAnime(ctx, w.name)
	}
	if page && known {
		w.template = episode.Template(file, name.Episode)
		if u, err := url.Parse(file); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
			w.pageURL = file
		}
	}
	// A show the name was linked to is the one this page plays: it is
	// watched here now. AniList's guess waits until someone says yes.
	if w.linked {
		r.saveAddress(ctx, w.anime.ID, w.template, w.pageURL, w.name.Episode)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.watch = w
	if w.known {
		r.sendPlayingLocked(w)
	}
	return w
}

// findAnime is the show a file name means: the one it was matched to
// before (linked), or AniList's best match. nil if neither knows.
func (r *Room) findAnime(ctx context.Context, n episode.Name) (a *store.Anime, linked bool) {
	a, err := r.hub.Watchlog.AnimeByAlias(ctx, n.Alias())
	if err == nil {
		r.log.Info("anime detection", "outcome", "alias")
		return a, true
	}
	if !errors.Is(err, store.ErrNotFound) {
		r.log.Warn("watchlog alias", "err", err)
	}
	if r.hub.AniList == nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(diag.WithCaller(ctx, "room_detect", r.Name), 10*time.Second)
	defer cancel()
	found, err := r.hub.AniList.Search(ctx, n.Query())
	if err != nil {
		r.log.Warn("anilist search", "err", err)
		return nil, false
	}
	if len(found) > 0 {
		r.log.Info("anime detection", "outcome", "search")
		return found[0].Anime(), false
	}
	// A page's address leaves out the signs between a title's letters ("im
	// dating" for "I'm Dating", "jojos" for "JoJo's"), and AniList finds
	// nothing for such a word. Then a few of the title's words are searched,
	// for a show whose title is the same letters: other seasons are found
	// with them, and are not it.
	words := strings.Fields(n.Title)
	var parts [][]string
	switch {
	case len(words) >= 4:
		parts = [][]string{words[:3], words[len(words)-3:], words[:2]}
	case len(words) == 3:
		parts = [][]string{words[:2], words[1:]}
	}
	// A later season has its number in the title, one way or another.
	want := []string{letters(n.Title)}
	if n.Season > 1 {
		no := strconv.Itoa(n.Season)
		ordinal := no + "th"
		if n.Season <= 3 {
			ordinal = no + [...]string{"nd", "rd"}[n.Season-2]
		}
		want = []string{want[0] + "season" + no, want[0] + no, want[0] + ordinal + "season", want[0] + "part" + no}
	}
	for _, part := range parts {
		found, err := r.hub.AniList.Search(ctx, strings.Join(part, " "))
		if err != nil {
			r.log.Warn("anilist search", "err", err)
			return nil, false
		}
		for _, m := range found {
			if slices.Contains(want, letters(m.Title.Romaji)) || slices.Contains(want, letters(m.Title.English)) {
				r.log.Info("anime detection", "outcome", "search_words")
				return m.Anime(), false
			}
		}
	}
	r.log.Info("anime detection", "outcome", "miss")
	return nil, false
}

// letters is a title's letters and digits, in lower case.
func letters(title string) string {
	return strings.Map(func(c rune) rune {
		if unicode.IsLetter(c) || unicode.IsDigit(c) {
			return unicode.ToLower(c)
		}
		return -1
	}, title)
}

// endWatching ends the episode being watched, suggesting it for those who
// saw it. A suggestion still open is saved first.
func (r *Room) endWatching(ctx context.Context) {
	r.mu.Lock()
	w := r.watch
	if w == nil {
		r.mu.Unlock()
		return
	}
	r.watch = nil
	if w.known {
		r.sendPlayingLocked(nil)
	}
	p := w.proposal()
	var old *proposal
	if p != nil {
		old = r.takeProposalLocked()
		r.proposalSeq++
		p.ID = r.proposalSeq
		r.proposal = p
		r.countEarlierLocked(ctx, p)
		r.scheduleLocked(p)
		r.sendProposalLocked()
	}
	r.mu.Unlock()
	if old != nil {
		r.saveProposal(ctx, old)
	}
}

// dropWatching forgets the episode and the suggestion, unsaved: the
// watchlog was turned off.
func (r *Room) dropWatching() {
	r.watchMisses = 0
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.watch != nil {
		if r.watch.known {
			r.sendPlayingLocked(nil)
		}
		r.watch = nil
	}
	if r.takeProposalLocked() != nil {
		r.sendProposalLocked()
	}
}

// takeProposalLocked removes the open suggestion and returns it.
func (r *Room) takeProposalLocked() *proposal {
	p := r.proposal
	if p != nil && p.timer != nil {
		p.timer.Stop()
	}
	r.proposal = nil
	return p
}

// scheduleLocked (re)starts the wait before p is saved by itself. Until it
// is saveable it waits for someone to pick the show or fix the episode.
func (r *Room) scheduleLocked(p *proposal) {
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
	p.ApplyAt = 0
	if !p.saveable() {
		return
	}
	p.ApplyAt = time.Now().Add(watchSaveAfter).UnixMilli()
	id := p.ID
	p.timer = time.AfterFunc(watchSaveAfter, func() {
		r.mu.Lock()
		if r.proposal == nil || r.proposal.ID != id {
			r.mu.Unlock()
			return
		}
		p := r.takeProposalLocked()
		r.sendProposalLocked()
		r.mu.Unlock()
		r.saveProposal(context.Background(), p)
	})
}

// saveProposal records the episode for the ticked viewers, and the show
// for the file name's title. One that is not saveable, or that nobody
// watched, is dropped.
func (r *Room) saveProposal(ctx context.Context, p *proposal) error {
	var ids, earlier []int64
	for _, v := range p.Viewers {
		if v.Checked {
			ids = append(ids, p.users[v.Key])
			if v.EarlierToo {
				earlier = append(earlier, p.users[v.Key])
			}
		}
	}
	if !p.saveable() || len(ids) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err := r.hub.Watchlog.SaveAnime(ctx, p.anime)
	if err == nil && p.alias != "" {
		err = r.hub.Watchlog.SaveAlias(ctx, p.alias, p.anime.ID)
	}
	// The episode as corrected, where the page's title and address differ.
	if t := episode.Template(p.pageURL, p.Episode); err == nil && t != "" {
		err = r.hub.Watchlog.SetWatchURL(ctx, p.anime.ID, t)
	}
	if err == nil && p.pageURL != "" {
		err = r.hub.Watchlog.SetPage(ctx, p.anime.ID, p.pageURL, p.Episode)
	}
	for _, id := range earlier {
		if err == nil && p.Episode > 1 {
			err = r.hub.Watchlog.SetWatched(ctx, id, p.anime.ID, 1, p.Episode-1, true, "manual")
		}
	}
	if err == nil {
		err = r.hub.Watchlog.MarkWatched(ctx, ids, p.anime.ID, p.Episode)
	}
	if err != nil {
		r.log.Error("save watched episode", "err", err)
	}
	return err
}

var (
	errProposalGone = &userError{"That suggestion is gone."}
	errNotAsked     = &userError{"You are not asked about that episode."}
)

// editProposal handles watch_answer, watch_earlier and watch_episode, from
// those asked about the episode: whether they watched it themselves
// (watch_answer) is the end of it for them, and for everyone once all have
// answered. Each may ask for the episodes before it that they have not
// marked to be marked with their yes (watch_earlier), and take that back:
// it marks nothing by itself. Anyone asked may correct the episode.
func (r *Room) editProposal(ctx context.Context, c *Client, msg ClientMsg) error {
	r.mu.Lock()
	p := r.proposal
	if p == nil || p.ID != msg.ID {
		r.mu.Unlock()
		return errProposalGone
	}
	i := p.viewer(c.m.key)
	if !tracked(c.m) || i < 0 {
		r.mu.Unlock()
		return errNotAsked
	}
	switch msg.Type {
	case "watch_answer":
		// Yes needs the show, and an episode it has: there is nothing to
		// save it to before.
		if msg.Checked && !p.saveable() {
			r.mu.Unlock()
			return &userError{"Pick the show first."}
		}
		p.Viewers[i].Checked, p.Viewers[i].Answered = msg.Checked, true
		// With everyone's answer there is nothing left to wait for. An
		// answer does not put the saving off, as a correction does.
		if !slices.ContainsFunc(p.Viewers, func(v WatchViewer) bool { return !v.Answered }) {
			r.takeProposalLocked()
			r.sendProposalLocked()
			r.mu.Unlock()
			return r.saveProposal(ctx, p)
		}
		r.sendProposalLocked()
		r.mu.Unlock()
		return nil
	case "watch_earlier":
		// Neither an answer nor a correction: the wait goes on as it was.
		p.Viewers[i].EarlierToo = msg.Earlier && p.Viewers[i].Earlier > 0
		r.sendProposalLocked()
		r.mu.Unlock()
		return nil
	case "watch_episode":
		if msg.Episode < 1 || msg.Episode > maxEpisode || p.Episodes != nil && msg.Episode > *p.Episodes {
			r.mu.Unlock()
			return &userError{"The show has no such episode."}
		}
		p.Episode = msg.Episode
		r.countEarlierLocked(ctx, p)
	}
	r.scheduleLocked(p)
	r.sendProposalLocked()
	r.mu.Unlock()
	return nil
}

// changeProposalShow picks another show for the suggestion (watch_anime),
// for one asked about it.
func (r *Room) changeProposalShow(ctx context.Context, c *Client, id, animeID int64) error {
	r.mu.Lock()
	key, p := c.m.key, r.proposal
	asked := p != nil && tracked(c.m) && p.viewer(key) >= 0
	r.mu.Unlock()
	if p == nil || p.ID != id {
		return errProposalGone
	}
	if !asked {
		return errNotAsked
	}
	if !r.showLimit.Allow(key) {
		return ErrRateLimited
	}
	a, err := r.showByID(ctx, animeID)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.proposal != p {
		return errProposalGone
	}
	p.setAnime(a, p.Title)
	r.countEarlierLocked(ctx, p)
	r.scheduleLocked(p)
	r.sendProposalLocked()
	return nil
}

// showByID is a show by its AniList id: the watchlog's, or else AniList's.
func (r *Room) showByID(ctx context.Context, animeID int64) (*store.Anime, error) {
	a, err := r.hub.Watchlog.Anime(ctx, animeID)
	if !errors.Is(err, store.ErrNotFound) {
		return a, err
	}
	if r.hub.AniList == nil {
		return nil, &userError{"AniList is not available."}
	}
	ctx, cancel := context.WithTimeout(diag.WithCaller(ctx, "room_pick", r.Name), 10*time.Second)
	defer cancel()
	m, err := r.hub.AniList.Get(ctx, animeID)
	if errors.Is(err, anilist.ErrNotFound) {
		return nil, &userError{"AniList has no such anime."}
	}
	if err != nil {
		r.log.Warn("anilist get", "err", err)
		return nil, &userError{"AniList did not answer. Try again."}
	}
	return m.Anime(), nil
}

// linkPlaying says which show the episode playing is of (watch_link), for
// one that was not recognised or was taken for another. The name it goes
// by is that show's from then on: for its other episodes, in every room,
// without asking AniList. Anyone in the room with an account that takes
// part in the watchlog may.
func (r *Room) linkPlaying(ctx context.Context, c *Client, animeID int64) error {
	r.mu.Lock()
	allowed, key, w := tracked(c.m), c.m.key, r.watch
	r.mu.Unlock()
	if !allowed {
		return ErrNotAllowed
	}
	if w == nil || !w.known {
		return &userError{"No episode is playing."}
	}
	if !r.showLimit.Allow(key) {
		return ErrRateLimited
	}
	a, err := r.showByID(ctx, animeID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := r.hub.Watchlog.SaveAnime(ctx, a); err != nil {
		return err
	}
	if alias := w.name.Alias(); alias != "" {
		if err := r.hub.Watchlog.SaveAlias(ctx, alias, a.ID); err != nil {
			return err
		}
	}
	r.saveAddress(ctx, a.ID, w.template, w.pageURL, w.name.Episode)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.watch == w {
		w.anime, w.linked = a, true
		r.sendPlayingLocked(w)
	}
	return nil
}

// saveAddress remembers where a show on the watchlog is watched: the
// address for its episodes, where the page's says which (template), and
// the page episode n played on. A show that is not on it (yet) has nothing
// to remember them for.
func (r *Room) saveAddress(ctx context.Context, animeID int64, template, page string, n int) {
	var err error
	if template != "" {
		err = r.hub.Watchlog.SetWatchURL(ctx, animeID, template)
	}
	if err == nil && page != "" {
		err = r.hub.Watchlog.SetPage(ctx, animeID, page, n)
	}
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		r.log.Warn("watchlog address", "err", err)
	}
}

// openEpisode opens an episode of a show on the watchlog in the desktop's
// Firefox (watch_open), at the show's address. It changes what everyone
// watches, as playing a file does: it needs the remote, respects who owns
// it, and an account that takes part in the watchlog.
func (r *Room) openEpisode(ctx context.Context, c *Client, animeID int64, n int) error {
	if !r.hub.watchlog.Load() {
		return &userError{"The watchlog is off."}
	}
	r.mu.Lock()
	allowed, owner := tracked(c.m) && c.m.rights.Remote, r.remoteOwnerLocked(c)
	r.mu.Unlock()
	if !allowed {
		return ErrNotAllowed
	}
	if owner != "" {
		return &userError{owner + " owns the remote."}
	}
	if n < 1 || n > maxEpisode {
		return &userError{"The show has no such episode."}
	}
	a, err := r.hub.Watchlog.Anime(ctx, animeID)
	if errors.Is(err, store.ErrNotFound) {
		return &userError{"That show is not on the watchlog."}
	}
	if err != nil {
		return err
	}
	// Where the address does not say the episode, only the page of the
	// latest one is known.
	address := a.PageURL
	if a.WatchURL != "" {
		address = episode.Fill(a.WatchURL, n)
	} else if a.PageURL == "" {
		return &userError{"Nobody has said where " + a.Title + " is watched yet. Set its address in the show's window."}
	} else if n != a.PageEpisode {
		return &userError{"Only the page of episode " + strconv.Itoa(a.PageEpisode) + " of " + a.Title + " is known."}
	}
	if r.openURL == "" {
		return &userError{"This room's desktop cannot open pages."}
	}
	status, err := r.postHelper(ctx, r.openURL+"?url="+url.QueryEscape(address))
	switch {
	case err != nil || status == http.StatusNotFound:
		// Restarting, or a room image from before /open.
		r.log.Warn("open page", "err", err, "status", status)
		return &userError{"The room's desktop did not open it. It may need a newer room image."}
	case status == http.StatusServiceUnavailable:
		return &userError{"The room's Firefox is not running. Try again in a moment."}
	case status != http.StatusNoContent:
		return fmt.Errorf("play helper: open: %d", status)
	}
	return nil
}

// helperTransport reaches the room's helpers (worker/play.py), keeping its
// requests in the diagnostics; polls are summarized per minute.
func (r *Room) helperTransport(poll bool) http.RoundTripper {
	return diag.Transport{Base: r.neko.Transport(), Store: r.hub.Diagnostics, Target: "helper", Room: r.Name, Summarize: poll}
}

func fetchPlayStatus(ctx context.Context, transport http.RoundTripper, url, token string) (playStatus, error) {
	var st playStatus
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return st, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	// The helper sits next to neko, reached the same way.
	res, err := (&http.Client{Timeout: watchInterval, Transport: transport}).Do(req)
	if err != nil {
		return st, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return st, errors.New("play helper: " + res.Status)
	}
	err = json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&st)
	return st, err
}
