package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// Migrations 0003 and 0004 turn the old quality presets into stream ids.
func TestMigrateQualityToStream(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")

	// Build a database at schema version 2 with rooms using each quality.
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"migrations/0001_init.sql", "migrations/0002_room_stream.sql"} {
		body, err := migrations.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, string(body)); err != nil {
			t.Fatal(err)
		}
	}
	_, err = db.ExecContext(ctx, `PRAGMA user_version = 2;
		INSERT INTO rooms (name, access, screen, quality) VALUES
		('h', 'invite', '1920x1080@30', 'high'), ('m', 'public', '', 'medium'), ('l', 'account', '', 'low')`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for name, want := range map[string]RoomSettings{
		"h": {Name: "h", Access: "invite", Screen: "1920x1080@30", Stream: "b4000-s100-veryfast"},
		"m": {Name: "m", Access: "public", Stream: ""},
		"l": {Name: "l", Access: "account", Stream: "b1000-s67-veryfast"},
	} {
		got, err := s.RoomSettings(ctx, name)
		if err != nil || got != want {
			t.Errorf("room %s: %v %+v, want %+v", name, err, got, want)
		}
	}
}
