package httpapi_test

import (
	"context"
	"cozycast/internal/httpapi"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

const resetError = "This reset link is invalid or has expired. Ask a moderator for a new link."

func TestPasswordResetLink(t *testing.T) {
	a := newAPITest(t)
	a.user("root", true)
	a.user("alice", false)
	admin, user, other, anon := a.login("root"), a.login("alice"), a.login("alice"), a.client()
	var link struct {
		Token, Path string
		ExpiresAt   int64
	}
	a.call(admin, "POST", "/api/admin/users/alice/password-reset", nil, 201, &link)
	if len(link.Token) != 43 || link.Path != "/reset/"+link.Token || link.ExpiresAt < time.Now().Unix()+86390 || link.ExpiresAt > time.Now().Unix()+86400 {
		t.Fatalf("link metadata: %+v", link)
	}
	old := link.Token
	a.call(admin, "POST", "/api/admin/users/alice/password-reset", nil, 201, &link)
	for _, endpoint := range []string{"check", "redeem"} {
		body := map[string]string{"token": old}
		if endpoint == "redeem" {
			body["password"] = "newpassword"
		}
		a.error(anon, "POST", "/api/auth/password-reset/"+endpoint, body, 404, resetError)
	}
	var check struct {
		Valid    bool
		Username string
	}
	a.call(anon, "POST", "/api/auth/password-reset/check", map[string]string{"token": link.Token}, 200, &check)
	if !check.Valid || check.Username != "alice" {
		t.Fatalf("check: %+v", check)
	}
	for _, password := range []string{"short", strings.Repeat("a", 73), strings.Repeat("é", 37)} {
		a.error(anon, "POST", "/api/auth/password-reset/redeem", map[string]string{"token": link.Token, "password": password}, 400, passwordError)
	}
	a.call(anon, "POST", "/api/auth/password-reset/redeem", map[string]string{"token": link.Token, "password": "newpassword"}, 204, nil)
	a.me(user, "")
	a.me(other, "")
	a.me(anon, "")
	a.error(anon, "POST", "/api/auth/password-reset/check", map[string]string{"token": link.Token}, 404, resetError)
	a.error(anon, "POST", "/api/auth/password-reset/redeem", map[string]string{"token": link.Token, "password": "newpassword"}, 404, resetError)
	a.error(anon, "POST", "/api/auth/login", map[string]string{"username": "alice", "password": testPassword}, 401, "Wrong username or password.")
	a.call(anon, "POST", "/api/auth/login", map[string]string{"username": "alice", "password": "newpassword"}, 200, nil)
	a.call(admin, "POST", "/api/admin/users/missing/password-reset", nil, 404, nil)
}

func TestPasswordChangeInvalidatesResetLink(t *testing.T) {
	for _, own := range []bool{false, true} {
		t.Run(map[bool]string{false: "admin", true: "own"}[own], func(t *testing.T) {
			a := newAPITest(t)
			a.user("root", true)
			a.user("alice", false)
			admin, user := a.login("root"), a.login("alice")
			var link struct{ Token string }
			a.call(admin, "POST", "/api/admin/users/alice/password-reset", nil, 201, &link)
			if own {
				a.call(user, "POST", "/api/me/password", map[string]string{"current": testPassword, "new": "newpassword"}, 204, nil)
			} else {
				a.call(admin, "POST", "/api/admin/users/alice/password", map[string]string{"password": "newpassword"}, 204, nil)
			}
			a.error(a.client(), "POST", "/api/auth/password-reset/redeem", map[string]string{"token": link.Token, "password": "newpassword"}, 404, resetError)
		})
	}
}

func TestResetLinkAdminDisabledAndBannedAccounts(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()
	a.user("root", true)
	u := a.user("second", true)
	admin := a.login("root")
	if err := a.st.UpdateFlags(ctx, u.ID, true, false, true); err != nil {
		t.Fatal(err)
	}
	if err := a.st.BanUser(ctx, "default", u.ID, nil); err != nil {
		t.Fatal(err)
	}
	var link struct{ Token string }
	a.call(admin, "POST", "/api/admin/users/second/password-reset", nil, 201, &link)
	a.call(a.client(), "POST", "/api/auth/password-reset/check", map[string]string{"token": link.Token}, 200, nil)
	a.call(a.client(), "POST", "/api/auth/password-reset/redeem", map[string]string{"token": link.Token, "password": "newpassword"}, 204, nil)
	got, err := a.st.UserByID(ctx, u.ID)
	if err != nil || !got.Disabled || !got.Admin {
		t.Fatalf("flags: %+v %v", got, err)
	}
	perm, err := a.st.Permission(ctx, "default", u.ID)
	if err != nil || !perm.Banned {
		t.Fatalf("ban: %+v %v", perm, err)
	}
	a.call(a.client(), "POST", "/api/auth/login", map[string]string{"username": "second", "password": "newpassword"}, 401, nil)
}

func TestPasswordResetRateLimit(t *testing.T) {
	for _, endpoint := range []string{"check", "redeem"} {
		t.Run(endpoint, func(t *testing.T) {
			a := newAPITest(t)
			c := a.client()
			body := map[string]string{"token": "missing"}
			if endpoint == "redeem" {
				body["password"] = "newpassword"
			}
			for i := 0; i < 10; i++ {
				a.error(c, "POST", "/api/auth/password-reset/"+endpoint, body, 404, resetError)
			}
			a.call(c, "POST", "/api/auth/password-reset/"+endpoint, body, 429, nil)
		})
	}
}

func TestResetPageTokenPrivacy(t *testing.T) {
	handler := httpapi.New(httpapi.Deps{Web: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>reset</html>")}}}).Handler()
	for _, method := range []string{"GET", "HEAD"} {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(method, "/reset/private-token", nil))
		if res.Code != 200 || res.Header().Get("Referrer-Policy") != "no-referrer" || res.Header().Get("Cache-Control") != "no-store" || !strings.Contains(res.Header().Get("Content-Security-Policy"), "connect-src 'self'") {
			t.Fatalf("page privacy: %d %v", res.Code, res.Header())
		}
	}
}
