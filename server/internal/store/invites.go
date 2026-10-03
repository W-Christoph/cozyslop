package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"math/big"
)

var ErrInvalidInvite = errors.New("invalid invite")

type Invite struct {
	Code      string `json:"code"`
	Room      string `json:"room"`
	Temporary bool   `json:"temporary"` // true: one room visit; false: account invite
	Name      string `json:"name"`
	Remote    bool   `json:"remote"`
	Image     bool   `json:"image"`
	Upload    bool   `json:"upload"`
	Uses      int    `json:"uses"`
	MaxUses   *int   `json:"maxUses"`   // nil = unlimited
	ExpiresAt *int64 `json:"expiresAt"` // unix seconds, nil = never
	CreatedAt int64  `json:"createdAt"`
}

// Valid reports whether the invite has not expired or reached its use limit.
func (i *Invite) Valid(now int64) bool {
	return (i.ExpiresAt == nil || *i.ExpiresAt > now) && (i.MaxUses == nil || i.Uses < *i.MaxUses)
}

const inviteColumns = "code, room, temporary, name, remote, image, upload, uses, max_uses, expires_at, created_at"

func scanInvite(row interface{ Scan(...any) error }) (*Invite, error) {
	var i Invite
	err := row.Scan(&i.Code, &i.Room, &i.Temporary, &i.Name, &i.Remote, &i.Image,
		&i.Upload, &i.Uses, &i.MaxUses, &i.ExpiresAt, &i.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &i, err
}

func inviteCode() (string, error) {
	const alphabet = "abcdefghijkmnpqrstuvwxyz23456789"
	var code [12]byte
	limit := big.NewInt(int64(len(alphabet)))
	for j := range code {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		code[j] = alphabet[n.Int64()]
	}
	return string(code[:]), nil
}

// CreateInvite inserts i, filling in its code, creation time and use count.
func (s *Store) CreateInvite(ctx context.Context, i *Invite) error {
	i.CreatedAt = s.unix()
	i.Uses = 0
	for {
		code, err := inviteCode()
		if err != nil {
			return err
		}
		_, err = s.db.ExecContext(ctx,
			"INSERT INTO invites ("+inviteColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			code, i.Room, i.Temporary, i.Name, i.Remote, i.Image, i.Upload, i.Uses, i.MaxUses, i.ExpiresAt, i.CreatedAt)
		if isUniqueViolation(err) {
			if err := ctx.Err(); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		i.Code = code
		return nil
	}
}

func (s *Store) Invite(ctx context.Context, code string) (*Invite, error) {
	return scanInvite(s.db.QueryRowContext(ctx, "SELECT "+inviteColumns+" FROM invites WHERE code = ?", code))
}

// ListInvites lists one room's invites, or all rooms if room is empty, newest first.
func (s *Store) ListInvites(ctx context.Context, room string) ([]*Invite, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+inviteColumns+" FROM invites WHERE (? = '' OR room = ?) ORDER BY created_at DESC, code", room, room)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var invites []*Invite
	for rows.Next() {
		i, err := scanInvite(rows)
		if err != nil {
			return nil, err
		}
		invites = append(invites, i)
	}
	return invites, rows.Err()
}

func (s *Store) DeleteInvite(ctx context.Context, code string) error {
	return s.execOne(ctx, "DELETE FROM invites WHERE code = ?", code)
}

// UseAccessInvite consumes a valid temporary invite for this room.
func (s *Store) UseAccessInvite(ctx context.Context, code, room string) (*Invite, error) {
	var out *Invite
	err := s.tx(ctx, func(tx *sql.Tx) error {
		i, err := scanInvite(tx.QueryRowContext(ctx, "SELECT "+inviteColumns+" FROM invites WHERE code = ?", code))
		if errors.Is(err, ErrNotFound) {
			return ErrInvalidInvite
		}
		if err != nil {
			return err
		}
		if !i.Temporary || i.Room != room || !i.Valid(s.unix()) {
			return ErrInvalidInvite
		}
		if _, err := tx.ExecContext(ctx, "UPDATE invites SET uses = uses + 1 WHERE code = ?", code); err != nil {
			return err
		}
		i.Uses++
		out = i
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RedeemInvite grants an account invite, counting each user's redemption once.
func (s *Store) RedeemInvite(ctx context.Context, code string, userID int64) (*Invite, error) {
	var out *Invite
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var err error
		out, err = s.redeemInvite(ctx, tx, code, userID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) redeemInvite(ctx context.Context, tx *sql.Tx, code string, userID int64) (*Invite, error) {
	i, err := scanInvite(tx.QueryRowContext(ctx, "SELECT "+inviteColumns+" FROM invites WHERE code = ?", code))
	if errors.Is(err, ErrNotFound) {
		return nil, ErrInvalidInvite
	}
	if err != nil {
		return nil, err
	}
	now := s.unix()
	if i.Temporary || (i.ExpiresAt != nil && *i.ExpiresAt <= now) {
		return nil, ErrInvalidInvite
	}
	var redeemed bool
	if err := tx.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM invite_redemptions WHERE code = ? AND user_id = ?)", code, userID).Scan(&redeemed); err != nil {
		return nil, err
	}
	if !redeemed {
		if !i.Valid(now) {
			return nil, ErrInvalidInvite
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO invite_redemptions (code, user_id) VALUES (?, ?)", code, userID); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE invites SET uses = uses + 1 WHERE code = ?", code); err != nil {
			return nil, err
		}
		i.Uses++
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO room_permissions (room, user_id, invited, remote, image, upload, invite_name)
		 VALUES (?, ?, 1, ?, ?, ?, ?)
		 ON CONFLICT (room, user_id) DO UPDATE SET invited = 1,
		 remote = room_permissions.remote OR excluded.remote,
		 image = room_permissions.image OR excluded.image,
		 upload = room_permissions.upload OR excluded.upload,
		 invite_name = CASE WHEN excluded.invite_name != '' THEN excluded.invite_name ELSE room_permissions.invite_name END`,
		i.Room, userID, i.Remote, i.Image, i.Upload, i.Name)
	if err != nil {
		return nil, err
	}
	return i, nil
}

// RegisterUser creates an account and redeems its optional required invite atomically.
func (s *Store) RegisterUser(ctx context.Context, u *User, inviteCode string, requireInvite bool) (*Invite, error) {
	if requireInvite && inviteCode == "" {
		return nil, ErrInvalidInvite
	}
	created := *u
	var out *Invite
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if err := s.createUser(ctx, tx, &created); err != nil {
			return err
		}
		if inviteCode != "" {
			var err error
			out, err = s.redeemInvite(ctx, tx, inviteCode, created.ID)
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	*u = created
	return out, nil
}
