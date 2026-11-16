package httpapi

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"cozycast/internal/anilist"
	"cozycast/internal/store"
)

// The watchlog: which episodes of which anime each account has watched
// (docs/architecture.md, "Watchlog"). Every endpoint needs an account and
// the watchlog setting on.

const maxEpisode = 9999

// watchlogUser returns the logged-in account if the watchlog is on, or
// answers 401/404 and returns nil.
func (s *Server) watchlogUser(w http.ResponseWriter, r *http.Request) *store.User {
	u := s.requireUser(w, r)
	if u == nil {
		return nil
	}
	set, err := s.store.Settings(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return nil
	}
	if !set.Watchlog || s.watchlog == nil {
		writeError(w, http.StatusNotFound, "The watchlog is off.")
		return nil
	}
	return u
}

// allowAniList limits how often an account makes the server ask AniList
// (searches and added shows together), or answers 429.
func (s *Server) allowAniList(w http.ResponseWriter, u *store.User) bool {
	if s.searchLimit.Allow(strconv.FormatInt(u.ID, 10)) {
		return true
	}
	writeError(w, http.StatusTooManyRequests, "Asking AniList too fast. Wait a moment.")
	return false
}

func (s *Server) getWatchlog(w http.ResponseWriter, r *http.Request) {
	if s.watchlogUser(w, r) == nil {
		return
	}
	ctx := r.Context()
	anime, err := s.watchlog.ListAnime(ctx)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	progress, err := s.watchlog.ListProgress(ctx)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	follows, err := s.watchlog.ListFollows(ctx)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	aliases, err := s.watchlog.ListAliases(ctx)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	// The watchlog knows accounts by id only. Those it has anything from
	// are listed, by username; disabled (and deleted) ones are left out.
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	names := map[int64]string{}
	for _, u := range users {
		if !u.Disabled {
			names[u.ID] = u.Username
		}
	}
	type person struct {
		Username  string `json:"username"`
		Nickname  string `json:"nickname"`
		NameColor string `json:"nameColor"`
		AvatarURL string `json:"avatarUrl"`
	}
	watched := map[int64]bool{}
	keptProgress := progress[:0]
	for _, p := range progress {
		if p.Username = names[p.UserID]; p.Username != "" {
			watched[p.UserID] = true
			keptProgress = append(keptProgress, p)
		}
	}
	sort.SliceStable(keptProgress, func(i, j int) bool {
		a, b := keptProgress[i], keptProgress[j]
		return a.AnimeID < b.AnimeID || a.AnimeID == b.AnimeID && a.Username < b.Username
	})
	keptFollows := follows[:0]
	for _, f := range follows {
		if f.Username = names[f.UserID]; f.Username != "" {
			keptFollows = append(keptFollows, f)
		}
	}
	sort.SliceStable(keptFollows, func(i, j int) bool {
		a, b := keptFollows[i], keptFollows[j]
		return a.AnimeID < b.AnimeID || a.AnimeID == b.AnimeID && a.Username < b.Username
	})
	people := []person{}
	for _, u := range users {
		if watched[u.ID] {
			people = append(people, person{u.Username, u.Nickname, u.NameColor, avatarURL(u)})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"anime": anime, "people": people, "progress": keptProgress, "follows": keptFollows, "aliases": aliases})
}

