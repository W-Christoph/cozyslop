package store_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cozycast/internal/auth"
	"cozycast/internal/store"
)

func TestIdentifyRenewsSessionOnTouch(t *testing.T) {
	for _, mode := range []string{"http", "https", "proxy https"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			s, err := store.Open(ctx, ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			base := time.Unix(1700000000, 0)
			now := base
			store.SetClock(t, s, func() time.Time { return now })
			u := &store.User{Username: "alice", PasswordHash: "hash", Nickname: "Alice"}
			if err := s.CreateUser(ctx, u); err != nil {
				t.Fatal(err)
			}
			a := auth.New(s, mode == "proxy https")
			request := func() *http.Request {
				t.Helper()
				url := "http://example.com/"
				if mode == "https" {
					url = "https://example.com/"
				}
				r := httptest.NewRequest(http.MethodGet, url, nil)
				if mode == "proxy https" {
					r.Header.Set("X-Forwarded-Proto", "https")
				}
				r.AddCookie(&http.Cookie{Name: "cozy_anon", Value: strings.Repeat("A", 43)})
				return r
			}
			w := httptest.NewRecorder()
			if err := a.StartSession(w, request(), u); err != nil {
				t.Fatal(err)
			}
			cookies := w.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatalf("login cookies: %+v", cookies)
			}
			loginCookie := cookies[0]
			if loginCookie.Name != "cozy_session" || loginCookie.MaxAge != int(store.SessionTTL.Seconds()) ||
				loginCookie.Path != "/" || !loginCookie.HttpOnly || loginCookie.SameSite != http.SameSiteLaxMode ||
				loginCookie.Secure != (mode != "http") {
				t.Fatalf("login cookie attributes: %+v", loginCookie)
			}
			identify := func() (auth.Identity, []*http.Cookie) {
				t.Helper()
				r := request()
				r.AddCookie(loginCookie)
				w := httptest.NewRecorder()
				id, err := a.Identify(w, r)
				if err != nil {
					t.Fatal(err)
				}
				return id, w.Result().Cookies()
			}
			for _, offset := range []time.Duration{0, 59 * time.Minute, time.Hour, time.Hour + time.Second} {
				now = base.Add(offset)
				id, cookies := identify()
				if id.User == nil || id.User.ID != u.ID {
					t.Fatalf("identity at %s: %+v", offset, id)
				}
				if offset == time.Hour {
					if len(cookies) != 1 || cookies[0].String() != loginCookie.String() {
						t.Fatalf("renewal differs from login: %+v", cookies)
					}
				} else if len(cookies) != 0 {
					t.Fatalf("cookie renewed before touch at %s: %+v", offset, cookies)
				}
			}
			// The touch extended database validity beyond the original expiry.
			now = base.Add(store.SessionTTL + time.Minute)
			if id, cookies := identify(); id.User == nil || len(cookies) != 1 || cookies[0].MaxAge <= 0 {
				t.Fatalf("sliding database expiry: %+v %+v", id, cookies)
			}
			now = now.Add(store.SessionTTL)
			if id, cookies := identify(); id.User != nil || len(cookies) != 1 || cookies[0].MaxAge != -1 {
				t.Fatalf("expired session was renewed: %+v %+v", id, cookies)
			}
			if err := s.DeleteExpiredSessions(ctx); err != nil {
				t.Fatal(err)
			}
			if id, cookies := identify(); id.User != nil || len(cookies) != 1 || cookies[0].MaxAge != -1 {
				t.Fatalf("missing session was renewed: %+v %+v", id, cookies)
			}
			r := request()
			w = httptest.NewRecorder()
			if _, err := a.Identify(w, r); err != nil || len(w.Result().Cookies()) != 0 {
				t.Fatalf("request without session cookie: %v %+v", err, w.Result().Cookies())
			}
		})
	}
}
