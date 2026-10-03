package store

import (
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
