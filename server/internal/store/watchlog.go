package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Watchlog is the watchlog's database: its own file beside the main one, so
// that a server without the watchlog (an older version too) runs on the
// main one unchanged. Accounts are the main database's, known here by id
// only; what is left of a deleted one is removed with DeleteUser.
type Watchlog struct {
	db  *sql.DB
	now func() time.Time
}

// OpenWatchlog opens (creating if needed) the watchlog's database at path
// and applies pending migrations. Use ":memory:" for tests.
func OpenWatchlog(ctx context.Context, path string) (*Watchlog, error) {
	db, err := open(path)
	if err != nil {
		return nil, err
	}
	if err := migrate(ctx, db, watchlogMigrations, "migrations/watchlog/*.sql"); err != nil {
		db.Close()
		return nil, err
	}
	return &Watchlog{db: db, now: time.Now}, nil
}

func (s *Watchlog) Close() error { return s.db.Close() }

// ProbeWrite is Store.ProbeWrite for the watchlog's database.
func (s *Watchlog) ProbeWrite(ctx context.Context) error { return probeWrite(ctx, s.db) }

// Snapshot writes a copy of the database to path, as Store.Snapshot does.
func (s *Watchlog) Snapshot(ctx context.Context, path string) error {
	_, err := s.db.ExecContext(ctx, "VACUUM INTO ?", path)
	return err
}

func (s *Watchlog) unix() int64 { return s.now().Unix() }

// Anime is a show on the watchlog, as AniList describes it.
type Anime struct {
	ID           int64  `json:"id"` // AniList media id
	Title        string `json:"title"`
	TitleEnglish string `json:"titleEnglish"`
	Episodes     *int   `json:"episodes"` // nil while unknown (still airing)
	CoverURL     string `json:"coverUrl"`
	AddedAt      int64  `json:"addedAt"`
	// WatchURL is where the show is watched: an episode's page with "{ep}"
	// for its number (SetWatchURL); "" if nobody knows. SaveAnime leaves it.
	WatchURL string `json:"watchUrl"`
	// PageURL is the page the latest episode played on in a room's Firefox,
	// and PageEpisode which one (SetPage); "" and 0 if none has. Sites whose
	// addresses do not say the episode have only this. SaveAnime leaves it.
	PageURL     string `json:"pageUrl"`
	PageEpisode int    `json:"pageEpisode"`
	// The next episode to air and when (unix seconds); nil while AniList
	// announces none.
	NextEpisode  *int   `json:"nextEpisode"`
	NextAiringAt *int64 `json:"nextAiringAt"`
}

// Progress is the episodes of a show an account has watched.
type Progress struct {
	UserID   int64  `json:"-"`
	Username string `json:"username"` // filled in by who knows the accounts
	AnimeID  int64  `json:"animeId"`
	// Watched is the episodes as ranges, in order: 1 to 4 and 6 is
	// [[1,4],[6,6]].
	Watched [][2]int `json:"watched"`
	// WatchedAt is when they watched one last (unix seconds).
	WatchedAt int64 `json:"watchedAt"`
}

// How someone follows a show. Following is the usual way and is not stored.
const (
	FollowFollowing = "following" // the others wait for them
	FollowCasual    = "casual"    // they watch along when they are there; nobody waits
	FollowDropped   = "dropped"   // they have stopped
)

// Follow is how an account follows a show, where it is not the usual way.
type Follow struct {
	UserID   int64  `json:"-"`
	Username string `json:"username"` // as in Progress
	AnimeID  int64  `json:"animeId"`
	Mode     string `json:"mode"` // casual | dropped
}

const animeColumns = "id, title, title_english, episodes, cover_url, added_at, watch_url, page_url, page_episode, next_episode, next_airing_at"

func scanAnime(row interface{ Scan(...any) error }) (*Anime, error) {
	var a Anime
	var episodes, next, airing sql.NullInt64
	err := row.Scan(&a.ID, &a.Title, &a.TitleEnglish, &episodes, &a.CoverURL, &a.AddedAt, &a.WatchURL, &a.PageURL, &a.PageEpisode, &next, &airing)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if episodes.Valid {
		n := int(episodes.Int64)
		a.Episodes = &n
	}
	if next.Valid && airing.Valid {
		n := int(next.Int64)
		a.NextEpisode, a.NextAiringAt = &n, &airing.Int64
	}
	return &a, err
}

