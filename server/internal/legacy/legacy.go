// Package legacy imports exports from the original PostgreSQL CozyCast server.
package legacy

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"cozycast/internal/store"

	"golang.org/x/crypto/bcrypt"
)

type Summary struct {
	Users, Rooms, Permissions, Invites, Avatars int
	Skipped                                     []string
}

var ErrNotEmpty = store.ErrImportNotEmpty

const (
	maxExport = 50 << 20
	maxAvatar = 5 << 20
)

var (
	avatarName = regexp.MustCompile(`^[0-9a-f]{64}\.(png|jpg|jpeg|webp)$`)
	nameColor  = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}){1,2}$`)
	bcryptHash = regexp.MustCompile(`^\$2[aby]\$[0-9]{2}\$[./A-Za-z0-9]{53}$`)
)

type export struct {
	Format      string          `json:"format"`
	Version     int             `json:"version"`
	Users       []userRow       `json:"users"`
	Rooms       []roomRow       `json:"room_persistence"`
	Permissions []permissionRow `json:"room_permission"`
	Invites     []inviteRow     `json:"room_invite"`
}

type userRow struct {
	Username       string `json:"username"`
	Password       string `json:"password"`
	Enabled        bool   `json:"enabled"`
	AccountExpired bool   `json:"account_expired"`
	AccountLocked  bool   `json:"account_locked"`
	Admin          bool   `json:"admin"`
	AvatarURL      string `json:"avatar_url"`
	Nickname       string `json:"nickname"`
	NameColor      string `json:"name_color"`
	Verified       bool   `json:"verified"`
}

type roomRow struct {
	Name                    string `json:"name"`
	AccountOnly             bool   `json:"account_only"`
	VerifiedOnly            bool   `json:"verified_only"`
	InviteOnly              bool   `json:"invite_only"`
	CenterRemote            bool   `json:"center_remote"`
	DefaultRemotePermission bool   `json:"default_remote_permission"`
	DefaultImagePermission  bool   `json:"default_image_permission"`
	RemoteOwnership         bool   `json:"remote_ownership"`
	HiddenToUnauthorized    bool   `json:"hidden_to_unauthorized"`
}

type permissionRow struct {
	Room             string     `json:"room"`
	UserID           string     `json:"user_id"`
	RemotePermission bool       `json:"remote_permission"`
	ImagePermission  bool       `json:"image_permission"`
	Banned           bool       `json:"banned"`
	BannedUntil      *time.Time `json:"banned_until"`
	Invited          bool       `json:"invited"`
	InviteName       string     `json:"invite_name"`
	Trusted          bool       `json:"trusted"`
}

type inviteRow struct {
	ID               string     `json:"id"`
	Room             string     `json:"room"`
	Uses             int        `json:"uses"`
	MaxUses          *int       `json:"max_uses"`
	Expiration       *time.Time `json:"expiration"`
	RemotePermission bool       `json:"remote_permission"`
	ImagePermission  bool       `json:"image_permission"`
	InviteName       string     `json:"invite_name"`
	Temporary        bool       `json:"temporary"`
}

// Import loads a cozycast-export.tar.gz into an empty store and copies
// avatars into avatarDir. Database writes commit before avatar copies begin.
func Import(ctx context.Context, st *store.Store, archivePath, avatarDir string) (Summary, error) {
	var summary Summary
	exists, err := st.HasUsers(ctx)
	if err != nil {
		return summary, err
	}
	if exists {
		return summary, ErrNotEmpty
	}
	staging, err := os.MkdirTemp("", "cozycast-import-")
	if err != nil {
		return summary, err
	}
	defer os.RemoveAll(staging)
	old, avatars, err := readArchive(ctx, archivePath, staging, &summary)
	if err != nil {
		return summary, err
	}
	if old.Format != "cozycast-export" || old.Version != 1 {
		return summary, errors.New("unsupported CozyCast export format or version")
	}
	data := mapRows(old, avatars, time.Now(), &summary)
	ids, err := st.ImportLegacy(ctx, data)
	if err != nil {
		return summary, err
	}
	summary.Users = len(data.Users)
	summary.Rooms = len(data.Rooms)
	summary.Permissions = len(data.Permissions)
	summary.Invites = len(data.Invites)

	// Multiple accounts may share one file. Copy it once and clear every
	// referencing account if the copy fails, including on cancellation.
	copied := make(map[string]error)
	for _, u := range data.Users {
		if u.Avatar == "" {
			continue
		}
		copyErr, attempted := copied[u.Avatar]
		if !attempted {
			copyErr = ctx.Err()
			if copyErr == nil {
				copyErr = os.MkdirAll(avatarDir, 0o750)
			}
			if copyErr == nil {
				copyErr = copyAvatar(filepath.Join(staging, u.Avatar), filepath.Join(avatarDir, u.Avatar))
			}
			copied[u.Avatar] = copyErr
			if copyErr == nil {
				summary.Avatars++
			} else {
				summary.Skipped = append(summary.Skipped, fmt.Sprintf("avatar %s: copy failed: %v", u.Avatar, copyErr))
			}
		}
		if copyErr != nil {
			if err := st.UpdateAvatar(context.WithoutCancel(ctx), ids[u.Username], ""); err != nil {
				summary.Skipped = append(summary.Skipped, fmt.Sprintf("user %q: clearing avatar failed: %v", u.Username, err))
			}
		}
	}
	return summary, nil
}

func readArchive(ctx context.Context, archivePath, staging string, summary *Summary) (export, map[string]bool, error) {
	var old export
	avatars := make(map[string]bool)
	f, err := os.Open(archivePath)
	if err != nil {
		return old, nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return old, nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	found := false
	for {
		if err := ctx.Err(); err != nil {
			return old, nil, err
		}
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return old, nil, fmt.Errorf("read import archive: %w", err)
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
			continue
		}
		if h.Name == "export.json" {
			if found {
				return old, nil, errors.New("duplicate export.json in archive")
			}
			if h.Size > maxExport {
				return old, nil, errors.New("export.json exceeds 50 MiB")
			}
			body, err := io.ReadAll(io.LimitReader(tr, maxExport+1))
			if err != nil {
				return old, nil, err
			}
			if len(body) > maxExport {
				return old, nil, errors.New("export.json exceeds 50 MiB")
			}
			if err := json.Unmarshal(body, &old); err != nil {
				return old, nil, fmt.Errorf("decode export.json: %w", err)
			}
			found = true
			continue
		}
		name, ok := strings.CutPrefix(h.Name, "avatar/")
		if !ok || !avatarName.MatchString(name) {
			continue
		}
		if h.Size > maxAvatar {
			summary.Skipped = append(summary.Skipped, "avatar "+name+": exceeds 5 MiB")
			continue
		}
		body, err := io.ReadAll(io.LimitReader(tr, maxAvatar+1))
		if err != nil {
			return old, nil, err
		}
		if len(body) > maxAvatar {
			summary.Skipped = append(summary.Skipped, "avatar "+name+": exceeds 5 MiB")
			continue
		}
		switch http.DetectContentType(body) {
		case "image/png", "image/jpeg", "image/webp":
		default:
			summary.Skipped = append(summary.Skipped, "avatar "+name+": unsupported image content")
			continue
		}
		if avatars[name] {
			return old, nil, errors.New("duplicate avatar in archive")
		}
		if err := os.WriteFile(filepath.Join(staging, name), body, 0o600); err != nil {
			return old, nil, err
		}
		avatars[name] = true
	}
	if !found {
		return old, nil, errors.New("archive has no export.json")
	}
	// Drain the gzip reader to verify its checksum even when tar reached its
	// end markers before the gzip trailer.
	if _, err := io.Copy(io.Discard, gz); err != nil {
		return old, nil, fmt.Errorf("read import archive: %w", err)
	}
	return old, avatars, nil
}

func mapRows(old export, avatars map[string]bool, now time.Time, summary *Summary) store.ImportData {
	var data store.ImportData
	users := make(map[string]bool)
	for _, row := range old.Users {
		username := strings.ToLower(row.Username)
		_, hashErr := bcrypt.Cost([]byte(row.Password))
		if !bcryptHash.MatchString(row.Password) || hashErr != nil {
			summary.Skipped = append(summary.Skipped, fmt.Sprintf("user %q: invalid bcrypt hash", username))
			continue
		}
		u := store.User{Username: username, PasswordHash: row.Password, Nickname: row.Nickname,
			NameColor: row.NameColor, Admin: row.Admin, Verified: row.Verified,
			Disabled: !row.Enabled || row.AccountLocked || row.AccountExpired}
		if u.Nickname == "" {
			u.Nickname = username
		}
		if !nameColor.MatchString(u.NameColor) {
			u.NameColor = "#fff"
		}
		if name, ok := strings.CutPrefix(row.AvatarURL, "/avatar/image/"); ok && avatarName.MatchString(name) {
			if avatars[name] {
				u.Avatar = name
			} else {
				summary.Skipped = append(summary.Skipped, fmt.Sprintf("user %q: avatar %s missing or rejected", username, name))
			}
		}
		data.Users = append(data.Users, u)
		users[username] = true
	}
	for _, row := range old.Rooms {
		access := "public"
		switch {
		case row.InviteOnly:
			access = "invite"
		case row.VerifiedOnly:
			access = "verified"
		case row.AccountOnly:
			access = "account"
		}
		data.Rooms = append(data.Rooms, store.RoomSettings{Name: row.Name, Access: access,
			Hidden: row.HiddenToUnauthorized, RemoteOwnership: row.RemoteOwnership,
			CenterRemote: row.CenterRemote, DefaultRemote: row.DefaultRemotePermission, DefaultImage: row.DefaultImagePermission})
	}
	type permissionKey struct{ room, username string }
	permissions := make(map[permissionKey]int)
	for _, row := range old.Permissions {
		username := strings.ToLower(row.UserID)
		if !users[username] {
			summary.Skipped = append(summary.Skipped, fmt.Sprintf("permission for %q in %q: unknown user", username, row.Room))
			continue
		}
		p := store.Permission{Room: row.Room, Username: username, Remote: row.RemotePermission,
			Image: row.ImagePermission, Trusted: row.Trusted, Invited: row.Invited, InviteName: row.InviteName,
			Banned: row.Banned, BannedUntil: unixTime(row.BannedUntil)}
		if p.BannedUntil != nil && *p.BannedUntil <= now.Unix() {
			p.Banned = false
		}
		if !p.Banned {
			p.BannedUntil = nil
		}
		key := permissionKey{p.Room, username}
		if idx, ok := permissions[key]; ok {
			mergePermission(&data.Permissions[idx], p)
		} else {
			permissions[key] = len(data.Permissions)
			data.Permissions = append(data.Permissions, p)
		}
	}
	for _, row := range old.Invites {
		maxUses := row.MaxUses
		if maxUses != nil && *maxUses == 0 {
			maxUses = nil
		}
		i := store.Invite{Code: row.ID, Room: row.Room, Temporary: row.Temporary, Name: row.InviteName,
			Remote: row.RemotePermission, Image: row.ImagePermission, Uses: row.Uses,
			MaxUses: maxUses, ExpiresAt: unixTime(row.Expiration), CreatedAt: now.Unix()}
		if !i.Valid(now.Unix()) {
			summary.Skipped = append(summary.Skipped, fmt.Sprintf("invite in %q: expired or used up", row.Room))
			continue
		}
		data.Invites = append(data.Invites, i)
	}
	return data
}

func unixTime(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	n := t.Unix()
	return &n
}

func mergePermission(p *store.Permission, row store.Permission) {
	p.Remote = p.Remote || row.Remote
	p.Image = p.Image || row.Image
	p.Trusted = p.Trusted || row.Trusted
	p.Invited = p.Invited || row.Invited
	if row.InviteName != "" {
		p.InviteName = row.InviteName
	}
	if row.Banned && (!p.Banned || row.BannedUntil == nil || (p.BannedUntil != nil && *row.BannedUntil > *p.BannedUntil)) {
		p.BannedUntil = row.BannedUntil
	}
	p.Banned = p.Banned || row.Banned
}

func copyAvatar(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(dst)
	}
	return err
}
