package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var ErrImportNotEmpty = errors.New("legacy import requires a database with no users")

// ImportData holds validated legacy rows. Permissions identify users by
// Username; ImportLegacy fills in the new numeric user IDs.
type ImportData struct {
	Users       []User
	Rooms       []RoomSettings
	Permissions []Permission
	Invites     []Invite
}

func (s *Store) HasUsers(ctx context.Context) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM users)").Scan(&exists)
	return exists, err
}

// ImportLegacy inserts all rows atomically and returns username -> user ID.
// The emptiness check is inside the write transaction to prevent races.
func (s *Store) ImportLegacy(ctx context.Context, data ImportData) (map[string]int64, error) {
	ids := make(map[string]int64, len(data.Users))
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM users)").Scan(&exists); err != nil {
			return err
		}
		if exists {
			return ErrImportNotEmpty
		}
		for _, u := range data.Users {
			if err := s.createUser(ctx, tx, &u); err != nil {
				return fmt.Errorf("import user %q: %w", u.Username, err)
			}
			ids[u.Username] = u.ID
		}
		for _, r := range data.Rooms {
			_, err := tx.ExecContext(ctx,
				"INSERT INTO rooms ("+roomColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
				r.Name, r.Access, r.Hidden, r.RemoteOwnership, r.CenterRemote, r.DefaultRemote, r.DefaultImage, r.DefaultUpload,
				r.Screen, r.Stream)
			if err != nil {
				return fmt.Errorf("import room %q: %w", r.Name, err)
			}
		}
		for _, p := range data.Permissions {
			id, ok := ids[strings.ToLower(p.Username)]
			if !ok {
				return fmt.Errorf("import permission: unknown user %q", p.Username)
			}
			_, err := tx.ExecContext(ctx,
				"INSERT INTO room_permissions ("+permissionColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
				p.Room, id, p.Remote, p.Image, p.Upload, p.Trusted, p.Invited, p.InviteName, p.Banned, p.BannedUntil)
			if err != nil {
				return fmt.Errorf("import permission for %q in %q: %w", p.Username, p.Room, err)
			}
		}
		for _, i := range data.Invites {
			if err := insertLegacyInvite(ctx, tx, i); err != nil {
				return fmt.Errorf("import invite: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// insertLegacyInvite preserves the old code, uses and creation time.
func insertLegacyInvite(ctx context.Context, tx *sql.Tx, i Invite) error {
	_, err := tx.ExecContext(ctx,
		"INSERT INTO invites ("+inviteColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		i.Code, i.Room, i.Temporary, i.Name, i.Remote, i.Image, i.Upload, i.Uses, i.MaxUses, i.ExpiresAt, i.CreatedAt)
	return err
}
