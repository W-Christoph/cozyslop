package neko

import (
	"encoding/json"
	"net/url"
)

// neko runs a capture pipeline (an encoder) for as long as someone watches
// it, and lets every session choose its own pipeline. CozyCast wants one
// encoder per room, so the proxy passes a browser's messages through
// PinStream before neko sees them.

type socketMessage struct {
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// The parts of neko's signal/request and signal/video payloads that are
// passed on. The rest (automatic quality, bitrate-based selection) is
// dropped: it would pick other pipelines.
type streamSelector struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type videoRequest struct {
	Disabled *bool           `json:"disabled,omitempty"`
	Selector *streamSelector `json:"selector,omitempty"`
}

type signalRequest struct {
	Video videoRequest `json:"video"`
	Audio struct {
		Disabled *bool `json:"disabled,omitempty"`
	} `json:"audio"`
}

func selector(stream string) *streamSelector {
	if stream == "" {
		return nil // neko uses its default
	}
	return &streamSelector{ID: stream, Type: "exact"}
}

func socketData(event string, payload any) []byte {
	msg := socketMessage{Event: event}
	if payload != nil {
		msg.Payload, _ = json.Marshal(payload)
	}
	data, _ := json.Marshal(msg)
	return data
}

// PinStream returns a browser's WebSocket message as it may be passed on to
// neko: whichever capture pipeline it asks for, it gets stream ("" = neko's
// default). ok is false for messages that are not passed on at all.
//
// Every message is rebuilt from what was understood of it, so neko never
// parses bytes this function read differently.
func PinStream(data []byte, stream string) (out []byte, ok bool) {
	var msg socketMessage
	if json.Unmarshal(data, &msg) != nil || msg.Event == "" {
		return nil, false
	}
	switch msg.Event {
	case "signal/request":
		var req signalRequest
		if len(msg.Payload) > 0 && json.Unmarshal(msg.Payload, &req) != nil {
			return nil, false
		}
		req.Video.Selector = selector(stream)
		return socketData(msg.Event, req), true
	case "signal/video":
		var req videoRequest
		if len(msg.Payload) > 0 && json.Unmarshal(msg.Payload, &req) != nil {
			return nil, false
		}
		req.Selector = selector(stream)
		if req.Selector == nil && req.Disabled == nil {
			return nil, false // nothing left to ask for
		}
		return socketData(msg.Event, req), true
	}
	out, _ = json.Marshal(msg)
	return out, true
}

// SelectStream is the message that moves a session to another capture
// pipeline.
func SelectStream(stream string) []byte {
	return socketData("signal/video", videoRequest{Selector: selector(stream)})
}

// SocketURL is the address of neko's WebSocket for the session token.
func (c *Client) SocketURL(token string) string {
	u := *c.base
	u.Scheme = map[string]string{"https": "wss"}[u.Scheme]
	if u.Scheme == "" {
		u.Scheme = "ws"
	}
	u.Path = u.JoinPath("/api/ws").Path
	u.RawQuery = url.Values{"token": {token}}.Encode()
	return u.String()
}
