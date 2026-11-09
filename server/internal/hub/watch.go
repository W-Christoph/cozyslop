package hub

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"

	"cozycast/internal/anilist"
	"cozycast/internal/episode"
	"cozycast/internal/store"
)

// The watchlog (docs/architecture.md, "Watchlog"). While it is on, the room
// follows what the VLC started by the play helper plays, and notes which
// part of an episode each person in the room saw. When the episode ends it
// suggests marking it watched for those who saw most of it; the people in
// the room can change the suggestion before it is saved.

const (
	watchInterval = 5 * time.Second
	watchBucket   = 10 // seconds: an episode is seen in parts this long
	watchMisses   = 3  // unanswered polls before the episode counts as ended
	// Shares of an episode someone saw: to be listed in the suggestion, to
	// be ticked in it, and that someone must have seen for a suggestion.
	watchListed  = 0.2
	watchChecked = 0.8
	watchWorth   = 0.5
	maxEpisode   = 9999
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
}

// watching is the file VLC is playing.
type watching struct {
	file   string
	name   episode.Name
	known  bool         // the name has an episode number
	anime  *store.Anime // nil if not recognised
	length int
	seen   map[string]*viewer // by identity key
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
	alias string           // the file name's title, remembered for its show on save
	anime *store.Anime     // nil while no show is picked
	users map[string]int64 // account ids of the viewers, by identity key
	timer *time.Timer
}

func (w *watching) playing() *WatchPlaying {
	if w == nil || !w.known {
		return nil
	}
	p := &WatchPlaying{Title: w.name.Title, Episode: w.name.Episode}
	if w.anime != nil {
		p.AnimeID, p.Title = w.anime.ID, w.anime.Title
	}
	return p
}