// searchAnime asks AniList for shows to add.
func (s *Server) searchAnime(w http.ResponseWriter, r *http.Request) {
	u := s.watchlogUser(w, r)
	if u == nil {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if n := utf8.RuneCountInString(q); n < 2 || n > 100 {
		writeError(w, http.StatusBadRequest, "Search for 2 to 100 characters.")
		return
	}
	if s.anilist == nil {
		writeError(w, http.StatusServiceUnavailable, "AniList is not available.")
		return
	}
	if !s.allowAniList(w, u) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	found, err := s.anilist.Search(ctx, q)
	if err != nil {
		s.log.Warn("anilist search", "err", err)
		writeError(w, http.StatusBadGateway, "AniList did not answer. Try again.")
		return
	}
	type result struct {
		*store.Anime
		Year *int `json:"year"`
	}
	out := make([]result, len(found))
	for i, m := range found {
		out[i] = result{m.Anime(), m.SeasonYear}
	}
	writeJSON(w, http.StatusOK, out)
}

// addAnime puts an AniList show on the watchlog, or refreshes it.
func (s *Server) addAnime(w http.ResponseWriter, r *http.Request) {
	u := s.watchlogUser(w, r)
	if u == nil {
		return
	}
	var req struct {
		ID int64 `json:"id"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if s.anilist == nil {
		writeError(w, http.StatusServiceUnavailable, "AniList is not available.")
		return
	}
	if !s.allowAniList(w, u) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	m, err := s.anilist.Get(ctx, req.ID)
	if errors.Is(err, anilist.ErrNotFound) {
		writeError(w, http.StatusNotFound, "AniList has no such anime.")
		return
	}
	if err != nil {
		s.log.Warn("anilist get", "err", err)
		writeError(w, http.StatusBadGateway, "AniList did not answer. Try again.")
		return
	}
	a := m.Anime()
	if err := s.watchlog.SaveAnime(r.Context(), a); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// deleteAnime takes a show off the watchlog with everyone's progress: an
// admin, or anyone while nobody has watched any of it.
func (s *Server) deleteAnime(w http.ResponseWriter, r *http.Request) {
	u := s.watchlogUser(w, r)
	if u == nil {
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	// A show nobody has watched was added by mistake: anyone takes it off.
	if !u.Admin {
		watched, err := s.watchlog.AnimeWatched(r.Context(), id)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		if watched {
			writeError(w, http.StatusForbidden, "Someone has watched this show: only admins can remove it.")
			return
		}
	}
	err := s.watchlog.DeleteAnime(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "That show is not on the watchlog.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteAlias forgets which show a name means, for one linked by mistake:
// what plays under that name is matched anew. Admins only.
func (s *Server) deleteAlias(w http.ResponseWriter, r *http.Request) {
	u := s.watchlogUser(w, r)
	if u == nil {
		return
	}
	if !u.Admin {
		writeError(w, http.StatusForbidden, "Only admins can unlink a name.")
		return
	}
	err := s.watchlog.DeleteAlias(r.Context(), r.PathValue("alias"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "That name is not linked to a show.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// setProgress marks episodes of a show as watched, or as not: your own, or
// anyone's as an admin. The episodes from "from" to "to"; "to" may be left
// out for one episode.
func (s *Server) setProgress(w http.ResponseWriter, r *http.Request) {
	u := s.watchlogUser(w, r)
	if u == nil {
		return
	}
	var req struct {
		From    int  `json:"from"`
		To      int  `json:"to"`
		Watched bool `json:"watched"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.To == 0 {
		req.To = req.From
	}
	target := u
	if name := strings.ToLower(r.PathValue("username")); name != u.Username {
		if !u.Admin {
			writeError(w, http.StatusForbidden, "You can only change your own progress.")
			return
		}
		if target = s.userByPath(w, r); target == nil {
			return
		}
	}
	id, _ := strconv.ParseInt(r.PathValue("anime"), 10, 64)
	a, err := s.watchlog.Anime(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "That show is not on the watchlog.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	// Unticking may reach past the show's end: its length can have changed.
	last := maxEpisode
	if req.Watched && a.Episodes != nil {
		last = *a.Episodes
	}
	if req.From < 1 || req.To < req.From || req.To > last {
		writeError(w, http.StatusBadRequest, "The show has no such episode.")
		return
	}
	err = s.watchlog.SetWatched(r.Context(), target.ID, a.ID, req.From, req.To, req.Watched, "manual")
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "That show is not on the watchlog.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	watched, err := s.watchlog.Watched(r.Context(), target.ID, a.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, store.Progress{Username: target.Username, AnimeID: a.ID, Watched: watched})
}

// setFollow says how someone follows a show: yourself, or anyone as an
// admin. The others wait for who is following; not for who watches along
// casually or has dropped it. What they watched stays.
func (s *Server) setFollow(w http.ResponseWriter, r *http.Request) {
	u := s.watchlogUser(w, r)
	if u == nil {
		return
	}
	var req struct {
		Mode string `json:"mode"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Mode != store.FollowFollowing && req.Mode != store.FollowCasual && req.Mode != store.FollowDropped {
		writeError(w, http.StatusBadRequest, "The mode is following, casual or dropped.")
		return
	}
	target := u
	if name := strings.ToLower(r.PathValue("username")); name != u.Username {
		if !u.Admin {
			writeError(w, http.StatusForbidden, "You can only change your own progress.")
			return
		}
		if target = s.userByPath(w, r); target == nil {
			return
		}
	}
	id, _ := strconv.ParseInt(r.PathValue("anime"), 10, 64)
	err := s.watchlog.SetFollow(r.Context(), target.ID, id, req.Mode)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "That show is not on the watchlog.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
