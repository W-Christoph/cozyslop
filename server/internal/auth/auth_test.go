package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnonIDIsNotTheCookie(t *testing.T) {
	a := &Service{}
	identify := func(cookie string) (Identity, *http.Response) {
		t.Helper()
		r := httptest.NewRequest("GET", "/", nil)
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: anonCookie, Value: cookie})
		}
		w := httptest.NewRecorder()
		id, err := a.Identify(w, r)
		if err != nil {
			t.Fatal(err)
		}
		return id, w.Result()
	}

	first, res := identify("")
	cookies := res.Cookies()
	if len(cookies) != 1 || cookies[0].Name != anonCookie || !validToken(cookies[0].Value) || !cookies[0].HttpOnly {
		t.Fatalf("new browser cookies: %+v", cookies)
	}
	token := cookies[0].Value
	if first.AnonID == "" || strings.Contains(first.Key(), token) || validToken(first.AnonID) {
		t.Fatalf("anon id %q exposes or looks like the cookie", first.AnonID)
	}

	// The same browser keeps its id and gets no new cookie.
	again, res := identify(token)
	if again.AnonID != first.AnonID || len(res.Cookies()) != 0 {
		t.Fatalf("returning browser: id %q want %q, cookies %+v", again.AnonID, first.AnonID, res.Cookies())
	}

	// Presenting someone's public id as the cookie does not make you them.
	thief, res := identify(first.AnonID)
	if thief.AnonID == first.AnonID || len(res.Cookies()) != 1 {
		t.Fatalf("public id accepted as a cookie: %q", thief.AnonID)
	}
	other, _ := identify(newToken())
	if other.AnonID == first.AnonID {
		t.Fatal("two browsers share an anon id")
	}
}
