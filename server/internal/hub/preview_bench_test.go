package hub

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// previewDesktop is a 1080p desktop-like frame: window chrome, text and a
// textured video panel. Building and encoding the source are not timed.
func previewDesktop(b *testing.B) []byte {
	b.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	fill := func(rect image.Rectangle, c color.RGBA) {
		draw.Draw(img, rect, image.NewUniform(c), image.Point{}, draw.Src)
	}
	fill(img.Bounds(), color.RGBA{28, 30, 36, 255})
	fill(image.Rect(0, 0, 1920, 40), color.RGBA{55, 59, 70, 255})
	fill(image.Rect(20, 60, 1260, 1000), color.RGBA{238, 240, 242, 255})
	fill(image.Rect(20, 60, 1260, 110), color.RGBA{85, 105, 135, 255})
	fill(image.Rect(20, 110, 220, 1000), color.RGBA{210, 217, 225, 255})
	d := font.Drawer{Dst: img, Src: image.NewUniform(color.RGBA{30, 35, 44, 255}), Face: basicfont.Face7x13}
	for y := 145; y < 970; y += 22 {
		d.Dot = fixed.P(245, y)
		d.DrawString("CozyCast desktop: a browser page with chat, documents and video playing.")
	}
	for y := 180; y < 750; y++ {
		for x := 1310; x < 1890; x++ {
			noise := (x*17 + y*31 + (x*y)%97) % 35
			img.SetRGBA(x, y, color.RGBA{uint8((x/4 + noise) % 256), uint8((y/3 + noise) % 256), uint8((x/6 + y/4 + noise) % 256), 255})
		}
	}
	d.Dot = fixed.P(1330, 800)
	d.Src = image.White
	d.DrawString("Now playing - Room desktop")
	var source bytes.Buffer
	if err := jpeg.Encode(&source, img, &jpeg.Options{Quality: previewSourceQuality}); err != nil {
		b.Fatal(err)
	}
	return source.Bytes()
}

func BenchmarkPreview1080p(b *testing.B) {
	source := previewDesktop(b)
	img, err := jpeg.Decode(bytes.NewReader(source))
	if err != nil {
		b.Fatal(err)
	}
	b.Run("CatmullRom", func(b *testing.B) {
		dst := image.NewRGBA(image.Rect(0, 0, previewWidth, previewHeight))
		b.ResetTimer()
		for b.Loop() {
			draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Src, nil)
		}
	})
	b.Run("JPEGToPreview", func(b *testing.B) {
		var result []byte
		for b.Loop() {
			result, err = shrinkPreview(source)
			if err != nil {
				b.Fatal(err)
			}
		}
		b.ReportMetric(float64(len(source)), "source-bytes")
		b.ReportMetric(float64(len(result)), "jpeg-bytes")
	})
	b.Run("ApproxBiLinearWithHalving", func(b *testing.B) {
		for b.Loop() {
			_ = scalePreview(img, previewWidth, previewHeight)
		}
	})
}
