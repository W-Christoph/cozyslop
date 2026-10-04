package hub

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"cozycast/internal/ratelimit"
	"cozycast/internal/rights"
	"cozycast/internal/store"
)

func expectError(t *testing.T, rec *recording, want string) {
	t.Helper()
	requireEqual(t, rec.wait(t, "error").(errorMsg).Message, want)
}

const notAllowed = "You are not allowed to do that."

func TestChatSendEditDelete(t *testing.T) {
	f := newFixture(t, store.RoomSettings{})
	alice := f.user("alice")
	bob := f.user("bob")
	admin := f.flags(f.user("admin"), true, false)
	c, a := f.join(alice)
	tab, at := f.join(alice)
	bc, b := f.join(bob)
	ac, mod := f.join(admin)
	f.r.Handle(f.ctx, c, ClientMsg{Type: "chat_send", Body: "\r\nHello 世界\n"})
	var id int64
	for _, rec := range []*recording{a, at, b, mod} {
		m := rec.wait(t, "chat").(chatMsg).Message
		requireEqual(t, m.Body, "Hello 世界")
		requireEqual(t, m.Author, alice.Key())
		requireEqual(t, m.Nickname, alice.User.Nickname)
		requireEqual(t, m.NameColor, alice.User.NameColor)
		requireEqual(t, m.Type, "text")
		requireEqual(t, m.Anonymous, false)
		if m.ID <= 0 || m.Time <= 0 {
			t.Fatal("missing chat id/time")
		}
		if id == 0 {
			id = m.ID
		}
		requireEqual(t, m.ID, id)
	}
	history, err := f.s.ChatHistory(f.ctx, f.r.Name)
	requireOK(t, err)
	requireEqual(t, len(history), 1)
	requireEqual(t, *history[0].UserID, alice.User.ID)
	for _, pair := range []struct {
		c   *Client
		rec *recording
	}{{bc, b}, {ac, mod}} {
		f.r.Handle(f.ctx, pair.c, ClientMsg{Type: "chat_edit", ID: id, Body: "stolen"})
		expectError(t, pair.rec, notAllowed)
	}
	// Authorship follows identity, so another tab can edit the author's text.
	f.r.Handle(f.ctx, tab, ClientMsg{Type: "chat_edit", ID: id, Body: "Edited"})
	for _, rec := range []*recording{a, at, b, mod} {
		m := rec.wait(t, "chat_edited").(chatEditedMsg)
		requireEqual(t, m.ID, id)
		requireEqual(t, m.Body, "Edited")
	}
	history, err = f.s.ChatHistory(f.ctx, f.r.Name)
	requireOK(t, err)
	requireEqual(t, history[0].Edited, true)
	requireEqual(t, history[0].Body, "Edited")
	f.r.Handle(f.ctx, bc, ClientMsg{Type: "chat_delete", ID: id})
	expectError(t, b, notAllowed)
	f.r.Handle(f.ctx, ac, ClientMsg{Type: "chat_delete", ID: id})
	for _, rec := range []*recording{a, at, b, mod} {
		requireEqual(t, rec.wait(t, "chat_deleted").(chatDeletedMsg).ID, id)
	}
	history, err = f.s.ChatHistory(f.ctx, f.r.Name)
	requireOK(t, err)
	requireEqual(t, len(history), 0)
	f.r.Handle(f.ctx, c, ClientMsg{Type: "chat_send", Body: "own delete"})
	own := a.wait(t, "chat").(chatMsg).Message.ID
	f.r.Handle(f.ctx, tab, ClientMsg{Type: "chat_delete", ID: own})
	requireEqual(t, at.wait(t, "chat_deleted").(chatDeletedMsg).ID, own)
	for _, typ := range []string{"chat_edit", "chat_delete"} {
		f.r.Handle(f.ctx, c, ClientMsg{Type: typ, ID: 99999, Body: "missing"})
		expectError(t, a, notAllowed)
	}
	f.r.Handle(f.ctx, c, ClientMsg{Type: "unknown"})
	expectError(t, a, "Unknown message type.")
}

