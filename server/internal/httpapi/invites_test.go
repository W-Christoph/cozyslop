package httpapi_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"cozycast/internal/store"
)

type inviteResult struct {
	store.Invite
	Valid bool   `json:"valid"`
	Path  string `json:"path"`
}

func TestInviteCreateValidation(t *testing.T) {
	a := newAPITest(t)
	a.user("root", true)
	admin := a.login("root")
	for _, tt := range []struct {
		field  string
		value  any
		status int
	}{
		{"room", "missing", 404},
		{"name", strings.Repeat("é", 65), 400},
		{"maxUses", 0, 400},
		{"maxUses", -1, 400},
		{"expiresInMinutes", 0, 400},
		{"expiresInMinutes", -1, 400},
		{"expiresInMinutes", int64(1<<63 - 1), 400},
	} {
		req := map[string]any{"room": "default"}
		req[tt.field] = tt.value
		a.call(admin, "POST", "/api/admin/invites", req, tt.status, nil)
	}
	var list []inviteResult
	a.call(admin, "GET", "/api/admin/invites", nil, 200, &list)
	if len(list) != 0 {
		t.Fatalf("invalid creation persisted: %+v", list)
	}
}

func TestInvites(t *testing.T) {
	a := newAPITest(t)
	a.user("root", true)
	u := a.user("alice", false)
	admin, user, anon := a.login("root"), a.login("alice"), a.client()
	var permanent, temporary inviteResult
	before := time.Now().Unix()
	a.call(admin, "POST", "/api/admin/invites", map[string]any{"room": "default", "name": strings.Repeat("é", 64), "remote": true, "image": true, "upload": true, "maxUses": 1, "expiresInMinutes": 10}, 201, &permanent)
	if permanent.Code == "" || permanent.Room != "default" || permanent.Temporary || !permanent.Valid || permanent.Path != "/invite/"+permanent.Code || permanent.Uses != 0 || permanent.MaxUses == nil || *permanent.MaxUses != 1 || permanent.ExpiresAt == nil || *permanent.ExpiresAt < before+600 || *permanent.ExpiresAt > time.Now().Unix()+600 || permanent.CreatedAt == 0 || !permanent.Remote || !permanent.Image || !permanent.Upload {
		t.Fatalf("account invite: %+v", permanent)
	}
	a.call(admin, "POST", "/api/admin/invites", map[string]any{"room": "default", "temporary": true, "maxUses": nil, "expiresInMinutes": nil}, 201, &temporary)
	if !temporary.Temporary || !temporary.Valid || temporary.Path != "/access/"+temporary.Code || temporary.MaxUses != nil || temporary.ExpiresAt != nil {
		t.Fatalf("temporary invite: %+v", temporary)
	}
	var check struct {
		Room      string `json:"room"`
		Temporary bool   `json:"temporary"`
	}
	for _, i := range []inviteResult{permanent, temporary} {
		a.call(anon, "GET", "/api/invites/"+i.Code, nil, 200, &check)
		if check.Room != "default" || check.Temporary != i.Temporary {
			t.Fatalf("public check: %+v", check)
		}
	}
	a.error(anon, "GET", "/api/invites/missing", nil, 404, "This invite is invalid or has expired.")
	a.call(anon, "POST", "/api/invites/"+permanent.Code+"/redeem", nil, 401, nil)
	a.call(user, "POST", "/api/invites/"+permanent.Code+"/redeem", nil, 204, nil)
	a.call(user, "POST", "/api/invites/"+permanent.Code+"/redeem", nil, 204, nil)
	p, err := a.st.Permission(context.Background(), "default", u.ID)
	if err != nil || !p.Invited || !p.Remote || !p.Image || !p.Upload || p.InviteName != permanent.Name {
		t.Fatalf("redeem grants: %v %+v", err, p)
	}
	i, err := a.st.Invite(context.Background(), permanent.Code)
	if err != nil || i.Uses != 1 {
		t.Fatalf("repeat redemption: %v %+v", err, i)
	}
	a.error(user, "POST", "/api/invites/"+temporary.Code+"/redeem", nil, 404, "This invite is invalid or has expired.")
	a.error(user, "POST", "/api/invites/missing/redeem", nil, 404, "This invite is invalid or has expired.")
	a.error(anon, "GET", "/api/invites/"+permanent.Code, nil, 404, "This invite is invalid or has expired.")
	var list []inviteResult
	for _, path := range []string{"/api/admin/invites", "/api/admin/invites?room=default"} {
		a.call(admin, "GET", path, nil, 200, &list)
		if len(list) != 2 {
			t.Fatalf("invite list: %+v", list)
		}
		for _, got := range list {
			if got.Code == permanent.Code {
				if got.Valid || got.Path != permanent.Path || got.Uses != 1 {
					t.Fatalf("exhausted view: %+v", got)
				}
			} else if got.Code != temporary.Code || !got.Valid || got.Path != temporary.Path {
				t.Fatalf("temporary view: %+v", got)
			}
		}
	}
	a.call(admin, "GET", "/api/admin/invites?room=other", nil, 200, &list)
	if len(list) != 0 {
		t.Fatalf("room filter: %+v", list)
	}
	a.call(admin, "DELETE", "/api/admin/invites/"+temporary.Code, nil, 204, nil)
	a.call(admin, "DELETE", "/api/admin/invites/"+temporary.Code, nil, 404, nil)
	a.call(anon, "GET", "/api/invites/"+temporary.Code, nil, 404, nil)
}

func TestExpiredInvite(t *testing.T) {
	a := newAPITest(t)
	a.user("root", true)
	a.user("alice", false)
	admin, user := a.login("root"), a.login("alice")
	past := time.Now().Unix() - 1
	i := &store.Invite{Room: "default", ExpiresAt: &past}
	if err := a.st.CreateInvite(context.Background(), i); err != nil {
		t.Fatal(err)
	}
	a.call(a.client(), "GET", "/api/invites/"+i.Code, nil, 404, nil)
	a.call(user, "POST", "/api/invites/"+i.Code+"/redeem", nil, 404, nil)
	var list []inviteResult
	a.call(admin, "GET", "/api/admin/invites", nil, 200, &list)
	if len(list) != 1 || list[0].Valid {
		t.Fatalf("expired list: %+v", list)
	}
}
