package httpapi_test

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"cozycast/internal/auth"
	"cozycast/internal/diag"
	"cozycast/internal/httpapi"
)

func TestAdminDiagnostics(t *testing.T) {
	a := newAPITest(t)
	d, err := diag.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{Store: a.st, Auth: auth.New(a.st, false), Hub: a.h, Diagnostics: d}).Handler())
	t.Cleanup(srv.Close)
	a.srv = srv
	a.user("root", true)
	admin := a.login("root")

	log := slog.New(diag.NewHandler(slog.DiscardHandler, d))
	log.Info("neko ready", "room", "default")
	log.Warn("anilist search", "err", "timeout", "username", "root")
	d.Add(diag.Entry{Kind: diag.KindOutbound, Level: slog.LevelInfo, Msg: "anilist", Attrs: map[string]any{"op": "search"}})
	d.Sync()

	type entry struct {
		ID    int64          `json:"id"`
		Time  int64          `json:"time"`
		Kind  string         `json:"kind"`
		Level string         `json:"level"`
		Room  string         `json:"room"`
		Msg   string         `json:"msg"`
		Attrs map[string]any `json:"attrs"`
	}
	var out struct {
		Enabled bool    `json:"enabled"`
		Entries []entry `json:"entries"`
	}
	a.call(admin, "GET", "/api/admin/diagnostics", nil, 200, &out)
	if !out.Enabled || len(out.Entries) != 3 || out.Entries[0].Msg != "anilist" || out.Entries[2].Room != "default" {
		t.Fatalf("all: %+v", out)
	}
	if e := out.Entries[1]; e.Level != "WARN" || e.Kind != "event" || e.Attrs["err"] != "timeout" || e.Attrs["username"] != nil ||
		time.Since(time.UnixMilli(e.Time)) > time.Minute {
		t.Errorf("warning: %+v", e)
	}
	a.call(admin, "GET", "/api/admin/diagnostics?kind=event&level=warn", nil, 200, &out)
	if len(out.Entries) != 1 || out.Entries[0].Msg != "anilist search" {
		t.Errorf("warnings: %+v", out.Entries)
	}
	a.call(admin, "GET", "/api/admin/diagnostics?room=default&limit=5", nil, 200, &out)
	if len(out.Entries) != 1 || out.Entries[0].Msg != "neko ready" {
		t.Errorf("room: %+v", out.Entries)
	}
	a.error(admin, "GET", "/api/admin/diagnostics?level=loud", nil, 400, "Unknown level.")
	a.error(admin, "GET", "/api/admin/diagnostics?since=yesterday", nil, 400, "Invalid since.")

	var sum struct {
		Enabled  bool  `json:"enabled"`
		BucketMs int64 `json:"bucketMs"`
		Counts   struct {
			Warnings int `json:"warnings"`
		} `json:"counts"`
		Samples []any `json:"samples"`
	}
	a.call(admin, "GET", "/api/admin/diagnostics/summary?hours=6", nil, 200, &sum)
	if !sum.Enabled || sum.Counts.Warnings != 1 || sum.BucketMs != 2*60_000 || sum.Samples == nil {
		t.Errorf("summary: %+v", sum)
	}
	a.error(admin, "GET", "/api/admin/diagnostics/summary?hours=200", nil, 400, "Invalid hours.")

	a.call(admin, "DELETE", "/api/admin/diagnostics", nil, 204, nil)
	d.Sync()
	a.call(admin, "GET", "/api/admin/diagnostics?kind=outbound", nil, 200, &out)
	if len(out.Entries) != 0 {
		t.Errorf("after clearing: %+v", out.Entries)
	}
}

func TestAdminDiagnosticsOff(t *testing.T) {
	a := newAPITest(t)
	a.user("root", true)
	var out struct {
		Enabled bool  `json:"enabled"`
		Entries []any `json:"entries"`
	}
	a.call(a.login("root"), "GET", "/api/admin/diagnostics", nil, 200, &out)
	if out.Enabled || out.Entries == nil || len(out.Entries) != 0 {
		t.Errorf("without diagnostics: %+v", out)
	}
	var sum struct {
		Enabled bool  `json:"enabled"`
		AniList []any `json:"anilist"`
	}
	a.call(a.login("root"), "GET", "/api/admin/diagnostics/summary", nil, 200, &sum)
	if sum.Enabled || sum.AniList == nil {
		t.Errorf("summary without diagnostics: %+v", sum)
	}
}
