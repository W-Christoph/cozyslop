package store

import (
	"context"
	"errors"
	"testing"
)

func TestWatchlog(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	alice := &User{Username: "alice", PasswordHash: "x", Nickname: "Alice"}
	bob := &User{Username: "bob", PasswordHash: "x", Nickname: "Bob"}
	for _, u := range []*User{alice, bob} {
		if err := s.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := s.SetProgress(ctx, alice.ID, 1, 3, "manual"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("progress for an unknown show: %v", err)
	}
	twelve := 12
	a := &Anime{ID: 1, Title: "Frieren", Episodes: &twelve}
	if err := s.SaveAnime(ctx, a); err != nil || a.AddedAt == 0 {
		t.Fatalf("save: %v %+v", err, a)
	}
	// Saving again updates what AniList says and keeps when it was added.
	added := a.AddedAt
	if err := s.SaveAnime(ctx, &Anime{ID: 1, Title: "Sousou no Frieren"}); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Anime(ctx, 1); err != nil || got.Title != "Sousou no Frieren" || got.Episodes != nil || got.AddedAt != added {
		t.Fatalf("updated: %v %+v", err, got)
	}

	if _, err := s.SetProgress(ctx, alice.ID, 1, 5, "manual"); err != nil {
		t.Fatal(err)
	}
	// Raising never lowers: Alice stays at 5, Bob gets 4.
	if err := s.RaiseProgress(ctx, []int64{alice.ID, bob.ID, 999}, 1, 4); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListProgress(ctx)
	if err != nil || len(list) != 2 || list[0].Username != "alice" || list[0].Episode != 5 || list[0].Source != "manual" ||
		list[1].Username != "bob" || list[1].Episode != 4 || list[1].Source != "auto" {
		t.Fatalf("progress: %v %+v", err, list)
	}
	users, err := s.WatchlogUsers(ctx)
	if err != nil || len(users) != 2 {
		t.Fatalf("users: %v %+v", err, users)
	}

	if _, err := s.SetProgress(ctx, bob.ID, 1, 0, "manual"); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListProgress(ctx); len(list) != 1 {
		t.Fatalf("after clearing: %+v", list)
	}

	if err := s.SaveAlias(ctx, "sousou no frieren", 1); err != nil {
		t.Fatal(err)
	}
	if got, err := s.AnimeByAlias(ctx, "sousou no frieren"); err != nil || got.ID != 1 {
		t.Fatalf("alias: %v %+v", err, got)
	}
	if _, err := s.AnimeByAlias(ctx, "other"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown alias: %v", err)
	}

	// Removing the show takes its progress and aliases along.
	if err := s.DeleteAnime(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListProgress(ctx); len(list) != 0 {
		t.Fatalf("progress left: %+v", list)
	}
	if _, err := s.AnimeByAlias(ctx, "sousou no frieren"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("alias left: %v", err)
	}
	if err := s.DeleteAnime(ctx, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
}