// SaveAnime adds a show, or updates what AniList says about it. AddedAt,
// WatchURL and the page are filled in.
func (s *Watchlog) SaveAnime(ctx context.Context, a *Anime) error {
	return s.db.QueryRowContext(ctx,
		`INSERT INTO anime (id, title, title_english, episodes, cover_url, added_at, next_episode, next_airing_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (id) DO UPDATE SET title = excluded.title, title_english = excluded.title_english,
		   episodes = excluded.episodes, cover_url = excluded.cover_url,
		   next_episode = excluded.next_episode, next_airing_at = excluded.next_airing_at
		 RETURNING added_at, watch_url, page_url, page_episode`,
		a.ID, a.Title, a.TitleEnglish, a.Episodes, a.CoverURL, s.unix(), a.NextEpisode, a.NextAiringAt).Scan(&a.AddedAt, &a.WatchURL, &a.PageURL, &a.PageEpisode)
}

// SetWatchURL says where a show is watched: an episode's page with "{ep}"
// for its number, or "" for nowhere known. ErrNotFound if the show is not
// on the watchlog.
func (s *Watchlog) SetWatchURL(ctx context.Context, id int64, url string) error {
	return execOne(ctx, s.db, "UPDATE anime SET watch_url = ? WHERE id = ?", url, id)
}

// SetPage says which page episode n of a show played on last. ErrNotFound
// if the show is not on the watchlog.
func (s *Watchlog) SetPage(ctx context.Context, id int64, url string, n int) error {
	return execOne(ctx, s.db, "UPDATE anime SET page_url = ?, page_episode = ? WHERE id = ?", url, n, id)
}

// AiringAnime returns the shows AniList may have news of: those whose
// length is not known, or that have an episode still to air.
func (s *Watchlog) AiringAnime(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id FROM anime WHERE episodes IS NULL OR next_episode IS NOT NULL ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Watchlog) Anime(ctx context.Context, id int64) (*Anime, error) {
	return scanAnime(s.db.QueryRowContext(ctx, "SELECT "+animeColumns+" FROM anime WHERE id = ?", id))
}

// ListAnime returns the shows on the watchlog, by title.
func (s *Watchlog) ListAnime(ctx context.Context) ([]*Anime, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+animeColumns+" FROM anime ORDER BY title COLLATE NOCASE")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Anime{}
	for rows.Next() {
		a, err := scanAnime(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteAnime removes a show with everyone's progress and its aliases.
func (s *Watchlog) DeleteAnime(ctx context.Context, id int64) error {
	return execOne(ctx, s.db, "DELETE FROM anime WHERE id = ?", id)
}

// AnimeWatched reports whether anyone has watched an episode of a show.
func (s *Watchlog) AnimeWatched(ctx context.Context, id int64) (bool, error) {
	var watched bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM watched_episodes WHERE anime_id = ?)", id).Scan(&watched)
	return watched, err
}

// AnimeByAlias returns the show a file name's title was matched to before.
func (s *Watchlog) AnimeByAlias(ctx context.Context, alias string) (*Anime, error) {
	return scanAnime(s.db.QueryRowContext(ctx,
		"SELECT "+animeColumns+" FROM anime WHERE id = (SELECT anime_id FROM anime_aliases WHERE alias = ?)", alias))
}

// SaveAlias remembers that a file name's title means the show animeID.
func (s *Watchlog) SaveAlias(ctx context.Context, alias string, animeID int64) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO anime_aliases (alias, anime_id) VALUES (?, ?) ON CONFLICT (alias) DO UPDATE SET anime_id = excluded.anime_id",
		alias, animeID)
	return err
}

// Alias is a name that means a show: the title part of a file's or a
// page's name, normalised.
type Alias struct {
	Alias   string `json:"alias"`
	AnimeID int64  `json:"animeId"`
}

