package store

import (
	"context"
	"database/sql"
	"errors"
)

type AnonBan struct {
	ID          int64  `json:"id"`
	Room        string `json:"room"`
	AnonID      string `json:"-"`
	IP          string `json:"ip"`
	BannedUntil *int64 `json:"bannedUntil"`
}

const anonBanColumns = "id, room, anon_id, ip, banned_until"

func scanAnonBan(row interface{ Scan(...any) error }) (*AnonBan, error) {
	var ban AnonBan
	err := row.Scan(&ban.ID, &ban.Room, &ban.AnonID, &ban.IP, &ban.BannedUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &ban, err
}

func (s *Store) AddAnonBan(ctx context.Context, room, anonID, ip string, until *int64) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO anon_bans (room, anon_id, ip, banned_until) VALUES (?, ?, ?, ?)", room, anonID, ip, until)
	return err
}

// AnonBan returns the longest active ban matching the anonymous ID or IP.
func (s *Store) AnonBan(ctx context.Context, room, anonID, ip string) (*AnonBan, error) {
	return scanAnonBan(s.db.QueryRowContext(ctx,
		`SELECT `+anonBanColumns+` FROM anon_bans
		 WHERE room = ? AND (anon_id = ? OR ip = ?) AND (banned_until IS NULL OR banned_until > ?)
		 ORDER BY banned_until IS NULL DESC, banned_until DESC, id LIMIT 1`, room, anonID, ip, s.unix()))
}

// ListAnonBans lists active bans in one room, or all rooms if room is empty.
func (s *Store) ListAnonBans(ctx context.Context, room string) ([]AnonBan, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+anonBanColumns+` FROM anon_bans
		 WHERE (? = '' OR room = ?) AND (banned_until IS NULL OR banned_until > ?)
		 ORDER BY room, id`, room, room, s.unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bans []AnonBan
	for rows.Next() {
		ban, err := scanAnonBan(rows)
		if err != nil {
			return nil, err
		}
		bans = append(bans, *ban)
	}
	return bans, rows.Err()
}

func (s *Store) DeleteAnonBan(ctx context.Context, id int64) error {
	return s.execOne(ctx, "DELETE FROM anon_bans WHERE id = ?", id)
}

func (s *Store) DeleteExpiredAnonBans(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM anon_bans WHERE banned_until <= ?", s.unix())
	return err
}
