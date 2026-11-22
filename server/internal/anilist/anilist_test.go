package anilist

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchAndGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		switch {
		case strings.Contains(req.Query, "Page") && req.Variables["q"] == "frieren":
			w.Write([]byte(`{"data":{"Page":{"media":[{"id":154587,"title":{"romaji":"Sousou no Frieren","english":"Frieren: Beyond Journey's End"},"episodes":28,"seasonYear":2023,"coverImage":{"medium":"https://img/f.jpg"}}]}}}`))
		case req.Variables["id"] == float64(154587):
			w.Write([]byte(`{"data":{"Media":{"id":154587,"title":{"romaji":"Sousou no Frieren","english":null},"episodes":null,"coverImage":{"medium":""},"nextAiringEpisode":{"episode":8,"airingAt":1700000000}}}}`))
		case req.Variables["ids"] != nil:
			if ids, _ := req.Variables["ids"].([]any); len(ids) != 2 || !strings.Contains(req.Query, "id_in") {
				t.Errorf("ids: %v", req.Variables["ids"])
			}
			w.Write([]byte(`{"data":{"Page":{"media":[{"id":154587,"title":{"romaji":"Sousou no Frieren"},"episodes":28,"nextAiringEpisode":null}]}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"errors":[{"message":"Not Found.","status":404}],"data":{"Media":null}}`))
		}
	}))
	defer srv.Close()
	c := New()
	c.URL = srv.URL
	ctx := context.Background()

	found, err := c.Search(ctx, "frieren")
	if err != nil || len(found) != 1 || found[0].ID != 154587 || *found[0].Episodes != 28 || *found[0].SeasonYear != 2023 {
		t.Fatalf("search: %v %+v", err, found)
	}
	if a := found[0].Anime(); a.Title != "Sousou no Frieren" || a.TitleEnglish != "Frieren: Beyond Journey's End" || a.CoverURL != "https://img/f.jpg" {
		t.Fatalf("anime: %+v", a)
	}
	m, err := c.Get(ctx, 154587)
	if err != nil || m.Episodes != nil || m.Title.English != "" {
		t.Fatalf("get: %v %+v", err, m)
	}
	if a := m.Anime(); *a.NextEpisode != 8 || *a.NextAiringAt != 1700000000 {
		t.Fatalf("airing: %+v", a)
	}
	many, err := c.GetMany(ctx, []int64{154587, 2})
	if err != nil || len(many) != 1 || many[0].Anime().NextEpisode != nil {
		t.Fatalf("get many: %v %+v", err, many)
	}
	if _, err := c.Get(ctx, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}