// ListAliases returns the names matched to shows, by show and name.
func (s *Watchlog) ListAliases(ctx context.Context) ([]Alias, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT alias, anime_id FROM anime_aliases ORDER BY anime_id, alias")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Alias{}
	for rows.Next() {
		var a Alias
		if err := rows.Scan(&a.Alias, &a.AnimeID); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteAlias forgets which show a name means. ErrNotFound if it meant none.
func (s *Watchlog) DeleteAlias(ctx context.Context, alias string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM anime_aliases WHERE alias = ?", alias)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListProgress returns what everyone has watched, by show and account.
func (s *Watchlog) ListProgress(ctx context.Context) ([]Progress, error) {
	return s.progress(ctx, "")
}

// Watched returns the episodes of a show an account has watched, as
// ranges; none if nothing.
func (s *Watchlog) Watched(ctx context.Context, userID, animeID int64) ([][2]int, error) {
	list, err := s.progress(ctx, "WHERE w.user_id = ? AND w.anime_id = ?", userID, animeID)
	if err != nil || len(list) == 0 {
		return [][2]int{}, err
	}
	return list[0].Watched, nil
}

func (s *Watchlog) progress(ctx context.Context, where string, args ...any) ([]Progress, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT w.user_id, w.anime_id, w.episode, w.watched_at
		 FROM watched_episodes w `+where+` ORDER BY w.anime_id, w.user_id, w.episode`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Progress{}
	for rows.Next() {
		var p Progress
		var episode int
		var at int64
		if err := rows.Scan(&p.UserID, &p.AnimeID, &episode, &at); err != nil {
			return nil, err
		}
		if n := len(out); n == 0 || out[n-1].UserID != p.UserID || out[n-1].AnimeID != p.AnimeID {
			out = append(out, p)
		}
		// Episodes come in order: the next one lengthens the last range.
		last := &out[len(out)-1]
		last.WatchedAt = max(last.WatchedAt, at)
		if n := len(last.Watched); n > 0 && last.Watched[n-1][1] == episode-1 {
			last.Watched[n-1][1] = episode
		} else {
			last.Watched = append(last.Watched, [2]int{episode, episode})
		}
	}
	return out, rows.Err()
}

// SetWatched marks the episodes from first to last of a show as watched
// by an account, or as not. ErrNotFound if the show is not on the watchlog.
func (s *Watchlog) SetWatched(ctx context.Context, userID, animeID int64, first, last int, watched bool, source string) error {
	if !watched {
		_, err := s.db.ExecContext(ctx,
			"DELETE FROM watched_episodes WHERE user_id = ? AND anime_id = ? AND episode BETWEEN ? AND ?", userID, animeID, first, last)
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`WITH RECURSIVE numbers(n) AS (SELECT ? UNION ALL SELECT n + 1 FROM numbers WHERE n < ?)
		 INSERT INTO watched_episodes (user_id, anime_id, episode, source, watched_at)
		 SELECT ?, ?, n, ?, ? FROM numbers WHERE true
		 ON CONFLICT (user_id, anime_id, episode) DO NOTHING`,
		first, last, userID, animeID, source, s.unix())
	if isForeignKeyViolation(err) {
		return ErrNotFound
	}
	return err
}

// MarkWatched records that the accounts watched an episode of a show in a
// room. What they had watched before stays as it is. ErrNotFound if the
// show is not on the watchlog.
func (s *Watchlog) MarkWatched(ctx context.Context, userIDs []int64, animeID int64, episode int) error {
	now := s.unix()
	return inTx(ctx, s.db, func(tx *sql.Tx) error {
		for _, id := range userIDs {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO watched_episodes (user_id, anime_id, episode, source, watched_at) VALUES (?, ?, ?, 'auto', ?)
				 ON CONFLICT (user_id, anime_id, episode) DO NOTHING`,
				id, animeID, episode, now)
			if isForeignKeyViolation(err) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// SetFollow says how an account follows a show: one of the Follow modes.
// ErrNotFound if the show is not on the watchlog.
func (s *Watchlog) SetFollow(ctx context.Context, userID, animeID int64, mode string) error {
	if mode == FollowFollowing {
		_, err := s.db.ExecContext(ctx, "DELETE FROM watch_follow WHERE user_id = ? AND anime_id = ?", userID, animeID)
		return err
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO watch_follow (user_id, anime_id, mode) VALUES (?, ?, ?) ON CONFLICT (user_id, anime_id) DO UPDATE SET mode = excluded.mode",
		userID, animeID, mode)
	if isForeignKeyViolation(err) {
		return ErrNotFound
	}
	return err
}

// ListFollows returns who follows which show casually or has dropped it.
func (s *Watchlog) ListFollows(ctx context.Context) ([]Follow, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT user_id, anime_id, mode FROM watch_follow ORDER BY anime_id, user_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Follow{}
	for rows.Next() {
		var f Follow
		if err := rows.Scan(&f.UserID, &f.AnimeID, &f.Mode); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// DeleteUser removes what a deleted account watched and how it followed.
func (s *Watchlog) DeleteUser(ctx context.Context, id int64) error {
	return inTx(ctx, s.db, func(tx *sql.Tx) error {
		for _, table := range []string{"watched_episodes", "watch_follow"} {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE user_id = ?", id); err != nil {
				return err
			}
		}
		return nil
	})
}
