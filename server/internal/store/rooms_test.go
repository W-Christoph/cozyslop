package store

import (
	"context"
	"errors"
	"testing"
)

func TestRoomSettings(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	defaults := RoomSettings{Name: "main", Access: "public"}
	if got, err := s.RoomSettings(ctx, "main"); err != nil || got != defaults {
		t.Fatalf("defaults: %v %+v", err, got)
	}
	set := RoomSettings{Name: "main", Access: "invite", Hidden: true, RemoteOwnership: true,
		DefaultRemote: true, DefaultImage: true, DefaultUpload: true}
	for _, want := range []RoomSettings{set, defaults} {
		if err := s.SaveRoomSettings(ctx, want); err != nil {
			t.Fatal(err)
		}
		if got, err := s.RoomSettings(ctx, "main"); err != nil || got != want {
			t.Fatalf("saved: %v %+v, want %+v", err, got, want)
		}
	}
	if got, err := s.RoomSettings(ctx, "other"); err != nil || got != (RoomSettings{Name: "other", Access: "public"}) {
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

func TestRegisteredRooms(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	user := &User{Username: "creator", Nickname: "Creator", PasswordHash: "hash"}
	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	room := RegisteredRoom{Name: "extra", NekoURL: "http://neko:8080", NekoToken: "secret", CreatedBy: &user.ID}
	if err := s.CreateRegisteredRoom(ctx, &room); err != nil {
		t.Fatal(err)
	}
	if room.CreatedAt == 0 {
		t.Fatal("missing creation time")
	}
	if err := s.CreateRegisteredRoom(ctx, &room); !errors.Is(err, ErrRoomExists) {
		t.Fatalf("duplicate: %v", err)
	}
	room.NekoURL, room.NekoToken = "https://other/neko", "new-secret"
	if err := s.UpdateRegisteredRoom(ctx, room); err != nil {
		t.Fatal(err)
	}
	got, err := s.RegisteredRoom(ctx, room.Name)
	if err != nil || got.Name != room.Name || got.NekoURL != room.NekoURL || got.NekoToken != room.NekoToken || *got.CreatedBy != user.ID || got.CreatedAt != room.CreatedAt {
		t.Fatalf("get: %+v %v", got, err)
	}
	list, err := s.RegisteredRooms(ctx)
	if err != nil || len(list) != 1 || list[0].NekoToken != room.NekoToken {
		t.Fatalf("list: %+v %v", list, err)
	}
	if err := s.DeleteUser(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	got, err = s.RegisteredRoom(ctx, room.Name)
	if err != nil || got.CreatedBy != nil {
		t.Fatalf("creator deletion: %+v %v", got, err)
	}
	settings := RoomSettings{Name: room.Name, Access: "invite", DefaultRemote: true}
	if err := s.SaveRoomSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	message := ChatMessage{Room: room.Name, Identity: "a:guest", Nickname: "Guest", Anonymous: true, Type: "text", Body: "preserved"}
	if err := s.InsertChat(ctx, &message); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteRegisteredRoom(ctx, room.Name); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteRegisteredRoom(ctx, room.Name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing delete: %v", err)
	}
	if _, err := s.RegisteredRoom(ctx, room.Name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing get: %v", err)
	}
	if err := s.UpdateRegisteredRoom(ctx, room); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing update: %v", err)
	}
	room.CreatedBy = nil
	if err := s.CreateRegisteredRoom(ctx, &room); err != nil {
		t.Fatal(err)
	}
	if got, err := s.RoomSettings(ctx, room.Name); err != nil || got != settings {
		t.Fatalf("settings lost: %+v %v", got, err)
	}
	history, err := s.ChatHistory(ctx, room.Name)
	if err != nil || len(history) != 1 || history[0].Body != "preserved" {
		t.Fatalf("history lost: %+v %v", history, err)
	}
}
