package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
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

func previousSchema(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	for _, name := range []string{"0001_init.sql", "0002_room_stream.sql", "0003_room_stream_id.sql", "0004_stream_preset.sql"} {
		body, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(context.Background(), string(body)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("PRAGMA user_version = 4"); err != nil {
		t.Fatal(err)
	}
	return db
}

func snapshotRows(t *testing.T, db *sql.DB, query string) [][]any {
	t.Helper()
	rows, err := db.Query(query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var result [][]any
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		result = append(result, values)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestMigrateUserIDs(t *testing.T) {
	for _, orphan := range []bool{false, true} {
		name := "existing IDs"
		if orphan {
			name = "orphaned chat ID"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "old.db")
			db := previousSchema(t, path)
			_, err := db.Exec(`
				INSERT INTO users VALUES
				(3, 'alice', 'hash1', 'Alice', '#f90', 'alice.png', 1, 1, 0, 123),
				(9, 'bob', 'hash2', 'Bob', '#123456', 'bob.png', 0, 0, 1, 456);
				INSERT INTO sessions VALUES (x'0102', 3, 100, 200, 150), (x'0304', 9, 110, 220, 160);
				INSERT INTO rooms (name, access, screen, stream) VALUES ('main', 'invite', '1280x720@30', 'b4000-s100-veryfast');
				INSERT INTO room_permissions VALUES
				('main', 3, 1, 0, 1, 1, 1, 'friends', 0, NULL),
				('main', 9, 0, 1, 0, 0, 1, 'guests', 1, 999);
				INSERT INTO invites VALUES
				('one', 'main', 0, 'friends', 1, 0, 1, 2, 5, 999, 100),
				('two', 'main', 0, 'guests', 0, 1, 0, 1, NULL, NULL, 200);
				INSERT INTO invite_redemptions VALUES ('one', 3), ('one', 9), ('two', 9);
				INSERT INTO chat_messages VALUES
				(4, 'main', 'u:3', 3, 'Alice', '#f90', 0, 'text', 'hello', '', 1, 1000),
				(7, 'main', 'u:9', 9, 'Bob', '#123456', 0, 'image', '', 'photo.png', 0, 2000);
				INSERT INTO settings VALUES ('message', 'hello');
				INSERT INTO anon_bans VALUES (2, 'main', 'anon', '127.0.0.1', NULL);
				INSERT INTO chat_messages (room, identity, nickname, name_color, anonymous, type, created_at_ms)
				VALUES ('main', 'u:900junk', 'Gone', '#fff', 0, 'text', 3000),
				       ('main', 'a:999', 'Anon', '#fff', 1, 'text', 3001);`)
			if err != nil {
				t.Fatal(err)
			}
			wantSeq := int64(9)
			if orphan {
				if _, err := db.Exec(`INSERT INTO chat_messages
					(room, identity, nickname, name_color, anonymous, type, created_at_ms)
					VALUES ('main', 'u:21', 'Gone', '#fff', 0, 'text', 3002)`); err != nil {
					t.Fatal(err)
				}
				wantSeq = 21
			}
			queries := []string{"PRAGMA table_info(users)", "PRAGMA index_list(users)"}
			for _, table := range []string{"users", "sessions", "room_permissions", "invites", "invite_redemptions", "chat_messages", "settings", "anon_bans"} {
				queries = append(queries, "SELECT * FROM "+table+" ORDER BY 1, 2")
			}
			// A later migration drops a column of rooms; the others stay.
			queries = append(queries, "SELECT "+roomColumns+" FROM rooms ORDER BY 1, 2")
			for _, table := range []string{"sessions", "room_permissions", "invite_redemptions", "chat_messages"} {
				queries = append(queries, "PRAGMA foreign_key_list("+table+")", "PRAGMA index_list("+table+")")
			}
			before := make(map[string][][]any)
			for _, query := range queries {
				before[query] = snapshotRows(t, db, query)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			s, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			for _, query := range queries {
				if got := snapshotRows(t, s.db, query); !reflect.DeepEqual(got, before[query]) {
					t.Errorf("%s: got %v, want %v", query, got, before[query])
				}
			}
			if rows := snapshotRows(t, s.db, "PRAGMA foreign_key_check"); len(rows) != 0 {
				t.Fatalf("foreign key violations: %v", rows)
			}
			var seq, version, foreignKeys int64
			if err := s.db.QueryRow("SELECT seq FROM sqlite_sequence WHERE name = 'users'").Scan(&seq); err != nil || seq != wantSeq {
				t.Fatalf("sequence: %d want %d, err=%v", seq, wantSeq, err)
			}
			if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 9 {
				t.Fatalf("version: %d err=%v", version, err)
			}
			if err := s.db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil || foreignKeys != 1 {
				t.Fatalf("foreign keys not restored: %d err=%v", foreignKeys, err)
			}
			if err := s.DeleteUser(ctx, 9); err != nil {
				t.Fatal(err)
			}
			u := &User{Username: "carol", PasswordHash: "hash3", Nickname: "Carol"}
			if err := s.CreateUser(ctx, u); err != nil || u.ID != wantSeq+1 {
				t.Fatalf("new user: %+v err=%v", u, err)
			}
			if err := s.EditChat(ctx, "main", 7, fmt.Sprintf("u:%d", u.ID), "stolen"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("new account inherited edit rights: %v", err)
			}
			var userID *int64
			if err := s.db.QueryRow("SELECT user_id FROM chat_messages WHERE id = 7").Scan(&userID); err != nil || userID != nil {
				t.Fatalf("delete no longer clears chat reference: %v %v", userID, err)
			}
			for table, want := range map[string]int{"sessions": 1, "room_permissions": 1, "invite_redemptions": 1} {
				var count int
				if err := s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != want {
					t.Fatalf("%s delete cascade: count=%d err=%v", table, count, err)
				}
			}
			if err := s.DeleteUser(ctx, u.ID); err != nil {
				t.Fatal(err)
			}
			next := &User{Username: "dave", PasswordHash: "hash4", Nickname: "Dave"}
			if err := s.CreateUser(ctx, next); err != nil || next.ID != u.ID+1 {
				t.Fatalf("newest ID reused: %+v previous=%d err=%v", next, u.ID, err)
			}
		})
	}
}

func TestMigrateRejectsBrokenReferences(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")
	db := previousSchema(t, path)
	if _, err := db.Exec(`PRAGMA foreign_keys = OFF;
		INSERT INTO sessions VALUES (x'01', 99, 1, 2, 1)`); err != nil {
		t.Fatal(err)
	}
	before := snapshotRows(t, db, "SELECT * FROM sessions")
	db.Close()
	if s, err := Open(ctx, path); err == nil {
		s.Close()
		t.Fatal("migration accepted a broken reference")
	} else if !strings.Contains(err.Error(), "foreign_key_check") {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 4 {
		t.Fatalf("failed migration advanced version: %d %v", version, err)
	}
	var schema string
	if err := db.QueryRow("SELECT sql FROM sqlite_schema WHERE name = 'users'").Scan(&schema); err != nil || strings.Contains(schema, "AUTOINCREMENT") {
		t.Fatalf("failed migration changed users: %q %v", schema, err)
	}
	if got := snapshotRows(t, db, "SELECT * FROM sessions"); !reflect.DeepEqual(got, before) {
		t.Fatalf("failed migration changed sessions: %v", got)
	}
}
