package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cozycast/internal/store"
	"golang.org/x/crypto/bcrypt"
)

func TestIdentitySessionHash(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	u := &store.User{Username: "alice", Nickname: "Alice"}
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	token := newToken()
	hash := hashToken(token)
	if err := s.CreateSession(ctx, hash, u.ID); err != nil {
		t.Fatal(err)
	}
	a := &Service{store: s}
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	id, err := a.Identify(httptest.NewRecorder(), r)
	if err != nil || id.User == nil || !bytes.Equal(id.SessionHash, hash) {
		t.Fatalf("account identity: %+v, %v", id, err)
	}
	b, err := json.Marshal(id)
	if err != nil || bytes.Contains(b, []byte("SessionHash")) || bytes.Contains(b, []byte(token)) {
		t.Fatalf("serialized identity exposes session: %s, %v", b, err)
	}
	ended, err := a.Logout(httptest.NewRecorder(), r)
	if err != nil || !bytes.Equal(ended, hash) {
		t.Fatalf("logout hash: %x, %v", ended, err)
	}
	id, err = a.Identify(httptest.NewRecorder(), r)
	if err != nil || id.User != nil || len(id.SessionHash) != 0 {
		t.Fatalf("anonymous identity retains session: %+v, %v", id, err)
	}
}

func TestValidatePassword(t *testing.T) {
	for _, tt := range []struct {
		password string
		valid    bool
	}{
		{"", false}, {"1234567", false}, {"12345678", true},
		{strings.Repeat("a", 72), true}, {strings.Repeat("a", 73), false},
		{strings.Repeat("é", 7), false}, {strings.Repeat("é", 36), true},
		{strings.Repeat("é", 37), false}, {strings.Repeat("🙂", 18), true},
		{strings.Repeat("🙂", 19), false},
	} {
		if err := ValidatePassword(tt.password); (err == nil) != tt.valid {
			t.Fatalf("password %q: %v, valid=%v", tt.password, err, tt.valid)
		}
	}
}

func TestCheckPasswordBcryptBoundary(t *testing.T) {
	password := strings.Repeat("a", 72)
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	user := &store.User{PasswordHash: hash}
	if !CheckPassword(user, password) || CheckPassword(user, password+"a") {
		t.Fatal("bcrypt byte limit was not enforced")
	}
	// Imported bcrypt hashes must still verify directly, without pre-hashing.
	imported, err := bcrypt.GenerateFromPassword([]byte("imported password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"$2a$", "$2b$", "$2y$"} {
		user.PasswordHash = prefix + string(imported[4:])
		if !CheckPassword(user, "imported password") {
			t.Fatalf("imported %s hash no longer verifies", prefix)
		}
	}
}

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
