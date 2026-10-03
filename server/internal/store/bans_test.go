package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAnonBans(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	now := int64(1000)
	s.now = func() time.Time { return time.Unix(now, 0) }
	if _, err := s.AnonBan(ctx, "main", "a", "ip"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if got, err := s.ListAnonBans(ctx, ""); err != nil || len(got) != 0 {
		t.Fatalf("empty: %v %+v", err, got)
	}
	short, long, expired, boundary := int64(1100), int64(1200), int64(999), now
	for _, ban := range []AnonBan{
		{Room: "main", AnonID: "a", IP: "ip1", BannedUntil: &short},
		{Room: "main", AnonID: "b", IP: "ip2", BannedUntil: &long},
		{Room: "main", AnonID: "old", IP: "old", BannedUntil: &expired},
		{Room: "main", AnonID: "boundary", IP: "boundary", BannedUntil: &boundary},
		{Room: "other", AnonID: "a", IP: "ip1"},
	} {
		if err := s.AddAnonBan(ctx, ban.Room, ban.AnonID, ban.IP, ban.BannedUntil); err != nil {
			t.Fatal(err)
		}
	}
	for _, match := range []struct{ anonID, ip string }{{"a", "unmatched"}, {"unmatched", "ip1"}} {
		got, err := s.AnonBan(ctx, "main", match.anonID, match.ip)
		if err != nil || got.AnonID != "a" || got.IP != "ip1" || got.BannedUntil == nil || *got.BannedUntil != short {
			t.Fatalf("match %+v: %v %+v", match, err, got)
		}
	}
	got, err := s.AnonBan(ctx, "main", "a", "ip2")
	if err != nil || got.BannedUntil == nil || *got.BannedUntil != long {
		t.Fatalf("longest: %v %+v", err, got)
	}
	for _, id := range []string{"old", "boundary"} {
		if _, err := s.AnonBan(ctx, "main", id, id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("expired %s: %v", id, err)
		}
	}
	if _, err := s.AnonBan(ctx, "unknown", "a", "ip1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("room isolation: %v", err)
	}
	if err := s.AddAnonBan(ctx, "main", "forever", "ip2", nil); err != nil {
		t.Fatal(err)
	}
	forever, err := s.AnonBan(ctx, "main", "a", "ip2")
	if err != nil || forever.AnonID != "forever" || forever.BannedUntil != nil {
		t.Fatalf("forever wins: %v %+v", err, forever)
	}
	if got, err := s.ListAnonBans(ctx, "main"); err != nil || len(got) != 3 {
		t.Fatalf("room active list: %v %+v", err, got)
	}
	if got, err := s.ListAnonBans(ctx, ""); err != nil || len(got) != 4 {
		t.Fatalf("all active list: %v %+v", err, got)
	}
	if err := s.DeleteExpiredAnonBans(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM anon_bans").Scan(&count); err != nil || count != 4 {
		t.Fatalf("cleanup: %v count=%d", err, count)
	}
	if err := s.DeleteAnonBan(ctx, forever.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAnonBan(ctx, forever.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing: %v", err)
	}
	now = long
	if _, err := s.AnonBan(ctx, "main", "a", "ip2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("time advance: %v", err)
	}
	if err := s.DeleteExpiredAnonBans(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteExpiredAnonBans(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err := s.ListAnonBans(ctx, ""); err != nil || len(got) != 1 || got[0].Room != "other" || got[0].BannedUntil != nil {
		t.Fatalf("forever remains: %v %+v", err, got)
	}
}
