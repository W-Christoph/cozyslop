package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"
)

var ErrInvalidPasswordReset = errors.New("invalid password reset")

const PasswordResetTTL = 24 * time.Hour

// CreatePasswordReset returns the secret once. Only its SHA-256 is stored.
func (s *Store) CreatePasswordReset(ctx context.Context, userID, createdBy int64) (string, int64, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", 0, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	hash := sha256.Sum256([]byte(token))
	now := s.unix()
	expires := now + int64(PasswordResetTTL.Seconds())
	err := s.tx(ctx, func(tx *sql.Tx) error {
		// Also clean up expired and used rows lazily on issuance.
		if _, err := tx.ExecContext(ctx, "DELETE FROM password_resets WHERE expires_at <= ? OR used_at IS NOT NULL OR user_id = ?", now, userID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			"INSERT INTO password_resets (token_hash, user_id, created_by, created_at, expires_at) VALUES (?, ?, ?, ?, ?)",
			hash[:], userID, createdBy, now, expires)
		return err
	})
	if err != nil {
		return "", 0, err
	}
	return token, expires, nil
}

// Lookup uses a fixed-size digest, never a prefix comparison of the secret.
func passwordResetHash(token string) ([32]byte, error) {
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(raw) != 32 || base64.RawURLEncoding.EncodeToString(raw) != token {
		return [32]byte{}, ErrInvalidPasswordReset
	}
	return sha256.Sum256([]byte(token)), nil
}

func (s *Store) PasswordResetUser(ctx context.Context, token string) (*User, error) {
	hash, err := passwordResetHash(token)
	if err != nil {
		return nil, err
	}
	u, err := scanUser(s.db.QueryRowContext(ctx,
		`SELECT `+prefixed("u.", userColumns)+` FROM password_resets p JOIN users u ON u.id = p.user_id
		 WHERE p.token_hash = ? AND p.used_at IS NULL AND p.expires_at > ?`, hash[:], s.unix()))
	if errors.Is(err, ErrNotFound) {
		return nil, ErrInvalidPasswordReset
	}
	return u, err
}

// RedeemPasswordReset changes the password, consumes the link and revokes
// all sessions (including legacy logins) in one transaction.
func (s *Store) RedeemPasswordReset(ctx context.Context, token, passwordHash string) (int64, error) {
	hash, err := passwordResetHash(token)
	if err != nil {
		return 0, err
	}
	var userID int64
	err = s.tx(ctx, func(tx *sql.Tx) error {
		now := s.unix()
		err := tx.QueryRowContext(ctx,
			"SELECT user_id FROM password_resets WHERE token_hash = ? AND used_at IS NULL AND expires_at > ?", hash[:], now).Scan(&userID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidPasswordReset
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE password_resets SET used_at = ? WHERE token_hash = ?", now, hash[:]); err != nil {
			return err
		}
		if err := s.updatePassword(ctx, tx, userID, passwordHash); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", userID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM legacy_logins WHERE user_id = ?", userID)
		return err
	})
	if err != nil {
		return 0, err
	}
	return userID, nil
}
