package rights

import (
	"reflect"
	"testing"

	"cozycast/internal/store"
)

func TestAdmitAccess(t *testing.T) {
	for _, identity := range []struct {
		name    string
		in      Input
		allowed [4]bool // public, account, verified, invite
	}{
		{"anonymous", Input{}, [4]bool{true, false, false, false}},
		{"account", Input{User: &store.User{}}, [4]bool{true, true, false, false}},
		{"verified", Input{User: &store.User{Verified: true}}, [4]bool{true, true, true, false}},
		{"admin", Input{User: &store.User{Admin: true}}, [4]bool{true, true, true, true}},
		{"invited account", Input{User: &store.User{}, Perm: store.Permission{Invited: true}}, [4]bool{true, true, true, true}},
		{"invited anonymous", Input{Perm: store.Permission{Invited: true}}, [4]bool{true, true, true, true}},
		{"trusted account", Input{User: &store.User{}, Perm: store.Permission{Trusted: true}}, [4]bool{true, true, false, true}},
		{"grant anonymous", Input{Grant: &Grant{}}, [4]bool{true, true, true, true}},
		{"grant account", Input{User: &store.User{}, Grant: &Grant{}}, [4]bool{true, true, true, true}},
	} {
		for j, mode := range []string{"public", "account", "verified", "invite"} {
			t.Run(identity.name+"/"+mode, func(t *testing.T) {
				in := identity.in
				in.Room.Access = mode
				got := Admit(in)
				if identity.allowed[j] {
					if got != nil {
						t.Fatalf("allowed: %+v", got)
					}
				} else if got == nil || got.Reason != mode || got.BannedUntil != nil {
					t.Fatalf("denial: %+v, want %s", got, mode)
				}
			})
		}
	}
}

