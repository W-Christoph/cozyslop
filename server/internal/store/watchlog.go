package store

import (
	"context"
	"database/sql"
	"errors"
)

// Anime is a show on the watchlog, as AniList describes it.
type Anime struct {
	ID           int64  `json:"id"` // AniList media id
	Title        string `json:"title"`
	TitleEnglish string `json:"titleEnglish"`
	Episodes     *int   `json:"episodes"` // nil while unknown (still airing)
	CoverURL     string `json:"coverUrl"`
	AddedAt      int64  `json:"addedAt"`
}

// Progress is the last episode of a show an account has watched.
type Progress struct {
	UserID    int64  `json:"-"`
	Username  string `json:"username"`
	AnimeID   int64  `json:"animeId"`
	Episode   int    `json:"episode"`
	Source    string `json:"source"` // manual | auto
	UpdatedAt int64  `json:"updatedAt"`
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
func (s *Store) SaveAnime(ctx context.Context, a *Anime) error {
	return s.db.QueryRowContext(ctx,
		`INSERT INTO anime (id, title, title_english, episodes, cover_url, added_at) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (id) DO UPDATE SET title = excluded.title, title_english = excluded.title_english,
		   episodes = excluded.episodes, cover_url = excluded.cover_url
		 RETURNING added_at`,
		a.ID, a.Title, a.TitleEnglish, a.Episodes, a.CoverURL, s.unix()).Scan(&a.AddedAt)
}

func (s *Store) Anime(ctx context.Context, id int64) (*Anime, error) {
	return scanAnime(s.db.QueryRowContext(ctx, "SELECT "+animeColumns+" FROM anime WHERE id = ?", id))
}

// ListAnime returns the shows on the watchlog, by title.
func (s *Store) ListAnime(ctx context.Context) ([]*Anime, error) {
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
func (s *Store) DeleteAnime(ctx context.Context, id int64) error {
	return s.execOne(ctx, "DELETE FROM anime WHERE id = ?", id)
}

// AnimeByAlias returns the show a file name's title was matched to before.
func (s *Store) AnimeByAlias(ctx context.Context, alias string) (*Anime, error) {
	return scanAnime(s.db.QueryRowContext(ctx,
		"SELECT "+animeColumns+" FROM anime WHERE id = (SELECT anime_id FROM anime_aliases WHERE alias = ?)", alias))
}

// SaveAlias remembers that a file name's title means the show animeID.
func (s *Store) SaveAlias(ctx context.Context, alias string, animeID int64) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO anime_aliases (alias, anime_id) VALUES (?, ?) ON CONFLICT (alias) DO UPDATE SET anime_id = excluded.anime_id",
		alias, animeID)
	return err
}

// ListProgress returns everyone's progress.
func (s *Store) ListProgress(ctx context.Context) ([]Progress, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT p.user_id, u.username, p.anime_id, p.episode, p.source, p.updated_at
		 FROM watch_progress p JOIN users u ON u.id = p.user_id ORDER BY p.anime_id, u.username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Progress{}
	for rows.Next() {
		var p Progress
		if err := rows.Scan(&p.UserID, &p.Username, &p.AnimeID, &p.Episode, &p.Source, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// WatchlogUsers returns the accounts with any progress, by username.
func (s *Store) WatchlogUsers(ctx context.Context) ([]*User, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+userColumns+" FROM users WHERE id IN (SELECT user_id FROM watch_progress) ORDER BY username")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// SetProgress sets how far an account has watched a show; episode 0
// removes it. ErrNotFound if the show is not on the watchlog.
func (s *Store) SetProgress(ctx context.Context, userID, animeID int64, episode int, source string) (*Progress, error) {
	if episode <= 0 {
		_, err := s.db.ExecContext(ctx, "DELETE FROM watch_progress WHERE user_id = ? AND anime_id = ?", userID, animeID)
		return nil, err
	}
	p := &Progress{UserID: userID, AnimeID: animeID, Episode: episode, Source: source, UpdatedAt: s.unix()}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO watch_progress (user_id, anime_id, episode, source, updated_at) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (user_id, anime_id) DO UPDATE SET episode = excluded.episode, source = excluded.source, updated_at = excluded.updated_at`,
		userID, animeID, episode, source, p.UpdatedAt)
	if isForeignKeyViolation(err) {
		return nil, ErrNotFound
	}
	return p, err
}

// RaiseProgress records that the accounts watched episode of a show, for
// those who were not further along already. It never lowers progress.
func (s *Store) RaiseProgress(ctx context.Context, userIDs []int64, animeID int64, episode int) error {
	now := s.unix()
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, id := range userIDs {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO watch_progress (user_id, anime_id, episode, source, updated_at) VALUES (?, ?, ?, 'auto', ?)
				 ON CONFLICT (user_id, anime_id) DO UPDATE SET episode = excluded.episode, source = 'auto', updated_at = excluded.updated_at
				 WHERE excluded.episode > watch_progress.episode`,
				id, animeID, episode, now)
			if isForeignKeyViolation(err) {
				continue // the account was deleted meanwhile
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
}
