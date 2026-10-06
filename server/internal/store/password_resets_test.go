package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestPasswordResets(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	now := int64(1000)
	s.now = func() time.Time { return time.Unix(now, 0) }
	u := &User{Username: "alice", Nickname: "alice", PasswordHash: "old"}
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	issue := func() string {
		t.Helper()
		token, expires, err := s.CreatePasswordReset(ctx, u.ID, u.ID)
		if err != nil || expires != now+86400 {
			t.Fatalf("issue: %d %v", expires, err)
		}
		return token
	}
	invalid := func(token string) {
		t.Helper()
		if _, err := s.PasswordResetUser(ctx, token); !errors.Is(err, ErrInvalidPasswordReset) {
			t.Fatalf("check invalid: %v", err)
		}
		if _, err := s.RedeemPasswordReset(ctx, token, "bad"); !errors.Is(err, ErrInvalidPasswordReset) {
			t.Fatalf("redeem invalid: %v", err)
		}
	}
	token := issue()
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		t.Fatalf("token format: %v", err)
	}
	var stored []byte
	var creator, created, expires int64
	var used *int64
	if err := s.db.QueryRowContext(ctx, "SELECT token_hash, created_by, created_at, expires_at, used_at FROM password_resets").Scan(&stored, &creator, &created, &expires, &used); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(token))
	if !bytes.Equal(stored, digest[:]) || creator != u.ID || created != now || expires != now+86400 || used != nil {
		t.Fatal("stored reset metadata")
	}
	if got, err := s.PasswordResetUser(ctx, token); err != nil || got.Username != "alice" {
		t.Fatalf("check: %+v %v", got, err)
	}
	old := token
	token = issue()
	invalid(old)
	invalid("missing")
	invalid(base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	session := []byte("session")
	if err := s.CreateSession(ctx, session, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, "INSERT INTO legacy_logins (user_id, token_hash) VALUES (?, ?)", u.ID, []byte("legacy")); err != nil {
		t.Fatal(err)
	}
	if id, err := s.RedeemPasswordReset(ctx, token, "new"); err != nil || id != u.ID {
		t.Fatalf("redeem: %d %v", id, err)
	}
	invalid(token)
	got, err := s.UserByID(ctx, u.ID)
	if err != nil || got.PasswordHash != "new" {
		t.Fatalf("password: %+v %v", got, err)
	}
	if _, _, err := s.SessionUser(ctx, session); !errors.Is(err, ErrNotFound) {
		t.Fatalf("session survived: %v", err)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM legacy_logins").Scan(&count); err != nil || count != 0 {
		t.Fatalf("legacy sessions: %d %v", count, err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT used_at FROM password_resets").Scan(&used); err != nil || used == nil || *used != now {
		t.Fatalf("used: %v %v", used, err)
	}
	token = issue()
	now += 86400
	invalid(token)
	issue()
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM password_resets").Scan(&count); err != nil || count != 1 {
		t.Fatalf("cleanup: %d %v", count, err)
	}
	token = issue()
	if err := s.UpdatePassword(ctx, u.ID, "changed"); err != nil {
		t.Fatal(err)
	}
	invalid(token)
	if err := s.DeleteUser(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
}

func TestPasswordResetConcurrentRedemption(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	u := &User{Username: "alice", Nickname: "alice", PasswordHash: "old"}
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	token, _, err := s.CreatePasswordReset(ctx, u.ID, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.RedeemPasswordReset(ctx, token, "new"); results <- err }()
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		} else if !errors.Is(err, ErrInvalidPasswordReset) {
			t.Fatal(err)
		}
	}
	if wins != 1 {
		t.Fatalf("successful redemptions: %d", wins)
	}
}

func TestResetAdminInvalidatesPasswordReset(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	u := &User{Username: "admin", Nickname: "admin", PasswordHash: "old", Admin: true}
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	token, _, err := s.CreatePasswordReset(ctx, u.ID, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResetAdmin(ctx, "new"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PasswordResetUser(ctx, token); !errors.Is(err, ErrInvalidPasswordReset) {
		t.Fatalf("reset-admin link survived: %v", err)
	}
}
