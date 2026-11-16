package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"cozycast/internal/anilist"
	"cozycast/internal/auth"
	"cozycast/internal/httpapi"
	"cozycast/internal/hub"
	"cozycast/internal/neko"
	"cozycast/internal/store"
)

func newWatchlogTest(t *testing.T) *apiTest {
	t.Helper()
	ani := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		switch {
		case strings.Contains(req.Query, "Page"):
			w.Write([]byte(`{"data":{"Page":{"media":[{"id":154587,"title":{"romaji":"Sousou no Frieren","english":"Frieren"},"episodes":28,"seasonYear":2023,"coverImage":{"medium":"f.jpg"}}]}}}`))
		case req.Variables["id"] == float64(154587):
			w.Write([]byte(`{"data":{"Media":{"id":154587,"title":{"romaji":"Sousou no Frieren","english":"Frieren"},"episodes":28,"coverImage":{"medium":"f.jpg"}}}}`))
		default:
			w.Write([]byte(`{"errors":[{"message":"Not Found.","status":404}]}`))
		}
	}))
	t.Cleanup(ani.Close)
	client := anilist.New()
	client.URL = ani.URL

	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	wl, err := store.OpenWatchlog(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { wl.Close() })
	nc, err := neko.NewClient("http://127.0.0.1:1", "x", nil)
	if err != nil {
		t.Fatal(err)
	}
	h := hub.New(st, t.TempDir(), []hub.RoomConfig{{Name: "default", Neko: nc}})
	h.Watchlog = wl
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{Store: st, Auth: auth.New(st, false), Hub: h, AniList: client, Watchlog: wl}).Handler())
	t.Cleanup(srv.Close)
	return &apiTest{t: t, st: st, srv: srv, h: h}
}