func TestChatLimitsAndRateLimit(t *testing.T) {
	for _, anonymous := range []bool{false, true} {
		t.Run(fmt.Sprint(anonymous), func(t *testing.T) {
			f := newFixture(t, store.RoomSettings{})
			id := anon("guest")
			limit, burst := 250, 5
			if !anonymous {
				id = f.user("alice")
				limit, burst = 4096, 10
			}
			// Keep the production burst sizes, with a refill interval longer than
			// the test deadline, so scheduler delays cannot replenish the bucket.
			f.r.chatUser = ratelimit.New(10, time.Hour)
			f.r.chatAnon = ratelimit.New(5, time.Hour)
			c, a := f.join(id)
			tab, b := f.join(id)
			_, watch := f.join(anon("watcher"))
			for _, body := range []string{"", " \t\r\n", strings.Repeat("界", limit+1)} {
				f.r.Handle(f.ctx, c, ClientMsg{Type: "chat_send", Body: body})
				want := "Message is empty."
				if len(body) > 10 {
					want = "Message is too long."
				}
				expectError(t, a, want)
			}
			requireEqual(t, watch.count("chat"), 0)
			f.r.Handle(f.ctx, c, ClientMsg{Type: "chat_send", Body: strings.Repeat("界", limit)})
			msg := a.wait(t, "chat").(chatMsg).Message
			requireEqual(t, len([]rune(msg.Body)), limit)
			f.r.Handle(f.ctx, tab, ClientMsg{Type: "chat_edit", ID: msg.ID, Body: strings.Repeat("界", limit+1)})
			expectError(t, b, "Message is too long.")
			// Drain the remainder of the burst across tabs: the bucket is per person.
			for i := 1; i < burst; i++ {
				sender := c
				if i%2 == 1 {
					sender = tab
				}
				f.r.Handle(f.ctx, sender, ClientMsg{Type: "chat_send", Body: "burst"})
			}
			f.r.Handle(f.ctx, tab, ClientMsg{Type: "chat_send", Body: "over limit"})
			expectError(t, b, "You are sending messages too fast.")
			requireEqual(t, watch.count("chat"), burst)
			other, otherRec := f.join(anon("independent"))
			f.r.Handle(f.ctx, other, ClientMsg{Type: "chat_send", Body: "separate bucket"})
			otherRec.wait(t, "chat")
		})
	}
}

func TestWhisper(t *testing.T) {
	f := newFixture(t, store.RoomSettings{})
	admin := f.flags(f.user("admin"), true, false)
	target := f.user("target")
	c, a := f.join(admin)
	_, at := f.join(admin)
	tc, b := f.join(target)
	_, bt := f.join(target)
	_, watch := f.join(anon("watcher"))
	f.r.Handle(f.ctx, tc, ClientMsg{Type: "whisper", To: admin.Key(), Body: "not admin"})
	expectError(t, b, notAllowed)
	f.r.Handle(f.ctx, c, ClientMsg{Type: "whisper", To: target.Key(), Body: "private"})
	for _, rec := range []*recording{a, at, b, bt} {
		m := rec.wait(t, "chat").(chatMsg).Message
		requireEqual(t, m.ID, int64(-1))
		requireEqual(t, m.Author, admin.Key())
		requireEqual(t, m.Type, "whisper")
		requireEqual(t, m.Body, "private")
		requireEqual(t, m.Nickname, "Mod Whisper")
	}
	requireEqual(t, watch.count("chat"), 0)
	f.r.Handle(f.ctx, c, ClientMsg{Type: "whisper", To: admin.Key(), Body: "self"})
	for _, rec := range []*recording{a, at} {
		requireEqual(t, rec.wait(t, "chat").(chatMsg).Message.ID, int64(-2))
		requireEqual(t, rec.count("chat"), 2)
	}
	requireEqual(t, b.count("chat"), 1)
	requireEqual(t, bt.count("chat"), 1)
	history, err := f.s.ChatHistory(f.ctx, f.r.Name)
	requireOK(t, err)
	requireEqual(t, len(history), 0)
	f.r.Handle(f.ctx, c, ClientMsg{Type: "whisper", To: "missing", Body: "hello"})
	expectError(t, a, "That user is not in the room anymore.")
	f.r.Handle(f.ctx, c, ClientMsg{Type: "whisper", To: target.Key(), Body: " "})
	expectError(t, a, "Message is empty.")
	f.r.Handle(f.ctx, c, ClientMsg{Type: "whisper", To: target.Key(), Body: strings.Repeat("x", 4097)})
	expectError(t, a, "Message is too long.")
}

