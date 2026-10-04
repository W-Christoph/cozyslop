package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

var ErrUsernameTaken = errors.New("username already taken")

type User struct {
	ID           int64  `json:"-"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Nickname     string `json:"nickname"`
	NameColor    string `json:"nameColor"`
	Avatar       string `json:"-"`
	Admin        bool   `json:"admin"`
	Verified     bool   `json:"verified"`
	Disabled     bool   `json:"disabled"`
	CreatedAt    int64  `json:"createdAt"`
}

const userColumns = "id, username, password_hash, nickname, name_color, avatar, admin, verified, disabled, created_at"

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Nickname, &u.NameColor,
		&u.Avatar, &u.Admin, &u.Verified, &u.Disabled, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &u, err
}

// CreateUser inserts u. Username is stored lowercase; ID and CreatedAt are
// filled in.
func (s *Store) CreateUser(ctx context.Context, u *User) error {
	return s.createUser(ctx, s.db, u)
}

func (s *Store) createUser(ctx context.Context, db interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, u *User) error {
	u.Username = strings.ToLower(u.Username)
	u.CreatedAt = s.unix()
	if u.NameColor == "" {
		u.NameColor = "#fff"
	}
	res, err := db.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, nickname, name_color, avatar, admin, verified, disabled, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.Username, u.PasswordHash, u.Nickname, u.NameColor, u.Avatar,
		u.Admin, u.Verified, u.Disabled, u.CreatedAt)
	if isUniqueViolation(err) {
		return ErrUsernameTaken
	}
	if err != nil {
		return err
	}
	u.ID, err = res.LastInsertId()
	return err
}

func (s *Store) UserByID(ctx context.Context, id int64) (*User, error) {
	return scanUser(s.db.QueryRowContext(ctx, "SELECT "+userColumns+" FROM users WHERE id = ?", id))
}

func (s *Store) UserByUsername(ctx context.Context, username string) (*User, error) {
	return scanUser(s.db.QueryRowContext(ctx,
		"SELECT "+userColumns+" FROM users WHERE username = ?", strings.ToLower(username)))
}

func (s *Store) ListUsers(ctx context.Context) ([]*User, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+userColumns+" FROM users ORDER BY username")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE admin = 1 AND disabled = 0").Scan(&n)
	return n, err
}

// UpdateProfile changes the user-editable profile fields.
func (s *Store) UpdateProfile(ctx context.Context, id int64, nickname, nameColor string) error {
	return s.execOne(ctx, "UPDATE users SET nickname = ?, name_color = ? WHERE id = ?", nickname, nameColor, id)
}

func (s *Store) UpdateAvatar(ctx context.Context, id int64, avatar string) error {
	return s.execOne(ctx, "UPDATE users SET avatar = ? WHERE id = ?", avatar, id)
}

// AvatarReferenced reports whether any account still uses the avatar file.
func (s *Store) AvatarReferenced(ctx context.Context, name string) (bool, error) {
	var used bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE avatar = ?)", name).Scan(&used)
	return used, err
}

func (s *Store) UpdatePassword(ctx context.Context, id int64, hash string) error {
	return s.execOne(ctx, "UPDATE users SET password_hash = ? WHERE id = ?", hash, id)
}

// ResetAdmin restores the admin account and revokes its sessions atomically.
// The caller validates and hashes the password before taking the write lock.
func (s *Store) ResetAdmin(ctx context.Context, hash string) (bool, error) {
	var created bool
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var id int64
		err := tx.QueryRowContext(ctx, "SELECT id FROM users WHERE username = 'admin'").Scan(&id)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			u := &User{Username: "admin", PasswordHash: hash, Nickname: "admin", Admin: true, Verified: true}
			if err := s.createUser(ctx, tx, u); err != nil {
				return err
			}
			id = u.ID
			created = true
		case err != nil:
			return err
		default:
			if _, err := tx.ExecContext(ctx, "UPDATE users SET password_hash = ?, admin = 1, disabled = 0 WHERE id = ?", hash, id); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", id)
		return err
	})
	return created, err
}

// UpdateFlags sets the admin-controlled account flags.
func (s *Store) UpdateFlags(ctx context.Context, id int64, admin, verified, disabled bool) error {
	return s.execOne(ctx, "UPDATE users SET admin = ?, verified = ?, disabled = ? WHERE id = ?",
		admin, verified, disabled, id)
}

// DeleteUser removes the account; sessions and room permissions go with it,
// chat messages keep their nickname snapshot.
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	return s.execOne(ctx, "DELETE FROM users WHERE id = ?", id)
}

// execOne runs a statement that must affect exactly one row.
func (s *Store) execOne(ctx context.Context, query string, args ...any) error {
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var se *sqlite.Error
	return errors.As(err, &se) &&
		(se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE || se.Code() == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY)
}