// proposal suggests the episode for those who saw enough of it; nil if
// nobody saw much.
func (w *watching) proposal() *proposal {
	if !w.known || w.length <= 0 {
		return nil
	}
	parts := float64((w.length + watchBucket - 1) / watchBucket)
	p := &proposal{alias: w.name.Alias(), users: make(map[string]int64)}
	p.Episode, p.File = w.name.Episode, w.file
	p.setAnime(w.anime, w.name.Title)
	best := 0.0
	for key, v := range w.seen {
		share := min(1, float64(len(v.parts))/parts)
		best = max(best, share)
		if share < watchListed {
			continue
		}
		p.Viewers = append(p.Viewers, WatchViewer{Key: key, Nickname: v.nickname,
			Coverage: math.Round(share*100) / 100, Checked: share >= watchChecked})
		p.users[key] = v.userID
	}
	if best < watchWorth {
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

// saveable: p has a show, and the episode is one it has. A file numbered
// past the show's end (or a show picked later) waits to be corrected.
func (p *proposal) saveable() bool {
	return p.anime != nil && (p.Episodes == nil || p.Episode <= *p.Episodes)
}

// view is the proposal as sent; a copy, since sending may outlast the lock.
func (p *proposal) view() *WatchProposal {
	if p == nil {
		return nil
	}
	v := p.WatchProposal
	v.Viewers = slices.Clone(p.Viewers)
	return &v
}

// seesStreamLocked: a tab of m shows the stream, playing.
func seesStreamLocked(m *member) bool {
	for _, c := range m.clients {
		if c.active && !c.paused && !c.listening {
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
	st, err := fetchPlayStatus(ctx, r.neko.Transport(), r.statusURL, r.playToken)
	if err != nil || st.State == "unknown" {
		if r.watchMisses++; r.watchMisses >= watchMisses {
			r.endWatching(ctx)
		}
		return
	}
	r.watchMisses = 0
	switch {
	case st.State == "paused":
		return
	case st.State != "playing" || st.File == "":
		r.endWatching(ctx)
		return
	}
	r.mu.Lock()
	w := r.watch
	r.mu.Unlock()
	if w == nil || w.file != st.File {
		r.endWatching(ctx)
		w = r.startWatching(ctx, st.File)
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
		if m.user == nil || !seesStreamLocked(m) {
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

func (r *Room) startWatching(ctx context.Context, file string) *watching {
	w := &watching{file: file, seen: make(map[string]*viewer)}
	w.name, w.known = episode.Parse(file)
	if w.known {
		w.anime = r.findAnime(ctx, w.name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.watch = w
	if w.known {
		r.broadcastLocked(playingMsg{Type: "playing", Playing: w.playing()}, nil)
	}
	return w
}

// findAnime is the show a file name means: the one it was matched to
// before, or AniList's best match. nil if neither knows.
func (r *Room) findAnime(ctx context.Context, n episode.Name) *store.Anime {
	a, err := r.hub.store.AnimeByAlias(ctx, n.Alias())
	if err == nil {
		return a
	}
	if !errors.Is(err, store.ErrNotFound) {
		r.log.Warn("watchlog alias", "err", err)
	}
	if r.hub.AniList == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	found, err := r.hub.AniList.Search(ctx, n.Query())
	if err != nil {
		r.log.Warn("anilist search", "err", err)
		return nil
	}
	if len(found) == 0 {
		return nil
	}
	return found[0].Anime()
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
		r.broadcastLocked(playingMsg{Type: "playing"}, nil)
	}
	p := w.proposal()
	var old *proposal
	if p != nil {
		old = r.takeProposalLocked()
		r.proposalSeq++
		p.ID = r.proposalSeq
		r.proposal = p
		r.scheduleLocked(p)
		r.broadcastLocked(watchMsg{Type: "watch", Proposal: p.view()}, nil)
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
			r.broadcastLocked(playingMsg{Type: "playing"}, nil)
		}
		r.watch = nil
	}
	if r.takeProposalLocked() != nil {
		r.broadcastLocked(watchMsg{Type: "watch"}, nil)
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
		r.broadcastLocked(watchMsg{Type: "watch"}, nil)
		r.mu.Unlock()
		r.saveProposal(context.Background(), p)
	})
}

// saveProposal records the episode for the ticked viewers, and the show
// for the file name's title. One that is not saveable is dropped.
func (r *Room) saveProposal(ctx context.Context, p *proposal) error {
	if !p.saveable() {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err := r.hub.store.SaveAnime(ctx, p.anime)
	if err == nil && p.alias != "" {
		err = r.hub.store.SaveAlias(ctx, p.alias, p.anime.ID)
	}
	if err == nil {
		var ids []int64
		for _, v := range p.Viewers {
			if v.Checked {
				ids = append(ids, p.users[v.Key])
			}
		}
		err = r.hub.store.RaiseProgress(ctx, ids, p.anime.ID, p.Episode)
	}
	if err != nil {
		r.log.Error("save watched episode", "err", err)
	}
	return err
}

var errProposalGone = &userError{"That suggestion is gone."}

// editProposal handles watch_check, watch_episode, watch_save and
// watch_dismiss: anyone in the room with an account may change or settle
// the suggestion.
func (r *Room) editProposal(ctx context.Context, c *Client, msg ClientMsg) error {
	r.mu.Lock()
	if c.m.user == nil {
		r.mu.Unlock()
		return ErrNotAllowed
	}
	p := r.proposal
	if p == nil || p.ID != msg.ID {
		r.mu.Unlock()
		return errProposalGone
	}
	switch msg.Type {
	case "watch_check":
		i := slices.IndexFunc(p.Viewers, func(v WatchViewer) bool { return v.Key == msg.Key })
		if i < 0 {
			r.mu.Unlock()
			return &userError{"That person is not in the suggestion."}
		}
		p.Viewers[i].Checked = msg.Checked
	case "watch_episode":
		if msg.Episode < 1 || msg.Episode > maxEpisode || p.Episodes != nil && msg.Episode > *p.Episodes {
			r.mu.Unlock()
			return &userError{"The show has no such episode."}
		}
		p.Episode = msg.Episode
	case "watch_save":
		if p.anime == nil {
			r.mu.Unlock()
			return &userError{"Pick the show first."}
		}
		if !p.saveable() {
			r.mu.Unlock()
			return &userError{"The show has no such episode."}
		}
		r.takeProposalLocked()
		r.broadcastLocked(watchMsg{Type: "watch"}, nil)
		r.mu.Unlock()
		return r.saveProposal(ctx, p)
	case "watch_dismiss":
		r.takeProposalLocked()
		r.broadcastLocked(watchMsg{Type: "watch"}, nil)
		r.mu.Unlock()
		return nil
	}
	r.scheduleLocked(p)
	r.broadcastLocked(watchMsg{Type: "watch", Proposal: p.view()}, nil)
	r.mu.Unlock()
	return nil
}

// changeProposalShow picks another show for the suggestion (watch_anime).
func (r *Room) changeProposalShow(ctx context.Context, c *Client, id, animeID int64) error {
	r.mu.Lock()
	user, key, p := c.m.user, c.m.key, r.proposal
	r.mu.Unlock()
	if user == nil {
		return ErrNotAllowed
	}
	if p == nil || p.ID != id {
		return errProposalGone
	}
	if !r.showLimit.Allow(key) {
		return ErrRateLimited
	}
	a, err := r.hub.store.Anime(ctx, animeID)
	if errors.Is(err, store.ErrNotFound) {
		if r.hub.AniList == nil {
			return &userError{"AniList is not available."}
		}
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		var m anilist.Media
		m, err = r.hub.AniList.Get(ctx, animeID)
		if errors.Is(err, anilist.ErrNotFound) {
			return &userError{"AniList has no such anime."}
		}
		if err != nil {
			r.log.Warn("anilist get", "err", err)
			return &userError{"AniList did not answer. Try again."}
		}
		a = m.Anime()
	} else if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.proposal != p {
		return errProposalGone
	}
	p.setAnime(a, p.Title)
	r.scheduleLocked(p)
	r.broadcastLocked(watchMsg{Type: "watch", Proposal: p.view()}, nil)
	return nil
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
