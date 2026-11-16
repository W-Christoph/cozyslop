package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestWatchlog(t *testing.T) {
	ctx := context.Background()
	s, err := OpenWatchlog(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	// Accounts are the main database's: ids only here.
	alice, bob := &User{ID: 1}, &User{ID: 2}

	if err := s.SetWatched(ctx, alice.ID, 1, 1, 3, true, "manual"); !errors.Is(err, ErrNotFound) {
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

	// Alice watched 1 to 5 and unticks 2; a room then saw 4 (which she has)
	// and 7, with Bob.
	if err := s.SetWatched(ctx, alice.ID, 1, 1, 5, true, "manual"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetWatched(ctx, alice.ID, 1, 2, 2, false, "manual"); err != nil {
		t.Fatal(err)
	}
	for _, episode := range []int{4, 7} {
		if err := s.MarkWatched(ctx, []int64{alice.ID, bob.ID}, 1, episode); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.MarkWatched(ctx, []int64{alice.ID}, 2, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an episode of an unknown show: %v", err)
	}
	list, err := s.ListProgress(ctx)
	if err != nil || len(list) != 2 || list[0].UserID != alice.ID || !reflect.DeepEqual(list[0].Watched, [][2]int{{1, 1}, {3, 5}, {7, 7}}) ||
		list[1].UserID != bob.ID || !reflect.DeepEqual(list[1].Watched, [][2]int{{4, 4}, {7, 7}}) {
		t.Fatalf("progress: %v %+v", err, list)
	}
	if got, err := s.Watched(ctx, bob.ID, 1); err != nil || !reflect.DeepEqual(got, [][2]int{{4, 4}, {7, 7}}) {
		t.Fatalf("watched: %v %+v", err, got)
	}

	// A deleted account leaves nothing.
	if err := s.SetIgnored(ctx, 3, 1, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetWatched(ctx, 3, 1, 1, 2, true, "manual"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUser(ctx, 3); err != nil {
		t.Fatal(err)
	}
	ignored, err := s.ListIgnored(ctx)
	if list, _ := s.ListProgress(ctx); err != nil || len(ignored) != 0 || len(list) != 2 {
		t.Fatalf("after deleting an account: %v %+v %+v", err, ignored, list)
	}

	if err := s.SetWatched(ctx, bob.ID, 1, 1, 9999, false, "manual"); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListProgress(ctx); len(list) != 1 {
		t.Fatalf("after clearing: %+v", list)
	}
	if got, err := s.Watched(ctx, bob.ID, 1); err != nil || len(got) != 0 {
		t.Fatalf("cleared: %v %+v", err, got)
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
