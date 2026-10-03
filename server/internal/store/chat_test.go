package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"
)

func insertChat(t *testing.T, s *Store, room, author, kind, media string) ChatMessage {
	t.Helper()
	m := ChatMessage{Room: room, Identity: author, Nickname: "Anonymous", NameColor: "#abc",
		Anonymous: true, Type: kind, Body: "hello", Media: media}
	if err := s.InsertChat(context.Background(), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestChatHistory(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	now := time.Unix(10000, 123000000)
	s.now = func() time.Time { return now }
	if got, err := s.ChatHistory(ctx, "main"); err != nil || len(got) != 0 {
		t.Fatalf("empty history: %v %+v", err, got)
	}
	first := insertChat(t, s, "main", "a:first", "text", "")
	if first.ID == 0 || first.CreatedAtMs != now.UnixMilli() {
		t.Fatalf("generated fields: %+v", first)
	}
	now = now.Add(time.Millisecond)
	second := insertChat(t, s, "main", "a:second", "image", "image.png")
	other := insertChat(t, s, "other", "a:other", "video", "video.mp4")
	if got, err := s.ChatHistory(ctx, "main"); err != nil || !reflect.DeepEqual(got, []ChatMessage{first, second}) {
		t.Fatalf("oldest first, round trip: %v %+v", err, got)
	}
	if got, err := s.ChatHistory(ctx, "other"); err != nil || !reflect.DeepEqual(got, []ChatMessage{other}) {
		t.Fatalf("independent room: %v %+v", err, got)
	}
	now = time.UnixMilli(first.CreatedAtMs).Add(ChatRetention)
	if got, err := s.ChatHistory(ctx, "main"); err != nil || !reflect.DeepEqual(got, []ChatMessage{second}) {
		t.Fatalf("retention boundary: %v %+v", err, got)
	}
	now = now.Add(time.Millisecond)
	if got, err := s.ChatHistory(ctx, "main"); err != nil || len(got) != 0 {
		t.Fatalf("expired history: %v %+v", err, got)
	}
}

func TestChatHistoryCap(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	s.now = func() time.Time { return time.Unix(10000, 0) }
	var messages []ChatMessage
	for j := 0; j < ChatMaxMessages+3; j++ {
		messages = append(messages, insertChat(t, s, "main", "a:author", "text", ""))
	}
	other := insertChat(t, s, "other", "a:author", "text", "")
	if got, err := s.ChatHistory(ctx, "main"); err != nil || !reflect.DeepEqual(got, messages[3:]) {
		t.Fatalf("newest 1000, oldest first: %v count=%d", err, len(got))
	}
	if got, err := s.ChatHistory(ctx, "other"); err != nil || !reflect.DeepEqual(got, []ChatMessage{other}) {
		t.Fatalf("room cap independent: %v %+v", err, got)
	}
}

func TestEditChat(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	text := insertChat(t, s, "main", "a:author", "text", "")
	image := insertChat(t, s, "main", "a:author", "image", "image.png")
	video := insertChat(t, s, "main", "a:author", "video", "video.mp4")
	for _, tt := range []struct {
		room   string
		id     int64
		author string
	}{
		{"main", text.ID, "a:other"},
		{"main", text.ID, ""},
		{"other", text.ID, "a:author"},
		{"main", image.ID, "a:author"},
		{"main", video.ID, "a:author"},
		{"main", 99999, "a:author"},
	} {
		if err := s.EditChat(ctx, tt.room, tt.id, tt.author, "changed"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unauthorized or non-text edit %+v: %v", tt, err)
		}
	}
	if got, err := s.ChatHistory(ctx, "main"); err != nil || !reflect.DeepEqual(got, []ChatMessage{text, image, video}) {
		t.Fatalf("failed edits changed messages: %v %+v", err, got)
	}
	if err := s.EditChat(ctx, "main", text.ID, "a:author", "changed"); err != nil {
		t.Fatal(err)
	}
	text.Body, text.Edited = "changed", true
	if got, err := s.ChatHistory(ctx, "main"); err != nil || !reflect.DeepEqual(got, []ChatMessage{text, image, video}) {
		t.Fatalf("body and edited flag only: %v %+v", err, got)
	}
}

func TestDeleteChat(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	text := insertChat(t, s, "main", "a:author", "text", "")
	image := insertChat(t, s, "main", "a:author", "image", "image.png")
	video := insertChat(t, s, "main", "a:other", "video", "video.mp4")
	for _, tt := range []struct {
		room, author string
		id           int64
	}{
		{"main", "a:other", image.ID},
		{"other", "", image.ID},
		{"main", "", 99999},
	} {
		if media, err := s.DeleteChat(ctx, tt.room, tt.id, tt.author); !errors.Is(err, ErrNotFound) || media != "" {
			t.Fatalf("refused delete %+v: media=%q err=%v", tt, media, err)
		}
	}
	if got, err := s.ChatHistory(ctx, "main"); err != nil || !reflect.DeepEqual(got, []ChatMessage{text, image, video}) {
		t.Fatalf("failed deletes changed messages: %v %+v", err, got)
	}
	for _, tt := range []struct {
		m      ChatMessage
		author string
	}{{text, "a:author"}, {image, "a:author"}, {video, ""}} {
		if media, err := s.DeleteChat(ctx, "main", tt.m.ID, tt.author); err != nil || media != tt.m.Media {
			t.Fatalf("delete returns media: media=%q err=%v", media, err)
		}
		if _, err := s.DeleteChat(ctx, "main", tt.m.ID, ""); !errors.Is(err, ErrNotFound) {
			t.Fatalf("deleted twice: %v", err)
		}
	}
	if got, err := s.ChatHistory(ctx, "main"); err != nil || len(got) != 0 {
		t.Fatalf("deleted history: %v %+v", err, got)
	}
}

func TestPruneChat(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	now := time.Unix(10000, 0)
	s.now = func() time.Time { return now }
	var removed []ChatMessage
	var wantMedia []string
	for _, room := range []string{"main", "other"} {
		m := insertChat(t, s, room, "a:author", "image", room+"-old.png")
		removed = append(removed, m)
		wantMedia = append(wantMedia, m.Media)
		removed = append(removed, insertChat(t, s, room, "a:author", "text", ""))
	}
	// Messages exactly at the retention cutoff must also be pruned.
	now = now.Add(ChatRetention)
	kept := make(map[string][]ChatMessage)
	for _, room := range []string{"main", "other"} {
		for j := 0; j < ChatMaxMessages+2; j++ {
			media, kind := "", "text"
			if j == 0 || j == 2 {
				media, kind = fmt.Sprintf("%s-%d.png", room, j), "image"
			}
			m := insertChat(t, s, room, "a:author", kind, media)
			if j < 2 {
				removed = append(removed, m)
				if media != "" {
					wantMedia = append(wantMedia, media)
				}
			} else {
				kept[room] = append(kept[room], m)
			}
		}
	}
	gotMedia, err := s.PruneChat(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(gotMedia)
	sort.Strings(wantMedia)
	if !reflect.DeepEqual(gotMedia, wantMedia) {
		t.Fatalf("pruned media: %v, want %v", gotMedia, wantMedia)
	}
	for room, want := range kept {
		if got, err := s.ChatHistory(ctx, room); err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("retained %s: %v count=%d", room, err, len(got))
		}
	}
	// History hides old/over-cap rows already. Deletion confirms pruning
	// physically removed them, including messages whose media name was empty.
	for _, m := range removed {
		if _, err := s.DeleteChat(ctx, m.Room, m.ID, ""); !errors.Is(err, ErrNotFound) {
			t.Fatalf("pruned row still exists: id=%d err=%v", m.ID, err)
		}
	}
	if got, err := s.PruneChat(ctx); err != nil || len(got) != 0 {
		t.Fatalf("second prune: %v %v", err, got)
	}
}
