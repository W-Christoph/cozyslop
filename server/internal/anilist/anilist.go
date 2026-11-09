// Package anilist looks up anime on AniList (https://anilist.co), whose
// GraphQL API answers reads without an account.
package anilist

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"cozycast/internal/store"
)

var ErrNotFound = errors.New("anilist: no such anime")

const DefaultURL = "https://graphql.anilist.co"

type Client struct {
	URL  string
	HTTP *http.Client
}

func New() *Client {
	return &Client{URL: DefaultURL, HTTP: &http.Client{Timeout: 10 * time.Second}}
}

// Media is an anime as AniList describes it.
type Media struct {
	ID    int64 `json:"id"`
	Title struct {
		Romaji  string `json:"romaji"`
		English string `json:"english"`
	} `json:"title"`
	Episodes   *int `json:"episodes"`
	SeasonYear *int `json:"seasonYear"`
	CoverImage struct {
		Medium string `json:"medium"`
	} `json:"coverImage"`
}

// Anime is m as the watchlog stores it.
func (m Media) Anime() *store.Anime {
	return &store.Anime{ID: m.ID, Title: m.Title.Romaji, TitleEnglish: m.Title.English,
		Episodes: m.Episodes, CoverURL: m.CoverImage.Medium}
}

const fields = "id title { romaji english } episodes seasonYear coverImage { medium }"

// Search returns up to 10 anime matching text, best match first.
func (c *Client) Search(ctx context.Context, text string) ([]Media, error) {
	var out struct {
		Page struct {
			Media []Media `json:"media"`
		} `json:"Page"`
	}
	err := c.query(ctx, `query ($q: String) { Page(perPage: 10) { media(search: $q, type: ANIME, sort: SEARCH_MATCH) { `+fields+` } } }`,
		map[string]any{"q": text}, &out)
	return out.Page.Media, err
}

// Get returns the anime with AniList id.
func (c *Client) Get(ctx context.Context, id int64) (Media, error) {
	var out struct {
		Media *Media `json:"Media"`
	}
	err := c.query(ctx, `query ($id: Int) { Media(id: $id, type: ANIME) { `+fields+` } }`,
		map[string]any{"id": id}, &out)
	if err == nil && out.Media == nil {
		err = ErrNotFound
	}
	if err != nil {
		return Media{}, err
	}
	return *out.Media, nil
}

func (c *Client) query(ctx context.Context, query string, vars map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var answer struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
			Status  int    `json:"status"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&answer); err != nil {
		return fmt.Errorf("anilist: %s: %w", res.Status, err)
	}
	if len(answer.Errors) > 0 {
		if answer.Errors[0].Status == http.StatusNotFound {
			return ErrNotFound
		}
		return fmt.Errorf("anilist: %s: %s", res.Status, answer.Errors[0].Message)
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("anilist: %s", res.Status)
	}
	return json.Unmarshal(answer.Data, out)
}
