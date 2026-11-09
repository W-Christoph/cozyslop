package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	nc, err := neko.NewClient("http://127.0.0.1:1", "x", nil)
	if err != nil {
		t.Fatal(err)
	}
	h := hub.New(st, t.TempDir(), []hub.RoomConfig{{Name: "default", Neko: nc}})
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{Store: st, Auth: auth.New(st, false), Hub: h, AniList: client}).Handler())
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

	a.call(alice, "PUT", "/api/watchlog/progress/154587/alice", map[string]any{"episode": 5}, 200, nil)
	a.error(alice, "PUT", "/api/watchlog/progress/154587/alice", map[string]any{"episode": 29}, 400, "The show has no such episode.")
	a.error(alice, "PUT", "/api/watchlog/progress/154587/bob", map[string]any{"episode": 1}, 403, "You can only change your own progress.")
	a.error(alice, "PUT", "/api/watchlog/progress/1/alice", map[string]any{"episode": 1}, 404, "That show is not on the watchlog.")
	a.call(admin, "PUT", "/api/watchlog/progress/154587/bob", map[string]any{"episode": 2}, 200, nil)
	a.error(admin, "PUT", "/api/watchlog/progress/154587/nobody", map[string]any{"episode": 2}, 404, "Unknown user.")

	var log struct {
		Anime    []store.Anime
		People   []struct{ Username, AvatarURL string }
		Progress []store.Progress
	}
	a.call(alice, "GET", "/api/watchlog", nil, 200, &log)
	if len(log.Anime) != 1 || log.Anime[0].Title != "Sousou no Frieren" || len(log.People) != 2 ||
		log.People[0].Username != "alice" || len(log.Progress) != 2 || log.Progress[1].Username != "bob" || log.Progress[1].Episode != 2 {
		t.Fatalf("watchlog: %+v", log)
	}

	a.call(alice, "PUT", "/api/watchlog/progress/154587/alice", map[string]any{"episode": 0}, 204, nil)
	a.error(alice, "DELETE", "/api/watchlog/anime/154587", nil, 403, "Admins only.")
	a.call(admin, "DELETE", "/api/watchlog/anime/154587", nil, 204, nil)
	a.call(alice, "GET", "/api/watchlog", nil, 200, &log)
	if len(log.Anime) != 0 || len(log.Progress) != 0 {
		t.Fatalf("after delete: %+v", log)
	}
}
