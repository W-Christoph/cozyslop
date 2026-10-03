package legacy

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"cozycast/internal/store"

	"golang.org/x/crypto/bcrypt"
)

// Spring BCryptPasswordEncoder's $2a$ cost-10 output for "password".
const testHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

type entry struct {
	name string
	body []byte
	kind byte
}

func archive(t *testing.T, entries ...entry) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		kind := e.kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		h := &tar.Header{Name: e.name, Typeflag: kind, Mode: 0o644, Size: int64(len(e.body))}
		if kind == tar.TypeSymlink {
			h.Linkname = "../../escape"
			h.Size = 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(e.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "cozycast-export.tar.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func jsonEntry(t *testing.T, value any) entry {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return entry{name: "export.json", body: body}
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func pngImage(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func user(t *testing.T, st *store.Store, username string) *store.User {
	t.Helper()
	u, err := st.UserByUsername(context.Background(), username)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestImport(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	present := strings.Repeat("a", 64) + ".png"
	missing := strings.Repeat("b", 64) + ".jpg"
	badContent := strings.Repeat("c", 64) + ".webp"
	unreferenced := strings.Repeat("d", 64) + ".jpeg"
	past := "2020-07-19T10:15:00+00:00"
	future := "2099-07-19T10:15:00.123456+00:00"
	later := "2099-07-20T10:15:00+00:00"
	expiry, _ := time.Parse(time.RFC3339Nano, future)
	longBan, _ := time.Parse(time.RFC3339Nano, later)
	old := map[string]any{
		"format": "cozycast-export", "version": 1, "exportedAt": "2026-07-19T10:15:00.123456+00:00",
		"extra": "unknown fields are ignored",
		"users": []map[string]any{
			{"username": "Alice", "password": testHash, "enabled": true, "admin": true, "verified": true, "nickname": "Ali", "name_color": "#aBc123", "avatar_url": "/avatar/image/" + present, "email": "unused", "password_expired": true},
			{"username": "bob", "password": strings.Replace(testHash, "$2a$", "$2b$", 1), "enabled": false, "nickname": nil, "name_color": "bad", "avatar_url": "/avatar/image/" + missing},
			{"username": "locked", "password": strings.Replace(testHash, "$2a$", "$2y$", 1), "enabled": true, "account_locked": true, "name_color": "#AbC", "avatar_url": "/png/default_avatar.png"},
			{"username": "expired", "password": testHash, "enabled": true, "account_expired": true, "avatar_url": "/avatar/image/" + badContent},
			{"username": "defaults", "password": testHash, "enabled": nil, "verified": nil, "nickname": nil, "name_color": nil, "avatar_url": nil},
			{"username": "shared", "password": testHash, "enabled": true, "avatar_url": "/avatar/image/" + present},
			{"username": "invalid", "password": "plaintext", "enabled": true},
			{"username": "malformed", "password": "$2a$10$broken", "enabled": true},
		},
		"room_persistence": []map[string]any{
			{"name": "public", "center_remote": true, "default_remote_permission": true, "default_image_permission": true, "remote_ownership": true, "hidden_to_unauthorized": true, "desktop_width": 1280, "video_codec": "h264"},
			{"name": "account", "account_only": true},
			{"name": "verified", "account_only": true, "verified_only": true},
			{"name": "invite", "account_only": true, "verified_only": true, "invite_only": true},
		},
		"room_permission": []map[string]any{
			{"id": 1, "room": "public", "user_id": "Alice", "remote_permission": true, "banned": true, "banned_until": past, "invite_name": nil},
			{"id": 2, "room": "public", "user_id": "alice", "image_permission": true, "trusted": true, "invited": true, "invite_name": "friends", "banned": true, "banned_until": future},
			{"id": 3, "room": "public", "user_id": "ALICE", "banned": true, "banned_until": later},
			{"id": 4, "room": "public", "user_id": "alice", "banned": true, "banned_until": future},
			{"id": 5, "room": "invite", "user_id": "alice", "banned": true, "banned_until": future},
			{"id": 6, "room": "invite", "user_id": "alice", "banned": true, "banned_until": nil},
			{"id": 7, "room": "invite", "user_id": "alice", "banned": true, "banned_until": later},
			{"id": 8, "room": "account", "user_id": "bob", "banned": true, "banned_until": past},
			{"id": 9, "room": "verified", "user_id": "bob", "banned": true, "banned_until": future},
			{"id": 10, "room": "public", "user_id": "invalid", "trusted": true},
			{"id": 11, "room": "public", "user_id": "unknown", "remote_permission": true},
		},
		"room_invite": []map[string]any{
			{"id": "0abc123", "room": "public", "uses": 2, "max_uses": 3, "expiration": future, "remote_permission": true, "image_permission": true, "invite_name": "friends", "temporary": false},
			{"id": "expired", "room": "public", "expiration": past},
			{"id": "usedup", "room": "public", "uses": 3, "max_uses": 3},
			{"id": "unlimited", "room": "invite", "uses": 99, "max_uses": 0, "expiration": nil, "temporary": true, "invite_name": nil},
			{"id": "forever", "room": "account", "uses": 1, "max_uses": nil},
		},
	}
	picture := pngImage(t)
	path := archive(t,
		// Avatars can precede the JSON document.
		entry{name: "avatar/" + present, body: picture}, jsonEntry(t, old),
		entry{name: "../x.png", body: picture},
		entry{name: "avatar/sub/a.png", body: picture},
		entry{name: "avatar/sub/" + missing, body: picture},
		entry{name: "avatar/../" + missing, body: picture},
		entry{name: "avatar/wrong.png", body: picture},
		entry{name: "avatar/" + badContent, body: []byte("not an image")},
		entry{name: "avatar/" + unreferenced, body: picture},
		entry{name: "avatar/" + missing, kind: tar.TypeSymlink},
		entry{name: "other.txt", body: []byte("ignored")},
	)
	avatarDir := filepath.Join(t.TempDir(), "avatars")
	before := time.Now().Unix()
	summary, err := Import(ctx, st, path, avatarDir)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Users != 6 || summary.Rooms != 4 || summary.Permissions != 4 || summary.Invites != 3 || summary.Avatars != 1 {
		t.Fatalf("counts: %+v", summary)
	}
	wantSkipped := []string{
		"avatar " + badContent + ": unsupported image content",
		"user \"bob\": avatar " + missing + " missing or rejected",
		"user \"expired\": avatar " + badContent + " missing or rejected",
		"user \"invalid\": invalid bcrypt hash",
		"user \"malformed\": invalid bcrypt hash",
		"permission for \"invalid\" in \"public\": unknown user",
		"permission for \"unknown\" in \"public\": unknown user",
		"invite in \"public\": expired or used up",
		"invite in \"public\": expired or used up",
	}
	if !reflect.DeepEqual(summary.Skipped, wantSkipped) {
		t.Fatalf("skipped: %v, want %v", summary.Skipped, wantSkipped)
	}
	users, err := st.ListUsers(ctx)
	if err != nil || len(users) != summary.Users {
		t.Fatalf("users: %v count=%d", err, len(users))
	}
	alice := user(t, st, "alice")
	if alice.Username != "alice" || alice.Nickname != "Ali" || alice.NameColor != "#aBc123" || alice.Avatar != present || !alice.Admin || !alice.Verified || alice.Disabled || alice.PasswordHash != testHash {
		t.Fatalf("alice mapping: id=%d username=%s nickname=%s color=%s avatar=%s flags=%v/%v/%v", alice.ID, alice.Username, alice.Nickname, alice.NameColor, alice.Avatar, alice.Admin, alice.Verified, alice.Disabled)
	}
	// Hashes are stored exactly, including each supported bcrypt prefix.
	for _, name := range []string{"bob", "locked", "expired", "defaults"} {
		u := user(t, st, name)
		if !u.Disabled || u.Avatar != "" || u.Nickname != name {
			t.Fatalf("disabled/default user %s: disabled=%v avatar=%s nickname=%s", name, u.Disabled, u.Avatar, u.Nickname)
		}
	}
	if u := user(t, st, "bob"); u.NameColor != "#fff" || u.PasswordHash != strings.Replace(testHash, "$2a$", "$2b$", 1) {
		t.Fatal("bob color or password mapping")
	}
	if u := user(t, st, "locked"); u.NameColor != "#AbC" || u.PasswordHash != strings.Replace(testHash, "$2a$", "$2y$", 1) {
		t.Fatal("locked color or password mapping")
	}
	for _, name := range []string{"invalid", "malformed"} {
		if _, err := st.UserByUsername(ctx, name); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("invalid user exists: %s %v", name, err)
		}
	}
	for _, mode := range []string{"public", "account", "verified", "invite"} {
		r, err := st.RoomSettings(ctx, mode)
		if err != nil || r.Access != mode || r.DefaultUpload {
			t.Fatalf("room mapping: %v %+v", err, r)
		}
		if mode == "public" && (!r.Hidden || !r.RemoteOwnership || !r.CenterRemote || !r.DefaultRemote || !r.DefaultImage) {
			t.Fatalf("room settings missing: %+v", r)
		}
	}
	permissions, err := st.ListPermissions(ctx, "")
	if err != nil || len(permissions) != summary.Permissions {
		t.Fatalf("permissions: %v %+v", err, permissions)
	}
	p, err := st.Permission(ctx, "public", alice.ID)
	if err != nil || !p.Remote || !p.Image || !p.Trusted || !p.Invited || p.InviteName != "friends" || !p.Banned || p.BannedUntil == nil || *p.BannedUntil != longBan.Unix() || p.Upload {
		t.Fatalf("merged permission: %v %+v", err, p)
	}
	p, err = st.Permission(ctx, "invite", alice.ID)
	if err != nil || !p.Banned || p.BannedUntil != nil {
		t.Fatalf("forever ban: %v %+v", err, p)
	}
	bob := user(t, st, "bob")
	p, err = st.Permission(ctx, "account", bob.ID)
	if err != nil || p.Banned || p.BannedUntil != nil {
		t.Fatalf("expired ban: %v %+v", err, p)
	}
	p, err = st.Permission(ctx, "verified", bob.ID)
	if err != nil || !p.Banned || p.BannedUntil == nil || *p.BannedUntil != expiry.Unix() {
		t.Fatalf("future ban: %v %+v", err, p)
	}
	invites, err := st.ListInvites(ctx, "")
	if err != nil || len(invites) != summary.Invites {
		t.Fatalf("invites: %v count=%d", err, len(invites))
	}
	i, err := st.Invite(ctx, "0abc123")
	if err != nil || i.Code != "0abc123" || i.Uses != 2 || i.MaxUses == nil || *i.MaxUses != 3 || i.ExpiresAt == nil || *i.ExpiresAt != expiry.Unix() || !i.Remote || !i.Image || i.Upload || i.Temporary || i.Name != "friends" || i.CreatedAt < before || i.CreatedAt > time.Now().Unix() {
		t.Fatalf("invite: %v %+v", err, i)
	}
	i, err = st.Invite(ctx, "unlimited")
	if err != nil || i.MaxUses != nil || i.ExpiresAt != nil || i.Uses != 99 || !i.Temporary || i.Name != "" {
		t.Fatalf("unlimited temporary invite: %v %+v", err, i)
	}
	i, err = st.Invite(ctx, "forever")
	if err != nil || i.MaxUses != nil || i.ExpiresAt != nil || i.Uses != 1 {
		t.Fatalf("null limits: %v %+v", err, i)
	}
	for _, code := range []string{"expired", "usedup"} {
		if _, err := st.Invite(ctx, code); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("invalid invite exists: %s %v", code, err)
		}
	}
	files, err := os.ReadDir(avatarDir)
	if err != nil || len(files) != 1 || files[0].Name() != present {
		t.Fatalf("avatar files: %v %v", err, files)
	}
	body, err := os.ReadFile(filepath.Join(avatarDir, present))
	if err != nil || !bytes.Equal(body, picture) {
		t.Fatalf("avatar content: %v", err)
	}
	info, err := os.Stat(filepath.Join(avatarDir, present))
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("avatar mode: %v %v", err, info)
	}
	if user(t, st, "shared").Avatar != present {
		t.Fatal("shared avatar missing")
	}
	if _, err := Import(ctx, st, path, avatarDir); !errors.Is(err, ErrNotEmpty) {
		t.Fatalf("second import: %v", err)
	}
}

func TestUnknownFormatVersion(t *testing.T) {
	for _, tt := range []struct {
		format  string
		version int
	}{{"unknown", 1}, {"cozycast-export", 2}, {"cozycast-export", 0}} {
		t.Run(tt.format+string(rune('0'+tt.version)), func(t *testing.T) {
			st := openStore(t)
			path := archive(t, jsonEntry(t, map[string]any{
				"format": tt.format, "version": tt.version,
				"users": []map[string]any{{"username": "alice", "password": testHash}},
			}))
			if _, err := Import(context.Background(), st, path, t.TempDir()); err == nil {
				t.Fatal("unknown format/version accepted")
			}
			if exists, err := st.HasUsers(context.Background()); err != nil || exists {
				t.Fatalf("store changed: exists=%v err=%v", exists, err)
			}
		})
	}
}

func TestAvatarCopyFailure(t *testing.T) {
	for _, failure := range []string{"exists", "directory"} {
		t.Run(failure, func(t *testing.T) {
			st := openStore(t)
			name := strings.Repeat("e", 64) + ".png"
			old := export{Format: "cozycast-export", Version: 1, Users: []userRow{
				{Username: "alice", Password: testHash, Enabled: true, AvatarURL: "/avatar/image/" + name},
				{Username: "bob", Password: testHash, Enabled: true, AvatarURL: "/avatar/image/" + name},
			}}
			path := archive(t, jsonEntry(t, old), entry{name: "avatar/" + name, body: pngImage(t)})
			dir := t.TempDir()
			existing := filepath.Join(dir, name)
			if failure == "directory" {
				existing = filepath.Join(dir, "blocked")
				dir = existing
			}
			if err := os.WriteFile(existing, []byte("keep me"), 0o600); err != nil {
				t.Fatal(err)
			}
			summary, err := Import(context.Background(), st, path, dir)
			if err != nil || summary.Users != 2 || summary.Avatars != 0 || len(summary.Skipped) != 1 || !strings.Contains(summary.Skipped[0], "copy failed") {
				t.Fatalf("copy failure: err=%v summary=%+v", err, summary)
			}
			for _, username := range []string{"alice", "bob"} {
				if user(t, st, username).Avatar != "" {
					t.Fatal("failed copy left avatar reference")
				}
			}
			if body, err := os.ReadFile(existing); err != nil || string(body) != "keep me" {
				t.Fatalf("existing file changed: %v", err)
			}
		})
	}
}

func TestMalformedArchive(t *testing.T) {
	valid := jsonEntry(t, export{Format: "cozycast-export", Version: 1, Users: []userRow{{Username: "alice", Password: testHash}}})
	for _, tt := range []struct {
		name    string
		entries []entry
	}{
		{"missing JSON", []entry{{name: "ignored"}}},
		{"invalid JSON", []entry{{name: "export.json", body: []byte("{")}}},
		{"duplicate JSON", []entry{valid, valid}},
		{"oversized JSON", []entry{{name: "export.json", body: bytes.Repeat([]byte(" "), maxExport+1)}}},
		{"invalid timestamp", []entry{{name: "export.json", body: []byte(`{"format":"cozycast-export","version":1,"room_permission":[{"banned_until":"not a date"}]}`)}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			st := openStore(t)
			path := archive(t, tt.entries...)
			dir := t.TempDir()
			if _, err := Import(context.Background(), st, path, dir); err == nil {
				t.Fatal("malformed archive accepted")
			}
			if exists, err := st.HasUsers(context.Background()); err != nil || exists {
				t.Fatalf("store changed: %v exists=%v", err, exists)
			}
			if files, err := os.ReadDir(dir); err != nil || len(files) != 0 {
				t.Fatalf("avatar directory changed: %v %v", err, files)
			}
		})
	}
}

func TestOversizedAvatar(t *testing.T) {
	st := openStore(t)
	name := strings.Repeat("f", 64) + ".png"
	picture := append(pngImage(t), make([]byte, maxAvatar)...)
	old := export{Format: "cozycast-export", Version: 1, Users: []userRow{{Username: "alice", Password: testHash, AvatarURL: "/avatar/image/" + name}}}
	path := archive(t, jsonEntry(t, old), entry{name: "avatar/" + name, body: picture})
	summary, err := Import(context.Background(), st, path, t.TempDir())
	if err != nil || summary.Users != 1 || summary.Avatars != 0 || len(summary.Skipped) != 2 || !strings.Contains(summary.Skipped[0], "exceeds 5 MiB") {
		t.Fatalf("oversized avatar: err=%v summary=%+v", err, summary)
	}
	if user(t, st, "alice").Avatar != "" {
		t.Fatal("oversized avatar referenced")
	}
}

func TestImportRollbackDoesNotCopyAvatars(t *testing.T) {
	st := openStore(t)
	name := strings.Repeat("a", 64) + ".png"
	old := export{Format: "cozycast-export", Version: 1, Users: []userRow{
		{Username: "Alice", Password: testHash, AvatarURL: "/avatar/image/" + name},
		{Username: "ALICE", Password: testHash},
	}}
	path := archive(t, entry{name: "avatar/" + name, body: pngImage(t)}, jsonEntry(t, old))
	dir := t.TempDir()
	if _, err := Import(context.Background(), st, path, dir); !errors.Is(err, store.ErrUsernameTaken) {
		t.Fatalf("expected duplicate user failure: %v", err)
	}
	if exists, err := st.HasUsers(context.Background()); err != nil || exists {
		t.Fatalf("transaction did not roll back: %v exists=%v", err, exists)
	}
	if files, err := os.ReadDir(dir); err != nil || len(files) != 0 {
		t.Fatalf("avatars copied before commit: %v %v", err, files)
	}
}

func TestImportPreservesLogin(t *testing.T) {
	// Generate one real hash so the importer is exercised through bcrypt's
	// password verification, without depending on a fixture's plaintext.
	hash, err := bcrypt.GenerateFromPassword([]byte("unchanged password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"$2a$", "$2b$", "$2y$"} {
		t.Run(prefix, func(t *testing.T) {
			st := openStore(t)
			legacyHash := prefix + string(hash[4:])
			path := archive(t, jsonEntry(t, export{Format: "cozycast-export", Version: 1, Users: []userRow{{Username: "alice", Password: legacyHash, Enabled: true}}}))
			if _, err := Import(context.Background(), st, path, t.TempDir()); err != nil {
				t.Fatal(err)
			}
			if err := bcrypt.CompareHashAndPassword([]byte(user(t, st, "alice").PasswordHash), []byte("unchanged password")); err != nil {
				t.Fatalf("imported password no longer verifies: %v", err)
			}
		})
	}
}
