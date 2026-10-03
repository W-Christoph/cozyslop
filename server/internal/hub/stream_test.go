package hub

import (
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

	// Back to the container default: the desktop is left as it is.
	set.Screen = ""
	requireOK(t, f.s.SaveRoomSettings(f.ctx, set))
	f.h.RoomSettingsChanged(f.ctx, f.r.Name)
	requireEqual(t, f.fake.Screen(), neko.ScreenSize{Width: 800, Height: 600, Rate: 30})
}

func TestNekoRestartRestoresSettings(t *testing.T) {
	f := newFixture(t, store.RoomSettings{Screen: "1280x720@30"})
	requireEqual(t, f.fake.ImplicitHosting(), true)

	// A container restart brings neko back with its defaults (no screen
	// set, implicit hosting off); the room puts its settings back once the
	// event stream reconnects.
	f.fake.Restart()
	requireEqual(t, f.fake.ImplicitHosting(), false)
	deadline := time.Now().Add(testTimeout)
	for f.fake.Screen() != (neko.ScreenSize{Width: 1280, Height: 720, Rate: 30}) || !f.fake.ImplicitHosting() {
		if time.Now().After(deadline) {
			t.Fatalf("settings not restored: screen %v implicit %v", f.fake.Screen(), f.fake.ImplicitHosting())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestStreamsAndFallback(t *testing.T) {
	f := newFixture(t, store.RoomSettings{Stream: "b9999-s100"})
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
	set.Stream = "b1000-s50"
	requireOK(t, f.s.SaveRoomSettings(f.ctx, set))
	f.h.RoomSettingsChanged(f.ctx, f.r.Name)
	requireEqual(t, rec.wait(t, "room_settings").(settingsMsg).Settings.Stream, "b1000-s50")
}
