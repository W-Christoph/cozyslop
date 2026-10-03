package httpapi

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"os"
	"path/filepath"
	"testing"
)

func testGIF(t *testing.T, frames, w, h int, localPalette bool) []byte {
	t.Helper()
	g := &gif.GIF{}
	global := color.Palette{color.Black, color.White}
	for i := 0; i < frames; i++ {
		palette := global
		if localPalette && i%2 == 1 {
			palette = color.Palette{color.Black, color.White, color.RGBA{R: 255, A: 255}, color.Transparent}
		}
		frame := image.NewPaletted(image.Rect(0, 0, w, h), palette)
		frame.SetColorIndex(i%w, i%h, 1)
		g.Image = append(g.Image, frame)
		g.Delay = append(g.Delay, 1)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestGIFFramePixels(t *testing.T) {
	for _, tc := range []struct {
		frames, w, h int
		local        bool
	}{{1, 3, 2, false}, {7, 60, 20, false}, {5, 300, 200, true}} {
		data := testGIF(t, tc.frames, tc.w, tc.h, tc.local)
		want := int64(tc.frames * tc.w * tc.h)
		got, err := gifFramePixels(bytes.NewReader(data), 1<<40)
		if err != nil || got != want {
			t.Fatalf("%+v: pixels=%d err=%v, want %d", tc, got, err, want)
		}
		// Stops counting once over the limit.
		if got, err := gifFramePixels(bytes.NewReader(data), want-1); err != nil || got <= want-1 {
			t.Fatalf("%+v: limited pixels=%d err=%v", tc, got, err)
		}
		// A file cut off anywhere is an error, never a low count.
		for _, cut := range []int{5, 13, len(data) / 2, len(data) - 1} {
			if got, err := gifFramePixels(bytes.NewReader(data[:cut]), 1<<40); err == nil {
				t.Fatalf("%+v cut at %d: pixels=%d, want an error", tc, cut, got)
			}
		}
	}
}

func TestDecodeUploadLimitsGIFFrames(t *testing.T) {
	old := maxGIFPixels
	maxGIFPixels = 10 * 60 * 20
	t.Cleanup(func() { maxGIFPixels = old })
	decode := func(frames int, all bool) error {
		t.Helper()
		name := filepath.Join(t.TempDir(), "x.gif")
		if err := os.WriteFile(name, testGIF(t, frames, 60, 20, false), 0o600); err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		_, _, err = decodeUpload(f, all)
		return err
	}
	if err := decode(10, true); err != nil {
		t.Fatal("GIF at the limit:", err)
	}
	if err := decode(11, true); !errors.Is(err, errUnsupportedMedia) {
		t.Fatal("GIF over the limit:", err)
	}
	// Avatars decode one frame, so the frame count does not matter.
	if err := decode(11, false); err != nil {
		t.Fatal("avatar GIF:", err)
	}
}
