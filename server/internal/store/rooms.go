package store

import (
	"context"
	"database/sql"
	"errors"
)

type RoomSettings struct {
	Name            string `json:"name"`
	Access          string `json:"access"` // public | account | verified | invite
	Hidden          bool   `json:"hidden"`
	RemoteOwnership bool   `json:"remoteOwnership"`
	DefaultRemote   bool   `json:"defaultRemote"`
	DefaultImage    bool   `json:"defaultImage"`
	DefaultUpload   bool   `json:"defaultUpload"`
	Screen          string `json:"screen"` // "1280x720@30"; "" = the room container's default
	Stream          string `json:"stream"` // capture pipeline id, e.g. "b2500-s100-veryfast"; "" = neko's default
}

const roomColumns = "name, access, hidden, remote_ownership, default_remote, default_image, default_upload, screen, stream"

func scanRoomSettings(row interface{ Scan(...any) error }) (RoomSettings, error) {
	var set RoomSettings
	err := row.Scan(&set.Name, &set.Access, &set.Hidden, &set.RemoteOwnership,
		&set.DefaultRemote, &set.DefaultImage, &set.DefaultUpload, &set.Screen, &set.Stream)
	return set, err
}

// RoomSettings returns the room's settings, or defaults if none are stored.
func (s *Store) RoomSettings(ctx context.Context, name string) (RoomSettings, error) {
	set, err := scanRoomSettings(s.db.QueryRowContext(ctx, "SELECT "+roomColumns+" FROM rooms WHERE name = ?", name))
	if errors.Is(err, sql.ErrNoRows) {
		return RoomSettings{Name: name, Access: "public"}, nil
	}
	return set, err
}

func (s *Store) SaveRoomSettings(ctx context.Context, set RoomSettings) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO rooms (`+roomColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (name) DO UPDATE SET access = excluded.access, hidden = excluded.hidden,
		 remote_ownership = excluded.remote_ownership,
		 default_remote = excluded.default_remote, default_image = excluded.default_image,
		 default_upload = excluded.default_upload, screen = excluded.screen, stream = excluded.stream`,
		set.Name, set.Access, set.Hidden, set.RemoteOwnership,
		set.DefaultRemote, set.DefaultImage, set.DefaultUpload, set.Screen, set.Stream)
	return err
}

// ValidAccess reports whether s is a room access mode.
func ValidAccess(s string) bool {
	switch s {
	case "public", "account", "verified", "invite":
		return true
	}
	return false
}
