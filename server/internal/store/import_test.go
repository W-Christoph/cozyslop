package store

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestImportLegacyRollback(t *testing.T) {
	ctx := context.Background()
	for _, failure := range []string{"user", "room", "permission", "invite"} {
		t.Run(failure, func(t *testing.T) {
			s := openTest(t)
			data := ImportData{
				Users:       []User{{Username: "Alice", PasswordHash: "hash", Nickname: "Alice"}},
				Rooms:       []RoomSettings{{Name: "main", Access: "invite"}},
				Permissions: []Permission{{Room: "main", Username: "alice", Remote: true}},
				Invites:     []Invite{{Code: "abc123", Room: "main", Uses: 2, CreatedAt: 123}},
			}
			switch failure {
			case "user":
				data.Users = append(data.Users, User{Username: "ALICE"}, User{Username: "bob"})
			case "room":
				data.Rooms = append(data.Rooms, RoomSettings{Name: "bad", Access: "invalid"}, RoomSettings{Name: "other", Access: "public"})
			case "permission":
				data.Permissions = append(data.Permissions, Permission{Room: "main", Username: "unknown"}, Permission{Room: "other", Username: "alice"})
			case "invite":
				data.Invites = append(data.Invites, Invite{Code: "abc123"}, Invite{Code: "other"})
			}
			if ids, err := s.ImportLegacy(ctx, data); err == nil || ids != nil {
				t.Fatalf("invalid import succeeded or returned IDs: err=%v ids=%v", err, ids)
			}
			for _, table := range []string{"users", "rooms", "room_permissions", "invites"} {
				var count int
				if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != 0 {
					t.Fatalf("rollback %s: err=%v count=%d", table, err, count)
				}
			}
		})
	}
}

func TestImportLegacy(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	data := ImportData{
		Users:       []User{{Username: "Alice", PasswordHash: "hash", Nickname: "Alice"}},
		Rooms:       []RoomSettings{{Name: "main", Access: "invite"}},
		Permissions: []Permission{{Room: "main", Username: "ALICE", Remote: true}},
		Invites:     []Invite{{Code: "0abc", Room: "main", Uses: 2, CreatedAt: 123}},
	}
	ids, err := s.ImportLegacy(ctx, data)
	if err != nil || ids["alice"] == 0 {
		t.Fatalf("import: err=%v ids=%v", err, ids)
	}
	if data.Users[0].ID != 0 || data.Users[0].Username != "Alice" {
		t.Fatal("import mutated caller's user rows")
	}
	if p, err := s.Permission(ctx, "main", ids["alice"]); err != nil || !p.Remote {
		t.Fatalf("mapped permission: %v %+v", err, p)
	}
	if i, err := s.Invite(ctx, "0abc"); err != nil || i.Code != "0abc" || i.Uses != 2 || i.CreatedAt != 123 {
		t.Fatalf("preserved invite: %v %+v", err, i)
	}
	if _, err := s.ImportLegacy(ctx, data); !errors.Is(err, ErrImportNotEmpty) {
		t.Fatalf("second import: %v", err)
	}
}

func TestLegacyLogins(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	hash := func(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }
	ids, err := s.ImportLegacy(ctx, ImportData{
		Users: []User{
			{Username: "alice", PasswordHash: "hash", Nickname: "alice"},
			{Username: "bob", PasswordHash: "hash", Nickname: "bob", Disabled: true},
			{Username: "carol", PasswordHash: "hash", Nickname: "carol"},
		},
		Logins: []LegacyLogin{
			{Username: "Alice", TokenHash: hash(1)}, {Username: "alice", TokenHash: hash(2)},
			{Username: "alice", TokenHash: hash(2)}, // the same token twice is one login
			{Username: "bob", TokenHash: hash(3)}, {Username: "carol", TokenHash: hash(4)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	redeem := func(want string, hashes ...[]byte) {
		t.Helper()
		u, err := s.RedeemLegacyLogin(ctx, hashes...)
		if want == "" {
			if !errors.Is(err, ErrNotFound) || u != nil {
				t.Fatalf("redeem: user=%+v err=%v, want not found", u, err)
			}
		} else if err != nil || u.Username != want {
			t.Fatalf("redeem: user=%+v err=%v, want %s", u, err, want)
		}
	}
	redeem("", hash(9))
	redeem("")
	// A login works once; the user's other logins stay.
	redeem("alice", hash(9), hash(1))
	redeem("", hash(1))
	// A disabled account cannot use its login, and keeps it for when it is enabled.
	redeem("", hash(3))
	if err := s.UpdateFlags(ctx, ids["bob"], false, false, false); err != nil {
		t.Fatal(err)
	}
	redeem("bob", hash(3))
	// Logging a user out everywhere ends the carried-over logins too.
	if err := s.DeleteUserSessions(ctx, ids["alice"], nil); err != nil {
		t.Fatal(err)
	}
	redeem("", hash(2))
	redeem("carol", hash(4))
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM legacy_logins").Scan(&count); err != nil || count != 0 {
		t.Fatalf("left over: %d err=%v", count, err)
	}

	if _, err := openTest(t).ImportLegacy(ctx, ImportData{Logins: []LegacyLogin{{Username: "nobody", TokenHash: hash(1)}}}); err == nil {
		t.Fatal("login of an unknown user imported")
	}
}
