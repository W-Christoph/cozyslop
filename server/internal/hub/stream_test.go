package hub

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"cozycast/internal/neko"
	"cozycast/internal/neko/nekotest"
	"cozycast/internal/store"
)

func TestRoomScreen(t *testing.T) {
	f := newFixture(t, store.RoomSettings{Screen: "1280x720@30"})
	// Applied before the room reports ready.
	requireEqual(t, f.fake.Screen(), neko.ScreenSize{Width: 1280, Height: 720, Rate: 30})

	set := f.r.Settings()
	set.Screen = "800x600@30"
	requireOK(t, f.s.SaveRoomSettings(f.ctx, set))
	f.h.RoomSettingsChanged(f.ctx, f.r.Name)
	requireEqual(t, f.fake.Screen(), neko.ScreenSize{Width: 800, Height: 600, Rate: 30})

	// Without a configured default, clearing leaves the desktop as it is.
	set.Screen = ""
	requireOK(t, f.s.SaveRoomSettings(f.ctx, set))
	f.h.RoomSettingsChanged(f.ctx, f.r.Name)
	requireEqual(t, f.fake.Screen(), neko.ScreenSize{Width: 800, Height: 600, Rate: 30})
}

func TestNekoRestartRestoresSettings(t *testing.T) {
	for _, ownership := range []bool{false, true} {
		t.Run(fmt.Sprint(ownership), func(t *testing.T) {
			f := newFixture(t, store.RoomSettings{Screen: "1280x720@30", RemoteOwnership: ownership})
			requireEqual(t, f.fake.ImplicitHosting(), !ownership)
			f.fake.Restart()
			requireEqual(t, f.fake.ImplicitHosting(), false)
			deadline := time.Now().Add(testTimeout)
			for {
				settingsCalls := 0
				for _, call := range f.fake.Calls() {
					if call.Path == "/api/room/settings" {
						settingsCalls++
					}
				}
				if settingsCalls >= 3 && f.fake.Screen() == (neko.ScreenSize{Width: 1280, Height: 720, Rate: 30}) && f.fake.ImplicitHosting() == !ownership {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("settings not restored: screen %v implicit %v calls %v", f.fake.Screen(), f.fake.ImplicitHosting(), f.fake.Calls())
				}
				time.Sleep(20 * time.Millisecond)
			}
		})
	}
}

func TestImplicitHostingExplicitlyApplied(t *testing.T) {
	ctx := context.Background()
	for _, ownership := range []bool{false, true} {
		t.Run(fmt.Sprint(ownership), func(t *testing.T) {
			fake := nekotest.New(t, "secret")
			nc, err := neko.NewClient(fake.URL(), "secret", nil)
			requireOK(t, err)
			r := newRoom(nil, "main", nc)
			r.settings.RemoteOwnership = ownership
			for _, prepare := range []bool{true, false} {
				requireOK(t, nc.SetImplicitHosting(ctx, ownership))
				if prepare {
					requireOK(t, r.prepareNeko(ctx))
				} else {
					r.reapplyNekoSettings(ctx)
				}
				requireEqual(t, fake.ImplicitHosting(), !ownership)
			}
		})
	}
}

func TestRoomScreenDefault(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, ":memory:")
	requireOK(t, err)
	t.Cleanup(func() { requireOK(t, s.Close()) })
	fake := nekotest.New(t, "secret")
	nc, err := neko.NewClient(fake.URL(), "secret", nil)
	requireOK(t, err)
	h := New(s, t.TempDir(), []RoomConfig{{Name: "main", Neko: nc, DefaultScreen: "1280x720@30"}})
	r := h.Room("main")
	defaultSize := neko.ScreenSize{Width: 1280, Height: 720, Rate: 30}
	requireOK(t, r.prepareNeko(ctx))
	requireEqual(t, fake.Screen(), defaultSize)
	set := r.Settings()
	set.Screen = "800x600@30"
	requireOK(t, s.SaveRoomSettings(ctx, set))
	h.RoomSettingsChanged(ctx, r.Name)
	requireEqual(t, fake.Screen(), neko.ScreenSize{Width: 800, Height: 600, Rate: 30})
	set.Screen = ""
	requireOK(t, s.SaveRoomSettings(ctx, set))
	h.RoomSettingsChanged(ctx, r.Name)
	requireEqual(t, fake.Screen(), defaultSize)
	before := len(fake.Calls())
	requireOK(t, r.applyScreen(ctx, ""))
	for _, call := range fake.Calls()[before:] {
		if call.Method == "POST" && call.Path == "/api/room/screen" {
			t.Fatal("unchanged default restarted the stream")
		}
	}
	fake.Restart()
	r.reapplyNekoSettings(ctx)
	requireEqual(t, fake.Screen(), defaultSize)
}

func TestStreamsAndFallback(t *testing.T) {
	f := newFixture(t, store.RoomSettings{Stream: "b9999-s100-veryfast"})
	deadline := time.Now().Add(testTimeout)
	for f.r.Streams() == nil {
		if time.Now().After(deadline) {
			t.Fatal("streams never reported")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !slices.Equal(f.r.Streams(), nekotest.Streams) {
		t.Fatalf("streams %v, want %v", f.r.Streams(), nekotest.Streams)
	}

	// A stored stream neko does not offer is not handed to browsers.
	_, rec := f.join(anon("guest"))
	requireEqual(t, rec.wait(t, "welcome").(welcomeMsg).Settings.Stream, "")

	set := f.r.Settings()
	set.Stream = "b1000-s50-ultrafast"
	requireOK(t, f.s.SaveRoomSettings(f.ctx, set))
	f.h.RoomSettingsChanged(f.ctx, f.r.Name)
	requireEqual(t, rec.wait(t, "room_settings").(settingsMsg).Settings.Stream, "b1000-s50-ultrafast")
}
