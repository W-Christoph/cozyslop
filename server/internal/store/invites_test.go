package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestInviteValid(t *testing.T) {
	now := int64(1000)
	past, future := now-1, now+1
	zero, one := 0, 1
	for _, tt := range []struct {
		name string
		i    Invite
		want bool
	}{
		{"unlimited", Invite{Uses: 100}, true},
		{"future", Invite{ExpiresAt: &future}, true},
		{"expired", Invite{ExpiresAt: &past}, false},
		{"expiry boundary", Invite{ExpiresAt: &now}, false},
		{"unused", Invite{MaxUses: &one}, true},
		{"exhausted", Invite{MaxUses: &one, Uses: 1}, false},
		{"over limit", Invite{MaxUses: &one, Uses: 2}, false},
		{"zero limit", Invite{MaxUses: &zero}, false},
		{"both valid", Invite{MaxUses: &one, ExpiresAt: &future}, true},
		{"expired unused", Invite{MaxUses: &one, ExpiresAt: &past}, false},
		{"future exhausted", Invite{MaxUses: &one, Uses: 1, ExpiresAt: &future}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.i.Valid(now); got != tt.want {
				t.Fatalf("Valid = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInvites(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	now := int64(1000)
	s.now = func() time.Time { return time.Unix(now, 0) }
	if _, err := s.Invite(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if got, err := s.ListInvites(ctx, ""); err != nil || len(got) != 0 {
		t.Fatalf("empty: %v %+v", err, got)
	}
	maxUses, expires := 3, int64(2000)
	first := &Invite{Code: "supplied", Room: "main", Temporary: true, Name: "friends", Remote: true,
		Image: true, Upload: true, Uses: 99, MaxUses: &maxUses, ExpiresAt: &expires, CreatedAt: 99}
	if err := s.CreateInvite(ctx, first); err != nil {
		t.Fatal(err)
	}
	if len(first.Code) != 12 || first.Code == "supplied" || first.Uses != 0 || first.CreatedAt != now {
		t.Fatalf("generated fields: %+v", first)
	}
	for _, c := range first.Code {
		if !strings.ContainsRune("abcdefghijkmnpqrstuvwxyz23456789", c) {
			t.Fatalf("unexpected code character %q", c)
		}
	}
	if got, err := s.Invite(ctx, first.Code); err != nil || !reflect.DeepEqual(got, first) {
		t.Fatalf("round trip: %v %+v, want %+v", err, got, first)
	}
	now++
	second := &Invite{Room: "other"}
	if err := s.CreateInvite(ctx, second); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Invite(ctx, second.Code); err != nil || !reflect.DeepEqual(got, second) {
		t.Fatalf("nullable round trip: %v %+v", err, got)
	}
	now++
	third := &Invite{Room: "main"}
	if err := s.CreateInvite(ctx, third); err != nil {
		t.Fatal(err)
	}
	if got, err := s.ListInvites(ctx, ""); err != nil || !reflect.DeepEqual(got, []*Invite{third, second, first}) {
		t.Fatalf("all newest first: %v %+v", err, got)
	}
	if got, err := s.ListInvites(ctx, "main"); err != nil || !reflect.DeepEqual(got, []*Invite{third, first}) {
		t.Fatalf("room newest first: %v %+v", err, got)
	}
	if err := s.DeleteInvite(ctx, first.Code); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteInvite(ctx, first.Code); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing: %v", err)
	}
	if _, err := s.Invite(ctx, first.Code); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted: %v", err)
	}
}

func TestUseAccessInvite(t *testing.T) {
	ctx := context.Background()
	now, past, future := int64(1000), int64(999), int64(1001)
	zero, two := 0, 2
	for _, tt := range []struct {
		name string
		i    Invite
		room string
	}{
		{"wrong room", Invite{Temporary: true}, "other"},
		{"account invite", Invite{}, "main"},
		{"expired", Invite{Temporary: true, ExpiresAt: &past}, "main"},
		{"expiry boundary", Invite{Temporary: true, ExpiresAt: &now}, "main"},
		{"exhausted", Invite{Temporary: true, MaxUses: &zero}, "main"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := openTest(t)
			s.now = func() time.Time { return time.Unix(now, 0) }
			tt.i.Room = "main"
			if err := s.CreateInvite(ctx, &tt.i); err != nil {
				t.Fatal(err)
			}
			if got, err := s.UseAccessInvite(ctx, tt.i.Code, tt.room); !errors.Is(err, ErrInvalidInvite) || got != nil {
				t.Fatalf("rejected: %v %+v", err, got)
			}
			if got, err := s.Invite(ctx, tt.i.Code); err != nil || !reflect.DeepEqual(got, &tt.i) {
				t.Fatalf("unchanged: %v %+v", err, got)
			}
		})
	}
	s := openTest(t)
	s.now = func() time.Time { return time.Unix(now, 0) }
	if got, err := s.UseAccessInvite(ctx, "missing", "main"); !errors.Is(err, ErrInvalidInvite) || got != nil {
		t.Fatalf("missing: %v %+v", err, got)
	}
	i := &Invite{Room: "main", Temporary: true, Remote: true, Image: true, Upload: true, MaxUses: &two, ExpiresAt: &future}
	if err := s.CreateInvite(ctx, i); err != nil {
		t.Fatal(err)
	}
	for uses := 1; uses <= two; uses++ {
		i.Uses = uses
		if got, err := s.UseAccessInvite(ctx, i.Code, "main"); err != nil || !reflect.DeepEqual(got, i) {
			t.Fatalf("use %d: %v %+v", uses, err, got)
		}
	}
	if _, err := s.UseAccessInvite(ctx, i.Code, "main"); !errors.Is(err, ErrInvalidInvite) {
		t.Fatalf("used up: %v", err)
	}
	if got, err := s.Invite(ctx, i.Code); err != nil || got.Uses != two {
		t.Fatalf("stored count: %v %+v", err, got)
	}
	i = &Invite{Room: "main", Temporary: true}
	if err := s.CreateInvite(ctx, i); err != nil {
		t.Fatal(err)
	}
	for uses := 1; uses <= 3; uses++ {
		if got, err := s.UseAccessInvite(ctx, i.Code, "main"); err != nil || got.Uses != uses {
			t.Fatalf("unlimited use: %v %+v", err, got)
		}
	}
}

func TestRedeemInvite(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	now, until, expires := int64(1000), int64(2000), int64(1100)
	s.now = func() time.Time { return time.Unix(now, 0) }
	alice, bob := &User{Username: "alice"}, &User{Username: "bob"}
	for _, u := range []*User{alice, bob} {
		if err := s.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	p := Permission{Room: "main", UserID: alice.ID, Remote: true, Trusted: true, Banned: true,
		BannedUntil: &until, InviteName: "old"}
	if err := s.SavePermission(ctx, p); err != nil {
		t.Fatal(err)
	}
	one := 1
	i := &Invite{Room: "main", Name: "new", Image: true, MaxUses: &one, ExpiresAt: &expires}
	if err := s.CreateInvite(ctx, i); err != nil {
		t.Fatal(err)
	}
	i.Uses = 1
	for n := 0; n < 2; n++ {
		if got, err := s.RedeemInvite(ctx, i.Code, alice.ID); err != nil || !reflect.DeepEqual(got, i) {
			t.Fatalf("redeem %d: %v %+v", n, err, got)
		}
	}
	p.Invited, p.Image, p.InviteName = true, true, "new"
	if got, err := s.Permission(ctx, p.Room, p.UserID); err != nil || !reflect.DeepEqual(got, p) {
		t.Fatalf("OR grants and preserve ban/trust: %v %+v, want %+v", err, got, p)
	}
	if got, err := s.RedeemInvite(ctx, i.Code, bob.ID); !errors.Is(err, ErrInvalidInvite) || got != nil {
		t.Fatalf("exhausted new user: %v %+v", err, got)
	}
	if got, err := s.Permission(ctx, "main", bob.ID); err != nil || !reflect.DeepEqual(got, Permission{Room: "main", UserID: bob.ID}) {
		t.Fatalf("failed redeem permission: %v %+v", err, got)
	}
	now = expires
	if _, err := s.RedeemInvite(ctx, i.Code, alice.ID); !errors.Is(err, ErrInvalidInvite) {
		t.Fatalf("expired repeat: %v", err)
	}
	emptyName := &Invite{Room: "main", Upload: true}
	if err := s.CreateInvite(ctx, emptyName); err != nil {
		t.Fatal(err)
	}
	if got, err := s.RedeemInvite(ctx, emptyName.Code, alice.ID); err != nil || got.Uses != 1 {
		t.Fatalf("second invite: %v %+v", err, got)
	}
	p.Upload = true
	if got, err := s.Permission(ctx, p.Room, p.UserID); err != nil || !reflect.DeepEqual(got, p) {
		t.Fatalf("empty name keeps old and ORs grants: %v %+v", err, got)
	}
	if got, err := s.RedeemInvite(ctx, emptyName.Code, bob.ID); err != nil || got.Uses != 2 {
		t.Fatalf("second user counts: %v %+v", err, got)
	}
	wantBob := Permission{Room: "main", UserID: bob.ID, Invited: true, Upload: true}
	if got, err := s.Permission(ctx, "main", bob.ID); err != nil || !reflect.DeepEqual(got, wantBob) {
		t.Fatalf("new permission: %v %+v", err, got)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM invite_redemptions WHERE code = ?", i.Code).Scan(&count); err != nil || count != 1 {
		t.Fatalf("one redemption per user: %v count=%d", err, count)
	}
	if err := s.DeleteInvite(ctx, i.Code); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM invite_redemptions WHERE code = ?", i.Code).Scan(&count); err != nil || count != 0 {
		t.Fatalf("delete cascades: %v count=%d", err, count)
	}
}

func TestRedeemInviteInvalid(t *testing.T) {
	ctx := context.Background()
	now, past, zero := int64(1000), int64(999), 0
	for _, tt := range []struct {
		name    string
		i       Invite
		missing bool
	}{
		{"missing", Invite{}, true},
		{"temporary", Invite{Temporary: true}, false},
		{"expired", Invite{ExpiresAt: &past}, false},
		{"expiry boundary", Invite{ExpiresAt: &now}, false},
		{"exhausted", Invite{MaxUses: &zero}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := openTest(t)
			s.now = func() time.Time { return time.Unix(now, 0) }
			u := &User{Username: "alice"}
			if err := s.CreateUser(ctx, u); err != nil {
				t.Fatal(err)
			}
			tt.i.Room = "main"
			if !tt.missing {
				if err := s.CreateInvite(ctx, &tt.i); err != nil {
					t.Fatal(err)
				}
			}
			if got, err := s.RedeemInvite(ctx, tt.i.Code, u.ID); !errors.Is(err, ErrInvalidInvite) || got != nil {
				t.Fatalf("invalid: %v %+v", err, got)
			}
			if !tt.missing {
				if got, err := s.Invite(ctx, tt.i.Code); err != nil || !reflect.DeepEqual(got, &tt.i) {
					t.Fatalf("unchanged invite: %v %+v", err, got)
				}
			}
			if got, err := s.ListPermissions(ctx, ""); err != nil || len(got) != 0 {
				t.Fatalf("no permission: %v %+v", err, got)
			}
		})
	}
}

func TestRegisterUser(t *testing.T) {
	ctx := context.Background()
	now, past, zero := int64(1000), int64(999), 0
	for _, tt := range []struct {
		name     string
		i        *Invite
		code     string
		required bool
		invalid  bool
	}{
		{"open", nil, "", false, false},
		{"required missing", nil, "", true, true},
		{"unknown", nil, "missing", false, true},
		{"unknown required", nil, "missing", true, true},
		{"temporary", &Invite{Temporary: true}, "", false, true},
		{"expired", &Invite{ExpiresAt: &past}, "", false, true},
		{"exhausted", &Invite{MaxUses: &zero}, "", true, true},
		{"optional invite", &Invite{Remote: true, Image: true, Upload: true, Name: "friends"}, "", false, false},
		{"required invite", &Invite{Remote: true, Name: "friends"}, "", true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := openTest(t)
			s.now = func() time.Time { return time.Unix(now, 0) }
			if tt.i != nil {
				tt.i.Room = "main"
				if err := s.CreateInvite(ctx, tt.i); err != nil {
					t.Fatal(err)
				}
				tt.code = tt.i.Code
			}
			original := User{Username: "Alice", PasswordHash: "hash", Nickname: "Alice", Avatar: "avatar", Admin: true, Verified: true, Disabled: true}
			u := original
			got, err := s.RegisterUser(ctx, &u, tt.code, tt.required)
			if tt.invalid {
				if !errors.Is(err, ErrInvalidInvite) || got != nil {
					t.Fatalf("invalid registration: %v %+v", err, got)
				}
				if u != original {
					t.Fatalf("caller changed on rollback: %+v", u)
				}
				if _, err := s.UserByUsername(ctx, "alice"); !errors.Is(err, ErrNotFound) {
					t.Fatalf("user rolled back: %v", err)
				}
				if tt.i != nil {
					if got, err := s.Invite(ctx, tt.code); err != nil || !reflect.DeepEqual(got, tt.i) {
						t.Fatalf("invite unchanged: %v %+v", err, got)
					}
				}
				return
			}
			if err != nil || u.ID == 0 || u.Username != "alice" || u.CreatedAt != now || u.NameColor != "#fff" {
				t.Fatalf("registered: %v %+v", err, u)
			}
			if stored, err := s.UserByID(ctx, u.ID); err != nil || !reflect.DeepEqual(stored, &u) {
				t.Fatalf("stored user: %v %+v", err, stored)
			}
			if tt.i == nil {
				if got != nil {
					t.Fatalf("open registration returned invite: %+v", got)
				}
			} else {
				tt.i.Uses = 1
				if !reflect.DeepEqual(got, tt.i) {
					t.Fatalf("redeemed invite: %+v, want %+v", got, tt.i)
				}
				want := Permission{Room: "main", UserID: u.ID, Invited: true, InviteName: tt.i.Name,
					Remote: tt.i.Remote, Image: tt.i.Image, Upload: tt.i.Upload}
				if p, err := s.Permission(ctx, "main", u.ID); err != nil || !reflect.DeepEqual(p, want) {
					t.Fatalf("registration grants: %v %+v", err, p)
				}
			}
			if _, err := s.RegisterUser(ctx, &User{Username: "ALICE"}, tt.code, false); !errors.Is(err, ErrUsernameTaken) {
				t.Fatalf("duplicate: %v", err)
			}
			if tt.i != nil {
				if got, err := s.Invite(ctx, tt.code); err != nil || got.Uses != 1 {
					t.Fatalf("duplicate did not consume invite: %v %+v", err, got)
				}
			}
		})
	}
}

