package hub

import (
	"testing"

	"cozycast/internal/neko"
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