func TestTypingAndPresence(t *testing.T) {
	f := newFixture(t, store.RoomSettings{})
	id := f.user("alice")
	c, a := f.join(id)
	tab, b := f.join(id)
	_, watch := f.join(anon("watcher"))
	w := a.wait(t, "welcome").(welcomeMsg)
	f.r.Handle(f.ctx, c, ClientMsg{Type: "typing", Typing: true})
	m := watch.wait(t, "typing").(typingMsg)
	requireEqual(t, m.Key, id.Key())
	requireEqual(t, m.Typing, true)
	f.r.Handle(f.ctx, c, ClientMsg{Type: "typing", Typing: true})
	requireEqual(t, watch.count("typing"), 1)
	f.r.Handle(f.ctx, c, ClientMsg{Type: "typing", Typing: false})
	requireEqual(t, watch.wait(t, "typing").(typingMsg).Typing, false)
	requireEqual(t, a.count("typing"), 0)
	requireEqual(t, b.count("typing"), 0)
	before := watch.count("user_updated")
	f.r.Handle(f.ctx, c, ClientMsg{Type: "activity", Active: false})
	requireEqual(t, watch.count("user_updated"), before)
	f.r.Handle(f.ctx, tab, ClientMsg{Type: "activity", Active: false})
	u := watch.wait(t, "user_updated").(userMsg).User
	requireEqual(t, u.Active, false)
	requireEqual(t, u.Muted, false)
	if u.LastSeen < w.Self.LastSeen {
		t.Fatal("lastSeen went backwards")
	}
	f.r.Handle(f.ctx, c, ClientMsg{Type: "activity", Active: true})
	requireEqual(t, watch.wait(t, "user_updated").(userMsg).User.Active, true)
	before = watch.count("user_updated")
	f.r.Handle(f.ctx, c, ClientMsg{Type: "muted", Muted: true})
	requireEqual(t, watch.count("user_updated"), before)
	f.r.Handle(f.ctx, tab, ClientMsg{Type: "muted", Muted: true})
	requireEqual(t, watch.wait(t, "user_updated").(userMsg).User.Muted, true)
	f.r.Handle(f.ctx, c, ClientMsg{Type: "muted", Muted: false})
	requireEqual(t, watch.wait(t, "user_updated").(userMsg).User.Muted, false)
	// Leaving the sole active/unmuted tab changes the remaining person's state.
	f.r.Leave(f.ctx, c)
	u = watch.wait(t, "user_updated").(userMsg).User
	requireEqual(t, u.Active, false)
	requireEqual(t, u.Muted, true)
	requireEqual(t, u.Key, id.Key())
	requireEqual(t, watch.count("user_left"), 0)
}

