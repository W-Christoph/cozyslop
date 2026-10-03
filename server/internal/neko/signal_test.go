package neko

import "testing"

func TestPinStream(t *testing.T) {
	const want = `"selector":{"id":"room","type":"exact"}`
	for _, tc := range []struct{ name, in, stream, out string }{
		{"request without a selector", `{"event":"signal/request","payload":{"video":{},"audio":{}}}`, "room",
			`{"event":"signal/request","payload":{"video":{` + want + `},"audio":{}}}`},
		{"request for another pipeline", `{"event":"signal/request","payload":{"video":{"disabled":true,"selector":{"id":"other","type":"exact"},"auto":true},"audio":{"disabled":false},"auto":true}}`, "room",
			`{"event":"signal/request","payload":{"video":{"disabled":true,` + want + `},"audio":{"disabled":false}}}`},
		{"request by bitrate", `{"event":"signal/request","payload":{"video":{"selector":{"bitrate":9000000,"type":"nearest"}}}}`, "room",
			`{"event":"signal/request","payload":{"video":{` + want + `},"audio":{}}}`},
		{"request without payload", `{"event":"signal/request"}`, "room",
			`{"event":"signal/request","payload":{"video":{` + want + `},"audio":{}}}`},
		{"request, neko's default", `{"event":"signal/request","payload":{"video":{"selector":{"id":"other","type":"exact"}}}}`, "",
			`{"event":"signal/request","payload":{"video":{},"audio":{}}}`},
		{"switch to another pipeline", `{"event":"signal/video","payload":{"selector":{"id":"other","type":"exact"},"auto":true}}`, "room",
			`{"event":"signal/video","payload":{` + want + `}}`},
		{"switch, neko's default", `{"event":"signal/video","payload":{"selector":{"id":"other","type":"exact"}}}`, "", ""},
		{"video off, neko's default", `{"event":"signal/video","payload":{"disabled":true,"selector":{"id":"other","type":"exact"}}}`, "",
			`{"event":"signal/video","payload":{"disabled":true}}`},
		// Go's decoder, like neko's, matches keys in any case and lets the last
		// duplicate win; neko must only ever see the rebuilt message.
		{"other key case", `{"EVENT":"signal/video","PAYLOAD":{"SELECTOR":{"ID":"other","TYPE":"exact"}}}`, "room",
			`{"event":"signal/video","payload":{` + want + `}}`},
		{"duplicate keys", `{"event":"control/move","event":"signal/video","payload":{},"payload":{"selector":{"id":"other","type":"exact"}}}`, "room",
			`{"event":"signal/video","payload":{` + want + `}}`},
		{"other events pass", `{"event":"signal/answer","payload":{"sdp":"v=0"}}`, "room", `{"event":"signal/answer","payload":{"sdp":"v=0"}}`},
		{"other events without payload", ` {"event" : "control/request"} `, "room", `{"event":"control/request"}`},
		{"not JSON", `signal/video`, "room", ""},
		{"no event", `{"payload":{}}`, "room", ""},
		{"wrong payload type", `{"event":"signal/video","payload":"other"}`, "room", ""},
		{"an array", `[{"event":"signal/video"}]`, "room", ""},
	} {
		out, ok := PinStream([]byte(tc.in), tc.stream)
		if string(out) != tc.out || ok != (tc.out != "") {
			t.Errorf("%s:\n got %s (%v)\nwant %s", tc.name, out, ok, tc.out)
		}
	}
	if got := string(SelectStream("room")); got != `{"event":"signal/video","payload":{`+want+`}}` {
		t.Error("SelectStream:", got)
	}
}

func TestSocketURL(t *testing.T) {
	for base, want := range map[string]string{
		"http://room-default:8080":  "ws://room-default:8080/api/ws?token=a+b%26c",
		"https://neko.example/base": "wss://neko.example/base/api/ws?token=a+b%26c",
	} {
		c, err := NewClient(base, "secret")
		if err != nil {
			t.Fatal(err)
		}
		if got := c.SocketURL("a b&c"); got != want {
			t.Errorf("%s: %s, want %s", base, got, want)
		}
	}
}
