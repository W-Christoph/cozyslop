package store

import (
	"context"
	"testing"
)

func TestRoomSettings(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	defaults := RoomSettings{Name: "main", Access: "public", Quality: "medium"}
	if got, err := s.RoomSettings(ctx, "main"); err != nil || got != defaults {
		t.Fatalf("defaults: %v %+v", err, got)
	}
	set := RoomSettings{Name: "main", Access: "invite", Hidden: true, RemoteOwnership: true,
		CenterRemote: true, DefaultRemote: true, DefaultImage: true, DefaultUpload: true, Quality: "medium"}
	for _, want := range []RoomSettings{set, defaults} {
		if err := s.SaveRoomSettings(ctx, want); err != nil {
			t.Fatal(err)
		}
		if got, err := s.RoomSettings(ctx, "main"); err != nil || got != want {
			t.Fatalf("saved: %v %+v, want %+v", err, got, want)
		}
	}
	if got, err := s.RoomSettings(ctx, "other"); err != nil || got != (RoomSettings{Name: "other", Access: "public", Quality: "medium"}) {
		t.Fatalf("other defaults: %v %+v", err, got)
	}
	if err := s.SaveRoomSettings(ctx, RoomSettings{Name: "main", Access: "invalid"}); err == nil {
		t.Fatal("invalid access accepted")
	}
}

func TestValidAccess(t *testing.T) {
	for _, mode := range []string{"public", "account", "verified", "invite", "", "PUBLIC", "invalid"} {
		want := mode == "public" || mode == "account" || mode == "verified" || mode == "invite"
		if ValidAccess(mode) != want {
			t.Errorf("ValidAccess(%q) = %v, want %v", mode, ValidAccess(mode), want)
		}
	}
}
