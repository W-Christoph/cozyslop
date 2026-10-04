package hub

import (
	"reflect"
	"testing"
	"time"

	"cozycast/internal/auth"
	"cozycast/internal/store"
)

func TestAnonymousBanReachesTabsOnTheIP(t *testing.T) {
	for _, action := range []string{"forever", "timed", "kick"} {
		t.Run(action, func(t *testing.T) {
			f := newFixture(t, store.RoomSettings{}, store.RoomSettings{Name: "other", Access: "public"})
			first := auth.Identity{AnonID: "first", IP: "192.0.2.1"}
			second := auth.Identity{AnonID: "second", IP: first.IP}
			account := f.user("alice")
			account.IP = first.IP
			unrelated := auth.Identity{AnonID: "unrelated", IP: "203.0.113.9"}
			// The target's latest tab supplies the stored ban IP. A later
			// tab of the second identity must not hide its tab on that IP.
			old := first
			old.IP = unrelated.IP
			secondElsewhere := second
			secondElsewhere.IP = unrelated.IP
			type tab struct {
				client *Client
				rec    *recording
				key    string
			}
			var tabs []tab
			for _, id := range []auth.Identity{old, first, second, secondElsewhere, account, unrelated} {
				c, rec := f.join(id)
				f.r.SendNekoToken(f.ctx, c)
				rec.wait(t, "neko")
				tabs = append(tabs, tab{c, rec, id.Key()})
			}
			otherRoom := f.h.Room("other")
			otherRec := newRecording()
			otherClient, err := otherRoom.Join(f.ctx, JoinRequest{Identity: second, Send: otherRec.send, Kill: otherRec.kill})
			requireOK(t, err)
			t.Cleanup(func() { otherRoom.Leave(f.ctx, otherClient) })
			var until *int64
			if action == "timed" {
				n := time.Now().Unix() + 3600
				until = &n
			}
			if action == "kick" {
				requireOK(t, f.r.Kick(first.Key()))
			} else {
				requireOK(t, f.r.Ban(f.ctx, first.Key(), until))
			}
			for _, tab := range tabs {
				removed := tab.key == first.Key() || (action != "kick" && tab.key == second.Key())
				if removed {
					msg := tab.rec.wait(t, "kicked").(kickedMsg)
					wantReason := "banned"
					if action == "kick" {
						wantReason = "kicked"
					}
					requireEqual(t, msg.Reason, wantReason)
					if !reflect.DeepEqual(msg.BannedUntil, until) {
						t.Fatal("ban expiry lost")
					}
					tab.rec.assertKills(t, 1)
					f.r.Leave(f.ctx, tab.client)
					if _, present := f.fake.Member(tab.client.ID); present {
						t.Fatal("removed tab kept its neko member")
					}
				} else {
					requireEqual(t, tab.rec.count("kicked"), 0)
					tab.rec.assertKills(t, 0)
					if _, present := f.fake.Member(tab.client.ID); !present {
						t.Fatal("unrelated tab lost its neko member")
					}
				}
			}
			wantCount := 2
			if action == "kick" {
				wantCount = 3
			}
			requireEqual(t, f.r.UserCount(), wantCount)
			requireEqual(t, otherRoom.UserCount(), 1)
			otherRec.assertKills(t, 0)
			bans, err := f.s.ListAnonBans(f.ctx, f.r.Name)
			requireOK(t, err)
			if action == "kick" {
				requireEqual(t, len(bans), 0)
				f.join(first)
			} else {
				requireEqual(t, len(bans), 1)
				requireEqual(t, bans[0].AnonID, first.AnonID)
				requireEqual(t, bans[0].IP, first.IP)
				assertDenied(t, f.r, f.ctx, first, "", "banned", until)
				assertDenied(t, f.r, f.ctx, second, "", "banned", until)
			}
		})
	}
}