func TestRedeemInviteRollback(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	u := &User{Username: "alice"}
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	i := &Invite{Room: "main"}
	if err := s.CreateInvite(ctx, i); err != nil {
		t.Fatal(err)
	}
	// Fail after the redemption and count writes, to verify the whole transaction.
	if _, err := s.db.ExecContext(ctx, `CREATE TRIGGER reject_permission BEFORE INSERT ON room_permissions
	 BEGIN SELECT RAISE(ABORT, 'test permission failure'); END`); err != nil {
		t.Fatal(err)
	}
	if got, err := s.RedeemInvite(ctx, i.Code, u.ID); err == nil || got != nil {
		t.Fatalf("expected failure: %v %+v", err, got)
	}
	if got, err := s.RegisterUser(ctx, &User{Username: "bob"}, i.Code, true); err == nil || got != nil {
		t.Fatalf("expected registration failure: %v %+v", err, got)
	}
	if _, err := s.UserByUsername(ctx, "bob"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("user rollback: %v", err)
	}
	if got, err := s.Invite(ctx, i.Code); err != nil || got.Uses != 0 {
		t.Fatalf("use rollback: %v %+v", err, got)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM invite_redemptions").Scan(&count); err != nil || count != 0 {
		t.Fatalf("redemption rollback: %v count=%d", err, count)
	}
}