func TestPostMediaAndRemoval(t *testing.T) {
	f := newFixture(t, store.RoomSettings{DefaultImage: true})
	alice := f.user("alice")
	bob := f.user("bob")
	c, a := f.join(alice)
	bc, b := f.join(bob)
	guest, g := f.join(anon("guest"))
	_ = guest
	requireEqual(t, errors.Is(f.r.CanPostMedia("missing"), ErrNotPresent), true)
	requireEqual(t, errors.Is(f.r.PostMedia(f.ctx, "missing", "image", "none"), ErrNotPresent), true)
	requireEqual(t, errors.Is(f.r.CanPostMedia("a:guest"), ErrNotAllowed), true)
	requireEqual(t, errors.Is(f.r.PostMedia(f.ctx, "a:guest", "image", "none"), ErrNotAllowed), true)
	for _, typ := range []string{"image", "video"} {
		requireOK(t, f.r.CanPostMedia(alice.Key()))
		file := typ + ".png"
		path := filepath.Join(f.h.mediaDir, file)
		requireOK(t, os.WriteFile(path, []byte("media"), 0600))
		requireOK(t, f.r.PostMedia(f.ctx, alice.Key(), typ, file))
		m := a.wait(t, "chat").(chatMsg).Message
		requireEqual(t, m.Type, typ)
		requireEqual(t, m.MediaURL, "/media/chat/"+file)
		requireEqual(t, m.Author, alice.Key())
		requireEqual(t, b.wait(t, "chat").(chatMsg).Message.ID, m.ID)
		requireEqual(t, g.wait(t, "chat").(chatMsg).Message.ID, m.ID)
		f.r.Handle(f.ctx, c, ClientMsg{Type: "chat_edit", ID: m.ID, Body: "cannot edit media"})
		expectError(t, a, notAllowed)
		f.r.Handle(f.ctx, bc, ClientMsg{Type: "chat_delete", ID: m.ID})
		expectError(t, b, notAllowed)
		if _, err := os.Stat(path); err != nil {
			t.Fatal("denied delete removed file", err)
		}
		f.r.Handle(f.ctx, c, ClientMsg{Type: "chat_delete", ID: m.ID})
		requireEqual(t, a.wait(t, "chat_deleted").(chatDeletedMsg).ID, m.ID)
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("media file retained", err)
		}
	}
	// Upload endpoints check twice; revoking rights between checks must reject.
	set := f.r.Settings()
	set.DefaultImage = false
	requireOK(t, f.s.SaveRoomSettings(f.ctx, set))
	f.h.RoomSettingsChanged(f.ctx, f.r.Name)
	requireEqual(t, errors.Is(f.r.CanPostMedia(alice.Key()), ErrNotAllowed), true)
	requireEqual(t, errors.Is(f.r.PostMedia(f.ctx, alice.Key(), "image", "later.png"), ErrNotAllowed), true)
	requireEqual(t, a.wait(t, "rights").(rightsMsg).Rights, rights.Rights{})
}

func TestMediaSharesChatRateLimit(t *testing.T) {
	f := newFixture(t, store.RoomSettings{DefaultImage: true})
	id := f.user("alice")
	f.r.chatUser = ratelimit.New(2, time.Hour)
	c, rec := f.join(id)
	requireOK(t, f.r.CanPostMedia(id.Key()))
	requireOK(t, f.r.PostMedia(f.ctx, id.Key(), "image", "one.png"))
	f.r.Handle(f.ctx, c, ClientMsg{Type: "chat_send", Body: "second token"})
	requireEqual(t, rec.count("chat"), 2)
	requireEqual(t, errors.Is(f.r.CanPostMedia(id.Key()), ErrRateLimited), true)
	f.r.Handle(f.ctx, c, ClientMsg{Type: "chat_send", Body: "over limit"})
	expectError(t, rec, "You are sending messages too fast.")
}