func TestWatchlog(t *testing.T) {
	a := newWatchlogTest(t)
	a.user("admin", true)
	a.user("alice", false)
	a.user("bob", false)
	admin, alice := a.login("admin"), a.login("alice")

	a.error(a.client(), "GET", "/api/watchlog", nil, 401, "Please log in.")
	a.error(alice, "GET", "/api/watchlog", nil, 404, "The watchlog is off.")
	a.call(admin, "PUT", "/api/admin/settings", map[string]any{"message": "", "registration": "invite", "watchlog": true}, 200, nil)
	var settings struct{ Watchlog bool }
	a.call(alice, "GET", "/api/settings", nil, 200, &settings)
	if !settings.Watchlog {
		t.Fatal("setting not on")
	}

	var found []struct {
		ID       int64
		Title    string
		Episodes int
		Year     int
	}
	a.call(alice, "GET", "/api/watchlog/search?q=frieren", nil, 200, &found)
	if len(found) != 1 || found[0].ID != 154587 || found[0].Episodes != 28 || found[0].Year != 2023 {
		t.Fatalf("search: %+v", found)
	}
	a.error(alice, "GET", "/api/watchlog/search?q=f", nil, 400, "Search for 2 to 100 characters.")
	a.error(alice, "POST", "/api/watchlog/anime", map[string]any{"id": 1}, 404, "AniList has no such anime.")
	a.call(alice, "POST", "/api/watchlog/anime", map[string]any{"id": 154587}, 200, nil)

	// Alice watched 1 to 5 but not 3.
	var changed store.Progress
	a.call(alice, "PUT", "/api/watchlog/progress/154587/alice", map[string]any{"from": 1, "to": 5, "watched": true}, 200, nil)
	a.call(alice, "PUT", "/api/watchlog/progress/154587/alice", map[string]any{"from": 3, "watched": false}, 200, &changed)
	if changed.Username != "alice" || changed.AnimeID != 154587 || !reflect.DeepEqual(changed.Watched, [][2]int{{1, 2}, {4, 5}}) {
		t.Fatalf("changed: %+v", changed)
	}
	for _, bad := range []map[string]any{{"from": 29, "watched": true}, {"from": 0, "watched": true}, {"from": 5, "to": 4, "watched": true}, {"from": 1, "to": 10000, "watched": false}} {
		a.error(alice, "PUT", "/api/watchlog/progress/154587/alice", bad, 400, "The show has no such episode.")
	}
	a.error(alice, "PUT", "/api/watchlog/progress/154587/bob", map[string]any{"from": 1, "watched": true}, 403, "You can only change your own progress.")
	a.error(alice, "PUT", "/api/watchlog/progress/1/alice", map[string]any{"from": 1, "watched": true}, 404, "That show is not on the watchlog.")
	a.call(admin, "PUT", "/api/watchlog/progress/154587/bob", map[string]any{"from": 2, "watched": true}, 200, nil)
	a.error(admin, "PUT", "/api/watchlog/progress/154587/nobody", map[string]any{"from": 2, "watched": true}, 404, "Unknown user.")

	var log struct {
		Anime    []store.Anime
		People   []struct{ Username, AvatarURL string }
		Progress []store.Progress
	}
	a.call(alice, "GET", "/api/watchlog", nil, 200, &log)
	if len(log.Anime) != 1 || log.Anime[0].Title != "Sousou no Frieren" || len(log.People) != 2 ||
		log.People[0].Username != "alice" || len(log.Progress) != 2 || log.Progress[1].Username != "bob" ||
		!reflect.DeepEqual(log.Progress[1].Watched, [][2]int{{2, 2}}) {
		t.Fatalf("watchlog: %+v", log)
	}

	// Bob stopped following it: alice may not say so, bob and admins may.
	a.error(alice, "PUT", "/api/watchlog/follow/154587/bob", map[string]any{"mode": "dropped"}, 403, "You can only change your own progress.")
	a.error(admin, "PUT", "/api/watchlog/follow/1/bob", map[string]any{"mode": "dropped"}, 404, "That show is not on the watchlog.")
	a.error(admin, "PUT", "/api/watchlog/follow/154587/bob", map[string]any{"mode": "sometimes"}, 400, "The mode is following, casual or dropped.")
	var following struct {
		Follows  []store.Follow
		Progress []store.Progress
	}
	// Then he watches along casually, and then follows it again.
	for _, mode := range []string{"dropped", "casual"} {
		a.call(admin, "PUT", "/api/watchlog/follow/154587/bob", map[string]any{"mode": mode}, 204, nil)
		a.call(alice, "GET", "/api/watchlog", nil, 200, &following)
		if len(following.Follows) != 1 || following.Follows[0].Username != "bob" || following.Follows[0].AnimeID != 154587 ||
			following.Follows[0].Mode != mode || following.Progress[0].WatchedAt == 0 {
			t.Fatalf("%s: %+v", mode, following)
		}
	}
	a.call(admin, "PUT", "/api/watchlog/follow/154587/bob", map[string]any{"mode": "following"}, 204, nil)
	// Names linked to a show are listed; an admin unlinks one.
	if err := a.h.Watchlog.SaveAlias(context.Background(), "some frieren", 154587); err != nil {
		t.Fatal(err)
	}
	var linked struct{ Aliases []store.Alias }
	a.call(alice, "GET", "/api/watchlog", nil, 200, &linked)
	if len(linked.Aliases) != 1 || linked.Aliases[0] != (store.Alias{Alias: "some frieren", AnimeID: 154587}) {
		t.Fatalf("aliases: %+v", linked)
	}
	a.error(alice, "DELETE", "/api/watchlog/aliases/some%20frieren", nil, 403, "Only admins can unlink a name.")
	a.call(admin, "DELETE", "/api/watchlog/aliases/some%20frieren", nil, 204, nil)
	a.error(admin, "DELETE", "/api/watchlog/aliases/some%20frieren", nil, 404, "That name is not linked to a show.")
	a.call(alice, "GET", "/api/watchlog", nil, 200, &following)
	if len(following.Follows) != 0 {
		t.Fatalf("follows still: %+v", following)
	}

	// Unticking past the show's end is fine; with everything unticked the
	// account is no longer listed.
	a.call(alice, "PUT", "/api/watchlog/progress/154587/alice", map[string]any{"from": 1, "to": 9999, "watched": false}, 200, &changed)
	if len(changed.Watched) != 0 {
		t.Fatalf("cleared: %+v", changed)
	}
	// Bob has watched some of it: only an admin removes it.
	a.error(alice, "DELETE", "/api/watchlog/anime/154587", nil, 403, "Someone has watched this show: only admins can remove it.")
	a.call(admin, "DELETE", "/api/watchlog/anime/154587", nil, 204, nil)
	a.call(alice, "GET", "/api/watchlog", nil, 200, &log)
	if len(log.Anime) != 0 || len(log.Progress) != 0 {
		t.Fatalf("after delete: %+v", log)
	}
	// A show nobody has watched is anyone's to remove.
	a.call(alice, "POST", "/api/watchlog/anime", map[string]any{"id": 154587}, 200, nil)
	a.call(alice, "DELETE", "/api/watchlog/anime/154587", nil, 204, nil)
}