func TestInviteConcurrentUse(t *testing.T) {
	ctx := context.Background()
	for _, temporary := range []bool{true, false} {
		t.Run(map[bool]string{true: "access", false: "account"}[temporary], func(t *testing.T) {
			s := openTest(t)
			one := 1
			i := &Invite{Room: "main", Temporary: temporary, MaxUses: &one}
			if err := s.CreateInvite(ctx, i); err != nil {
				t.Fatal(err)
			}
			var users [8]User
			if !temporary {
				for j := range users {
					users[j].Username = string(rune('a' + j))
					if err := s.CreateUser(ctx, &users[j]); err != nil {
						t.Fatal(err)
					}
				}
			}
			results := make(chan error, len(users))
			var wg sync.WaitGroup
			for j := range users {
				wg.Go(func() {
					var err error
					if temporary {
						_, err = s.UseAccessInvite(ctx, i.Code, "main")
					} else {
						_, err = s.RedeemInvite(ctx, i.Code, users[j].ID)
					}
					results <- err
				})
			}
			wg.Wait()
			close(results)
			success := 0
			for err := range results {
				if err == nil {
					success++
				} else if !errors.Is(err, ErrInvalidInvite) {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			if got, err := s.Invite(ctx, i.Code); err != nil || got.Uses != 1 || success != 1 {
				t.Fatalf("atomic limit: %v %+v success=%d", err, got, success)
			}
		})
	}
}