func TestConcurrentChatAndJoinOrdering(t *testing.T) {
	f := newFixture(t, store.RoomSettings{})
	const senders, perSender, joiners = 8, 8, 16
	clients := make([]*Client, senders)
	recordings := make([]*recording, senders)
	for i := range clients {
		clients[i], recordings[i] = f.join(f.user(fmt.Sprintf("sender%d", i)))
	}
	// Seed known history, then race simultaneous senders and joiners behind a
	// shared barrier. Each account stays below the real limiter's burst.
	f.r.Handle(f.ctx, clients[0], ClientMsg{Type: "chat_send", Body: "before race"})
	start := make(chan struct{})
	var wg sync.WaitGroup
	joined := make([]*recording, joiners)
	joinErrs := make([]error, joiners)
	joinedClients := make([]*Client, joiners)
	for i, c := range clients {
		wg.Go(func() {
			<-start
			for j := 0; j < perSender; j++ {
				f.r.Handle(f.ctx, c, ClientMsg{Type: "chat_send", Body: fmt.Sprintf("%d:%d", i, j)})
			}
		})
	}
	for i := range joiners {
		wg.Go(func() {
			<-start
			rec := newRecording()
			joined[i] = rec
			joinedClients[i], joinErrs[i] = f.r.Join(f.ctx, JoinRequest{Identity: anon(fmt.Sprintf("joiner-%d", i)), Send: rec.send, Kill: rec.kill})
		})
	}
	close(start)
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(testTimeout):
		t.Fatal("concurrent sends/joins stalled")
	}
	for i, err := range joinErrs {
		requireOK(t, err)
		c := joinedClients[i]
		t.Cleanup(func() { f.r.Leave(f.ctx, c) })
	}
	history, err := f.s.ChatHistory(f.ctx, f.r.Name)
	requireOK(t, err)
	requireEqual(t, len(history), senders*perSender+1)
	ids := make([]int64, len(history))
	for i, m := range history {
		ids[i] = m.ID
	}
	for i, rec := range joined {
		w := rec.wait(t, "welcome").(welcomeMsg)
		seen := make([]int64, 0, len(ids))
		for _, m := range w.History {
			seen = append(seen, m.ID)
		}
		for _, raw := range rec.snapshot() {
			if m, ok := raw.(chatMsg); ok {
				seen = append(seen, m.Message.ID)
			}
		}
		if !reflect.DeepEqual(seen, ids) {
			t.Fatalf("joiner %d history+chat ids=%v, want each once in order %v", i, seen, ids)
		}
	}
	for i, rec := range recordings {
		seen := []int64{}
		for _, raw := range rec.snapshot() {
			if m, ok := raw.(chatMsg); ok {
				seen = append(seen, m.Message.ID)
			}
		}
		if !reflect.DeepEqual(seen, ids) {
			t.Fatalf("sender %d received ids=%v, want %v", i, seen, ids)
		}
		requireEqual(t, rec.count("error"), 0)
	}
}

func TestInboundRateLimitCoversEveryMessageType(t *testing.T) {
	f := newFixture(t, store.RoomSettings{})
	// A refill interval longer than the test, as in TestChatLimitsAndRateLimit.
	f.r.inbound = ratelimit.New(6, time.Hour)
	c, a := f.join(anon("flooder"))
	tab, b := f.join(anon("flooder"))
	_, watch := f.join(anon("watcher"))
	for i := 0; i < 6; i++ {
		f.r.Handle(f.ctx, c, ClientMsg{Type: "typing", Typing: false})
	}
	requireEqual(t, watch.count("typing"), 6)
	// The bucket is per person, and no message type is exempt.
	for _, msg := range []ClientMsg{
		{Type: "typing"}, {Type: "activity"}, {Type: "muted", Muted: true}, {Type: "chat_edit", ID: 1, Body: "x"},
		{Type: "chat_send", Body: "x"}, {Type: "unknown"},
	} {
		f.r.Handle(f.ctx, tab, msg)
		expectError(t, b, "You are sending messages too fast.")
	}
	requireEqual(t, watch.count("typing")+watch.count("user_updated")+watch.count("chat"), 6)
	requireEqual(t, a.count("error"), 0)
	other, rec := f.join(anon("independent"))
	f.r.Handle(f.ctx, other, ClientMsg{Type: "chat_send", Body: "separate bucket"})
	rec.wait(t, "chat")
}

func TestPostChecksMediaRights(t *testing.T) {
	f := newFixture(t, store.RoomSettings{DefaultImage: true})
	id := f.user("alice")
	_, rec := f.join(id)
	requireOK(t, f.r.CanPostMedia(id.Key()))
	set := f.r.Settings()
	set.DefaultImage = false
	requireOK(t, f.s.SaveRoomSettings(f.ctx, set))
	f.h.RoomSettingsChanged(f.ctx, f.r.Name)
	for _, typ := range []string{"image", "video"} {
		// Exercise the final commit point directly: every media post is
		// checked under the same lock that inserts and broadcasts it.
		requireEqual(t, errors.Is(f.r.post(f.ctx, id.Key(), typ, "", "refused"), ErrNotAllowed), true)
	}
	requireEqual(t, rec.count("chat"), 0)
	history, err := f.s.ChatHistory(f.ctx, f.r.Name)
	requireOK(t, err)
	requireEqual(t, len(history), 0)
	requireOK(t, f.r.post(f.ctx, id.Key(), "text", "still allowed", ""))
}
