package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestUsersAndSessions(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	u := &User{Username: "Alice", PasswordHash: "x", Nickname: "Alice"}
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	if u.Username != "alice" || u.ID == 0 {
		t.Fatalf("got %+v", u)
	}
	if err := s.CreateUser(ctx, &User{Username: "ALICE", PasswordHash: "x", Nickname: "a"}); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("duplicate: %v", err)
	}

	token := []byte("hash")
	if err := s.CreateSession(ctx, token, u.ID); err != nil {
		t.Fatal(err)
	}
	got, _, err := s.SessionUser(ctx, token)
	if err != nil || got.ID != u.ID {
		t.Fatalf("session user: %v %+v", err, got)
	}

	// Disabled accounts lose their sessions' validity.
	if err := s.UpdateFlags(ctx, u.ID, false, false, true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SessionUser(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("disabled user session: %v", err)
	}
	s.UpdateFlags(ctx, u.ID, false, false, false)

	// Expiry.
	s.now = func() time.Time { return time.Now().Add(SessionTTL + time.Minute) }
	if _, _, err := s.SessionUser(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired session: %v", err)
	}
}

func TestSettingsDefaultsAndSave(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	set, err := s.Settings(ctx)
	if err != nil || set.Registration != "invite" || set.Message != "" {
		t.Fatalf("defaults: %v %+v", err, set)
	}
	if err := s.SaveSettings(ctx, Settings{Message: "hi", Registration: "open"}); err != nil {
		t.Fatal(err)
	}
	if set, _ := s.Settings(ctx); set.Message != "hi" || set.Registration != "open" {
		t.Fatalf("saved: %+v", set)
	}
}
