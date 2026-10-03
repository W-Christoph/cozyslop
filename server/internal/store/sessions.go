package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Session lifetime: expires this long after last use.
const (
	SessionTTL = 30 * 24 * time.Hour
	// Sliding the expiry is a write; do it at most this often per session.
	sessionTouchInterval = time.Hour
)

func (s *Store) CreateSession(ctx context.Context, tokenHash []byte, userID int64) error {
	now := s.unix()
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO sessions (token_hash, user_id, created_at, expires_at, last_seen_at) VALUES (?, ?, ?, ?, ?)",
		tokenHash, userID, now, now+int64(SessionTTL.Seconds()), now)
	return err
}

// SessionUser returns the enabled user owning a valid session and slides
// the session's expiry.
func (s *Store) SessionUser(ctx context.Context, tokenHash []byte) (*User, error) {
	now := s.unix()
	var lastSeen int64
	row := s.db.QueryRowContext(ctx,
		`SELECT s.last_seen_at, `+prefixed("u.", userColumns)+`
		 FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = ? AND s.expires_at > ? AND u.disabled = 0`, tokenHash, now)
	var u User
	err := row.Scan(&lastSeen, &u.ID, &u.Username, &u.PasswordHash, &u.Nickname, &u.NameColor,
		&u.Avatar, &u.Admin, &u.Verified, &u.Disabled, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if now-lastSeen >= int64(sessionTouchInterval.Seconds()) {
		_, err = s.db.ExecContext(ctx, "UPDATE sessions SET last_seen_at = ?, expires_at = ? WHERE token_hash = ?",
			now, now+int64(SessionTTL.Seconds()), tokenHash)
		if err != nil {
			return nil, err
		}
	}
	return &u, nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash []byte) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash = ?", tokenHash)
	return err
}

// DeleteUserSessions logs a user out everywhere, optionally keeping one
// session (the one changing its password).
func (s *Store) DeleteUserSessions(ctx context.Context, userID int64, keep []byte) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ? AND token_hash IS NOT ?", userID, keep)
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= ?", s.unix())
	return err
}

// prefixed qualifies a column list with a table alias: "a, b" -> "u.a, u.b".
func prefixed(prefix, columns string) string {
	cols := strings.Split(columns, ", ")
	for i, c := range cols {
		cols[i] = prefix + c
	}
	return strings.Join(cols, ", ")
}
