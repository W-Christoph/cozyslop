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
			w.Write([]byte(`{"data":{"Media":{"id":154587,"title":{"romaji":"Sousou no Frieren","english":null},"episodes":null,"coverImage":{"medium":""}}}}`))
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
	if _, err := c.Get(ctx, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}
