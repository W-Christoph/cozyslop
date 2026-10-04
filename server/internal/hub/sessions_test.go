package hub

import (
	"testing"

	"cozycast/internal/auth"
	"cozycast/internal/store"
)

func TestEndSessionsAcrossRooms(t *testing.T) {
	for _, mode := range []string{"one", "others", "all", "no session"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t, store.RoomSettings{DefaultRemote: true}, store.RoomSettings{Name: "other", Access: "public", DefaultRemote: true})
			first := f.user("alice")
			first.SessionHash = []byte("first session")
			second := first
			second.SessionHash = []byte("second session")
			other := f.user("bob")
			other.SessionHash = []byte("other account")
			anonymous := anon("anon")
			anonymous.SessionHash = first.SessionHash
			type tab struct {
				room    *Room
				client  *Client
				rec     *recording
				session int
			}
			var tabs []tab
			for _, r := range f.h.Rooms() {
				for session, id := range []auth.Identity{first, first, second, second, other, anonymous} {
					rec := newRecording()
					c, err := r.Join(f.ctx, JoinRequest{Identity: id, Send: rec.send, Kill: rec.kill})
					requireOK(t, err)
					t.Cleanup(func() { r.Leave(f.ctx, c) })
					tabs = append(tabs, tab{r, c, rec, session / 2})
				}
			}
			switch mode {
			case "one":
				f.h.EndSessions(0, first.SessionHash)
			case "others":
				f.h.EndSessions(first.User.ID, first.SessionHash)
			case "all":
				f.h.EndSessions(first.User.ID, nil)
			case "no session":
				f.h.EndSessions(0, nil)
			}
			for _, tab := range tabs {
				ended := tab.session < 2 && (mode == "all" || (mode == "one" && tab.session == 0) || (mode == "others" && tab.session == 1))
				tab.room.mu.Lock()
				_, present := tab.room.clients[tab.client.ID]
				remote := tab.client.m.rights.Remote
				tab.room.mu.Unlock()
				requireEqual(t, present, !ended)
				if ended {
					requireEqual(t, tab.rec.wait(t, "kicked").(kickedMsg).Reason, "session")
					tab.rec.assertKills(t, 1)
				} else {
					requireEqual(t, remote, true)
					requireEqual(t, tab.rec.count("kicked"), 0)
					tab.rec.assertKills(t, 0)
					tab.room.Handle(f.ctx, tab.client, ClientMsg{Type: "chat_send", Body: "still joined"})
					tab.rec.wait(t, "chat")
				}
			}
			for _, r := range f.h.Rooms() {
				want := 3
				if mode == "all" {
					want = 2
				}
				requireEqual(t, r.UserCount(), want)
			}
		})
	}
}
