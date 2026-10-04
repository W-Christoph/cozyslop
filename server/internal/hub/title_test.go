package hub

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"cozycast/internal/store"
)

func TestWindowTitle(t *testing.T) {
	var mu sync.Mutex
	title, status, asked := "YouTube — Mozilla Firefox", http.StatusOK, 0
	desktop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		asked++
		w.WriteHeader(status)
		_, _ = w.Write([]byte(title))
	}))
	defer desktop.Close()
	set := func(t string, s int) { mu.Lock(); title, status = t, s; mu.Unlock() }

	f := newFixture(t, store.RoomSettings{})
	f.r.titleURL = desktop.URL

	// Nobody is in the room: the desktop is not asked.
	f.r.checkTitle(f.ctx)
	if asked != 0 {
		t.Fatal("asked for the title of an empty room")
	}

	_, alice := f.join(anon("alice"))
	requireEqual(t, alice.wait(t, "welcome").(welcomeMsg).WindowTitle, "")
	f.r.checkTitle(f.ctx)
	requireEqual(t, alice.wait(t, "window_title").(windowTitleMsg).Title, "YouTube — Mozilla Firefox")
	// Unchanged: nothing is sent again.
	f.r.checkTitle(f.ctx)
	requireEqual(t, alice.count("window_title"), 1)

	// Someone joining later is told at once.
	_, bob := f.join(anon("bobby"))
	requireEqual(t, bob.wait(t, "welcome").(welcomeMsg).WindowTitle, "YouTube — Mozilla Firefox")

	// The title is whatever a window was named: it is cleaned up.
	set("  evil\x00\n<b>title</b>\t"+strings.Repeat("x", 500)+" \xff", http.StatusOK)
	f.r.checkTitle(f.ctx)
	got := alice.wait(t, "window_title").(windowTitleMsg).Title
	if !strings.HasPrefix(got, "evil <b>title</b> xxx") || !strings.HasSuffix(got, "x…") || len([]rune(got)) != maxTitleRunes {
		t.Fatalf("cleaned title %q (%d runes)", got, len([]rune(got)))
	}
	requireEqual(t, bob.wait(t, "window_title").(windowTitleMsg).Title, got)

	// A desktop that does not answer properly has no title.
	set("not found", http.StatusNotFound)
	f.r.checkTitle(f.ctx)
	requireEqual(t, alice.wait(t, "window_title").(windowTitleMsg).Title, "")
	desktop.Close()
	f.r.checkTitle(f.ctx)
	requireEqual(t, alice.count("window_title"), 3)
}

func TestCleanTitle(t *testing.T) {
	for in, want := range map[string]string{
		"":                        "",
		"  Downloads - Thunar \n": "Downloads - Thunar",
		"a\r\nb\x1b[31m c":        "a b [31m c",
		"日本語 — Firefox":           "日本語 — Firefox",
		strings.Repeat("é", 200):  strings.Repeat("é", 200),
		strings.Repeat("é", 201):  strings.Repeat("é", 199) + "…",
		"bad \xff\xfe bytes":      "bad bytes",
	} {
		if got := cleanTitle(in); got != want {
			t.Errorf("cleanTitle(%q) = %q, want %q", in, got, want)
		}
	}
}
