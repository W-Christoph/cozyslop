package hub

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"time"

	"golang.org/x/image/draw"
)

const (
	previewCacheAge      = 1500 * time.Millisecond // share recent shots across all tabs
	previewWidth         = 320
	previewHeight        = 180
	previewSourceQuality = 80       // neko's JPEG stays on the server
	previewQuality       = 60       // the small JPEG sent over the room socket
	maxPreviewPixels     = 32 << 20 // bound decode memory; allows an 8K desktop
)

type previewShot struct {
	msg   previewMsg
	taken time.Time
}

type previewRequest struct {
	done chan struct{}
	shot previewShot // read after done closes
}

func (r *Room) sendPreview(ctx context.Context, c *Client) {
	r.mu.Lock()
	if c.previewBusy || r.clients[c.ID] != c {
		r.mu.Unlock()
		return
	}
	c.previewBusy = true
	r.mu.Unlock()

	msg := r.previewAnswer(ctx, c)
	r.mu.Lock()
	defer r.mu.Unlock()
	c.previewBusy = false
	// As with neko tokens, only tabs still admitted to the room may watch.
	if ctx.Err() != nil || r.removed.Err() != nil || r.clients[c.ID] != c {
		return
	}
	if r.offline {
		msg = previewMsg{Type: "preview"}
	}
	c.send(msg)
}

func (r *Room) previewAnswer(ctx context.Context, c *Client) previewMsg {
	empty := previewMsg{Type: "preview"}
	r.mu.Lock()
	if ctx.Err() != nil || r.removed.Err() != nil || r.offline || r.clients[c.ID] != c {
		r.mu.Unlock()
		return empty
	}
	if shot := r.preview; shot != nil && time.Since(shot.taken) < previewCacheAge {
		r.mu.Unlock()
		return shot.msg
	}
	flight := r.previewFlight
	if flight == nil {
		flight = &previewRequest{done: make(chan struct{})}
		r.previewFlight = flight
		// One tab leaving must not cancel a shot other tabs are waiting for.
		// Room removal cancels it; the neko client's timeout bounds the call.
		go r.capturePreview(flight)
	}
	r.mu.Unlock()
	select {
	case <-flight.done:
		return flight.shot.msg
	case <-ctx.Done():
	case <-r.removed.Done():
	}
	return empty
}

func (r *Room) capturePreview(flight *previewRequest) {
	shot := previewShot{msg: previewMsg{Type: "preview"}, taken: time.Now()}
	data, err := r.neko.Screenshot(r.removed, previewSourceQuality)
	if err == nil {
		data, err = shrinkPreview(data)
	}
	if err != nil {
		if r.removed.Err() == nil {
			r.log.Debug("capture preview", "err", err)
		}
	} else {
		shot.msg.Image = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data)
		shot.msg.Time = shot.taken.UnixMilli()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err == nil && !r.offline && r.removed.Err() == nil {
		r.preview = &shot
	}
	flight.shot = shot
	r.previewFlight = nil
	close(flight.done)
}

// shrinkPreview fits a JPEG inside the preview bounds without upscaling.
func shrinkPreview(data []byte) ([]byte, error) {
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > maxPreviewPixels {
		return nil, fmt.Errorf("preview: invalid dimensions %dx%d", cfg.Width, cfg.Height)
	}
	src, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	w, h := cfg.Width, cfg.Height
	if w > previewWidth {
		h = max(1, h*previewWidth/w)
		w = previewWidth
	}
	if h > previewHeight {
		w = max(1, w*previewHeight/h)
		h = previewHeight
	}
	var img image.Image = src
	if w != cfg.Width || h != cfg.Height {
		img = scalePreview(src, w, h)
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: previewQuality}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// scalePreview halves with a cheap four-sample filter before the final scale.
// This avoids the cost of a wide CatmullRom filter on a 1 vCPU server.
func scalePreview(src image.Image, w, h int) image.Image {
	for src.Bounds().Dx() >= 2*w && src.Bounds().Dy() >= 2*h {
		dst := image.NewRGBA(image.Rect(0, 0, src.Bounds().Dx()/2, src.Bounds().Dy()/2))
		draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
		src = dst
	}
	if src.Bounds().Dx() == w && src.Bounds().Dy() == h {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return dst
}
