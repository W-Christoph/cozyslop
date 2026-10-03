package store

import (
	"context"
	"database/sql"
	"errors"
)

type Permission struct {
	Room        string `json:"room"`
	UserID      int64  `json:"-"`
	Username    string `json:"username"` // filled by queries that join users
	Remote      bool   `json:"remote"`
	Image       bool   `json:"image"`
	Upload      bool   `json:"upload"`
	Trusted     bool   `json:"trusted"`
	Invited     bool   `json:"invited"`
	InviteName  string `json:"inviteName"`
	Banned      bool   `json:"banned"`
	BannedUntil *int64 `json:"bannedUntil"` // nil with Banned = forever
}

const permissionColumns = "room, user_id, remote, image, upload, trusted, invited, invite_name, banned, banned_until"

func scanPermission(row interface{ Scan(...any) error }) (Permission, error) {
	var p Permission
	err := row.Scan(&p.Room, &p.UserID, &p.Remote, &p.Image, &p.Upload, &p.Trusted,
		&p.Invited, &p.InviteName, &p.Banned, &p.BannedUntil, &p.Username)
	return p, err
}

// Permission returns the stored permission, or an empty one for this user and room.
func (s *Store) Permission(ctx context.Context, room string, userID int64) (Permission, error) {
	p, err := scanPermission(s.db.QueryRowContext(ctx,
		"SELECT "+permissionColumns+", '' FROM room_permissions WHERE room = ? AND user_id = ?", room, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return Permission{Room: room, UserID: userID}, nil
	}
	return p, err
}

// ListPermissions lists one room's permissions, or all rooms if room is empty.
func (s *Store) ListPermissions(ctx context.Context, room string) ([]Permission, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+prefixed("p.", permissionColumns)+`, u.username
		 FROM room_permissions p JOIN users u ON u.id = p.user_id
		 WHERE (? = '' OR p.room = ?) ORDER BY p.room, u.username`, room, room)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var permissions []Permission
	for rows.Next() {
		p, err := scanPermission(rows)
		if err != nil {
			return nil, err
		}
		permissions = append(permissions, p)
	}
	return permissions, rows.Err()
}

func (s *Store) SavePermission(ctx context.Context, p Permission) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO room_permissions (`+permissionColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (room, user_id) DO UPDATE SET remote = excluded.remote, image = excluded.image,
		 upload = excluded.upload, trusted = excluded.trusted, invited = excluded.invited,
		 invite_name = excluded.invite_name, banned = excluded.banned, banned_until = excluded.banned_until`,
		p.Room, p.UserID, p.Remote, p.Image, p.Upload, p.Trusted, p.Invited, p.InviteName, p.Banned, p.BannedUntil)
	return err
}

func (s *Store) DeletePermission(ctx context.Context, room string, userID int64) error {
	return s.execOne(ctx, "DELETE FROM room_permissions WHERE room = ? AND user_id = ?", room, userID)
}

// BanUser sets a ban without changing the user's grants or trust.
func (s *Store) BanUser(ctx context.Context, room string, userID int64, until *int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO room_permissions (room, user_id, banned, banned_until) VALUES (?, ?, 1, ?)
		 ON CONFLICT (room, user_id) DO UPDATE SET banned = 1, banned_until = excluded.banned_until`,
		room, userID, until)
	return err
}
