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

// Ignored is an account that is not following a show any more.
type Ignored struct {
	UserID   int64  `json:"-"`
	Username string `json:"username"` // as in Progress
	AnimeID  int64  `json:"animeId"`
}

const animeColumns = "id, title, title_english, episodes, cover_url, added_at"

func scanAnime(row interface{ Scan(...any) error }) (*Anime, error) {
	var a Anime
	var episodes sql.NullInt64
	err := row.Scan(&a.ID, &a.Title, &a.TitleEnglish, &episodes, &a.CoverURL, &a.AddedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if episodes.Valid {
		n := int(episodes.Int64)
		a.Episodes = &n
	}
	return &a, err
}

// SaveAnime adds a show, or updates what AniList says about it. AddedAt is
// filled in.
func (s *Watchlog) SaveAnime(ctx context.Context, a *Anime) error {
	return s.db.QueryRowContext(ctx,
		`INSERT INTO anime (id, title, title_english, episodes, cover_url, added_at) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (id) DO UPDATE SET title = excluded.title, title_english = excluded.title_english,
		   episodes = excluded.episodes, cover_url = excluded.cover_url
		 RETURNING added_at`,
		a.ID, a.Title, a.TitleEnglish, a.Episodes, a.CoverURL, s.unix()).Scan(&a.AddedAt)
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

// SetIgnored says whether an account has stopped following a show.
// ErrNotFound if the show is not on the watchlog.
func (s *Watchlog) SetIgnored(ctx context.Context, userID, animeID int64, ignored bool) error {
	if !ignored {
		_, err := s.db.ExecContext(ctx, "DELETE FROM watch_ignored WHERE user_id = ? AND anime_id = ?", userID, animeID)
		return err
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO watch_ignored (user_id, anime_id) VALUES (?, ?) ON CONFLICT (user_id, anime_id) DO NOTHING", userID, animeID)
	if isForeignKeyViolation(err) {
		return ErrNotFound
	}
	return err
}

// ListIgnored returns who has stopped following which show.
func (s *Watchlog) ListIgnored(ctx context.Context) ([]Ignored, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT user_id, anime_id FROM watch_ignored ORDER BY anime_id, user_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Ignored{}
	for rows.Next() {
		var i Ignored
		if err := rows.Scan(&i.UserID, &i.AnimeID); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// DeleteUser removes what a deleted account watched and ignored.
func (s *Watchlog) DeleteUser(ctx context.Context, id int64) error {
	return inTx(ctx, s.db, func(tx *sql.Tx) error {
		for _, table := range []string{"watched_episodes", "watch_ignored"} {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE user_id = ?", id); err != nil {
				return err
			}
		}
		return nil
	})
}