func TestAdmitBans(t *testing.T) {
	now, past, future := int64(1000), int64(999), int64(1001)
	for _, tt := range []struct {
		name string
		in   Input
		want *Denial
	}{
		{"account timed", Input{User: &store.User{}, Perm: store.Permission{Banned: true, BannedUntil: &future}}, &Denial{Reason: "banned", BannedUntil: &future}},
		{"account expired", Input{User: &store.User{}, Perm: store.Permission{Banned: true, BannedUntil: &past}}, nil},
		{"account boundary", Input{User: &store.User{}, Perm: store.Permission{Banned: true, BannedUntil: &now}}, nil},
		{"account forever", Input{User: &store.User{}, Perm: store.Permission{Banned: true}}, &Denial{Reason: "banned"}},
		{"account unbanned", Input{User: &store.User{}, Perm: store.Permission{BannedUntil: &future}}, nil},
		{"admin timed", Input{User: &store.User{Admin: true}, Perm: store.Permission{Banned: true, BannedUntil: &future}}, &Denial{Reason: "banned", BannedUntil: &future}},
		{"admin forever", Input{User: &store.User{Admin: true}, Perm: store.Permission{Banned: true}}, &Denial{Reason: "banned"}},
		{"admin expired", Input{User: &store.User{Admin: true}, Perm: store.Permission{Banned: true, BannedUntil: &past}}, nil},
		{"anonymous timed", Input{AnonBan: &store.AnonBan{BannedUntil: &future}}, &Denial{Reason: "banned", BannedUntil: &future}},
		{"anonymous forever", Input{AnonBan: &store.AnonBan{}}, &Denial{Reason: "banned"}},
		{"anonymous active input", Input{AnonBan: &store.AnonBan{BannedUntil: &past}}, &Denial{Reason: "banned", BannedUntil: &past}},
		{"anonymous ignores account ban", Input{Perm: store.Permission{Banned: true}}, nil},
		{"account ignores anonymous ban", Input{User: &store.User{}, AnonBan: &store.AnonBan{}}, nil},
		{"invited banned", Input{User: &store.User{}, Perm: store.Permission{Invited: true, Banned: true}}, &Denial{Reason: "banned"}},
		{"trusted banned", Input{User: &store.User{}, Perm: store.Permission{Trusted: true, Banned: true}}, &Denial{Reason: "banned"}},
		{"grant banned", Input{Grant: &Grant{}, AnonBan: &store.AnonBan{}}, &Denial{Reason: "banned"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.in.Now, tt.in.Room.Access = now, "public"
			if got := Admit(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
	// Bans take precedence over each access denial.
	for _, mode := range []string{"account", "verified", "invite"} {
		t.Run("ban before "+mode, func(t *testing.T) {
			in := Input{Room: store.RoomSettings{Access: mode}, AnonBan: &store.AnonBan{}, Now: now}
			if got := Admit(in); got == nil || got.Reason != "banned" {
				t.Fatalf("ban precedence: %+v", got)
			}
		})
	}
	// An expired account ban still leaves the access requirement in force.
	in := Input{Room: store.RoomSettings{Access: "verified"}, User: &store.User{},
		Perm: store.Permission{Banned: true, BannedUntil: &past}, Now: now}
	if got := Admit(in); got == nil || got.Reason != "verified" {
		t.Fatalf("expired ban admission: %+v", got)
	}
}

func TestCompute(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   Input
		want Rights
	}{
		{"anonymous", Input{}, Rights{}},
		{"account", Input{User: &store.User{}}, Rights{}},
		{"verified", Input{User: &store.User{Verified: true}}, Rights{}},
		{"admin", Input{User: &store.User{Admin: true}}, Rights{Admin: true, Trusted: true, Remote: true, Image: true, Upload: true}},
		{"trusted account", Input{User: &store.User{}, Perm: store.Permission{Trusted: true}}, Rights{Trusted: true, Remote: true, Image: true, Upload: true}},
		{"trusted anonymous", Input{Perm: store.Permission{Trusted: true}}, Rights{Trusted: true, Remote: true, Upload: true}},
		{"invited", Input{User: &store.User{}, Perm: store.Permission{Invited: true}}, Rights{}},
		{"empty grant", Input{Grant: &Grant{}}, Rights{}},
		{"given to an anonymous person", Input{Given: Grant{Remote: true, Image: true, Upload: true}}, Rights{Remote: true, Upload: true}},
		{"given remote only", Input{Given: Grant{Remote: true}}, Rights{Remote: true}},
		{"banned admin", Input{User: &store.User{Admin: true}, Perm: store.Permission{Banned: true}}, Rights{Admin: true, Trusted: true, Remote: true, Image: true, Upload: true}},
		{"room behavior flags", Input{Room: store.RoomSettings{Hidden: true, RemoteOwnership: true}}, Rights{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := Compute(tt.in); got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestComputeGrants(t *testing.T) {
	// Each grant source independently gives exactly its selected rights.
	for _, source := range []string{"permission", "room", "grant"} {
		for _, account := range []bool{false, true} {
			for mask := 0; mask < 8; mask++ {
				in := Input{}
				if account {
					in.User = &store.User{}
				}
				remote, image, upload := mask&1 != 0, mask&2 != 0, mask&4 != 0
				switch source {
				case "permission":
					in.Perm = store.Permission{Remote: remote, Image: image, Upload: upload}
				case "room":
					in.Room = store.RoomSettings{DefaultRemote: remote, DefaultImage: image, DefaultUpload: upload}
				case "grant":
					in.Grant = &Grant{Remote: remote, Image: image, Upload: upload}
				}
				want := Rights{Remote: remote, Image: account && image, Upload: upload}
				if got := Compute(in); got != want {
					t.Errorf("source=%s account=%v mask=%d: got %+v, want %+v", source, account, mask, got, want)
				}
			}
		}
	}
	for _, account := range []bool{false, true} {
		in := Input{Perm: store.Permission{Remote: true}, Room: store.RoomSettings{DefaultImage: true}, Grant: &Grant{Upload: true}}
		if account {
			in.User = &store.User{}
		}
		if got := Compute(in); got != (Rights{Remote: true, Image: account, Upload: true}) {
			t.Errorf("combined sources account=%v: %+v", account, got)
		}
	}
}
