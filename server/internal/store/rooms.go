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

// RegisteredRoom holds the connection details separately from room settings.
// Tokens must only be exposed when created or rotated.
type RegisteredRoom struct {
	Name      string
	NekoURL   string
	NekoToken string `json:"-"`
	CreatedBy *int64
	CreatedAt int64
}

var ErrRoomExists = errors.New("room already registered")

const registeredRoomColumns = "name, neko_url, neko_token, created_by, created_at"

func scanRegisteredRoom(row interface{ Scan(...any) error }) (RegisteredRoom, error) {
	var room RegisteredRoom
	err := row.Scan(&room.Name, &room.NekoURL, &room.NekoToken, &room.CreatedBy, &room.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return room, err
}

func (s *Store) RegisteredRooms(ctx context.Context) ([]RegisteredRoom, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+registeredRoomColumns+" FROM registered_rooms ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []RegisteredRoom{}
	for rows.Next() {
		room, err := scanRegisteredRoom(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, room)
	}
	return list, rows.Err()
}

func (s *Store) RegisteredRoom(ctx context.Context, name string) (RegisteredRoom, error) {
	return scanRegisteredRoom(s.db.QueryRowContext(ctx, "SELECT "+registeredRoomColumns+" FROM registered_rooms WHERE name = ?", name))
}

func (s *Store) CreateRegisteredRoom(ctx context.Context, room *RegisteredRoom) error {
	room.CreatedAt = s.unix()
	_, err := s.db.ExecContext(ctx, "INSERT INTO registered_rooms ("+registeredRoomColumns+") VALUES (?, ?, ?, ?, ?)",
		room.Name, room.NekoURL, room.NekoToken, room.CreatedBy, room.CreatedAt)
	if isUniqueViolation(err) {
		return ErrRoomExists
	}
	return err
}

func (s *Store) UpdateRegisteredRoom(ctx context.Context, room RegisteredRoom) error {
	res, err := s.db.ExecContext(ctx, "UPDATE registered_rooms SET neko_url = ?, neko_token = ? WHERE name = ?", room.NekoURL, room.NekoToken, room.Name)
	return registeredRoomResult(res, err)
}

// DeleteRegisteredRoom keeps settings, permissions and chat history.
func (s *Store) DeleteRegisteredRoom(ctx context.Context, name string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM registered_rooms WHERE name = ?", name)
	return registeredRoomResult(res, err)
}

func registeredRoomResult(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}
