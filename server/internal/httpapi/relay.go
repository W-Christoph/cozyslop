package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/pion/webrtc/v4"

	"cozycast/internal/hub"
	"cozycast/internal/relay"
)

// RelayMediaHost is where the relay reaches paired rooms' media: their
// forwarded ports, which with the relay on listen on this machine only.
const RelayMediaHost = "127.0.0.1"

// relayedMedia is one proxied neko connection's media, served by the relay
// instead of neko (docs/home-hosting.md, "The media relay").
type relayedMedia struct {
	relay *relay.Relay
	room  *hub.Room

	mu     sync.Mutex
	viewer *relay.Viewer
	closed bool
}

// signal takes a browser's message. Media signalling ("signal/...") is
// handled here and never reaches neko; reply, if any, goes to the browser.
func (m *relayedMedia) signal(ctx context.Context, data []byte) (reply []byte, handled bool, err error) {
	var msg struct {
		Event   string          `json:"event"`
		Payload json.RawMessage `json:"payload"`
	}
	if json.Unmarshal(data, &msg) != nil || !strings.HasPrefix(msg.Event, "signal/") {
		return nil, false, nil
	}
	switch msg.Event {
	case "signal/request":
		// Asking again replaces the connection, as with neko.
		var req struct {
			Video struct {
				Disabled bool `json:"disabled"`
			} `json:"video"`
		}
		_ = json.Unmarshal(msg.Payload, &req)
		m.set(nil)
		source := relay.Source{Neko: m.room.Neko(), Stream: m.room.Stream, MediaHost: RelayMediaHost}
		v, offer, err := m.relay.Join(ctx, m.room.Name, source, relay.Request{NoVideo: req.Video.Disabled})
		if err != nil {
			return nil, true, err
		}
		m.set(v)
		reply, _ = json.Marshal(map[string]any{"event": "signal/provide",
			"payload": map[string]any{"sdp": offer, "iceservers": []any{}}})
		return reply, true, nil
	case "signal/answer":
		var p struct {
			SDP string `json:"sdp"`
		}
		if v := m.get(); v != nil && json.Unmarshal(msg.Payload, &p) == nil {
			_ = v.Answer(p.SDP) // a late or repeated answer; the tab asks again
		}
	case "signal/candidate":
		// The relay has a public address and the tab connects to it, so its
		// candidates are not needed; unusable ones are ignored.
		var c webrtc.ICECandidateInit
		if v := m.get(); v != nil && json.Unmarshal(msg.Payload, &c) == nil {
			_ = v.Candidate(c)
		}
	case "signal/video":
		m.relay.Sync(m.room.Name)
	}
	return nil, true, nil
}

func (m *relayedMedia) get() *relay.Viewer {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.viewer
}

// set replaces the tab's connection to the relay, closing the old one.
func (m *relayedMedia) set(v *relay.Viewer) {
	m.mu.Lock()
	old := m.viewer
	m.viewer = v
	closed := m.closed
	m.mu.Unlock()
	if old != nil {
		old.Close()
	}
	if closed && v != nil {
		v.Close()
	}
}

func (m *relayedMedia) close() {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	m.set(nil)
}
