package relay

import (
	"context"
	"encoding/json"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"

	"cozycast/internal/neko"
	"cozycast/internal/neko/nekotest"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// fakeMedia plays neko's WebRTC side for the member the relay watches with,
// set up like a paired room's neko: one media port, ICE lite, and an
// announced address that is not where the relay reaches it.
type fakeMedia struct {
	t            *testing.T
	api          *webrtc.API
	video, audio *webrtc.TrackLocalStaticRTP

	mu        sync.Mutex
	pc        *webrtc.PeerConnection
	requests  []string // signal/request and signal/video payloads
	keyframes int      // asked for by the relay
}

func newFakeMedia(t *testing.T, fake *nekotest.Server) *fakeMedia {
	t.Helper()
	udp, err := net.ListenPacket("udp4", net.JoinHostPort("", "0"))
	if err != nil {
		t.Fatal(err)
	}
	var se webrtc.SettingEngine
	se.SetLite(true)
	se.SetICEUDPMux(webrtc.NewICEUDPMux(nil, udp))
	se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	se.SetNAT1To1IPs([]string{"203.0.113.10"}, webrtc.ICECandidateTypeHost)
	api, err := newAPI(se)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeMedia{t: t, api: api}
	f.video, _ = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000,
		SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f"}, "video", "stream")
	f.audio, _ = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2}, "audio", "stream")
	fake.OnMessage = f.onMessage
	t.Cleanup(func() {
		f.mu.Lock()
		if f.pc != nil {
			f.pc.Close()
		}
		f.mu.Unlock()
		udp.Close()
	})

	// The room's picture and sound, from the start.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for seq := uint16(0); ; seq++ {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
			for track, payload := range map[*webrtc.TrackLocalStaticRTP]string{f.video: "picture", f.audio: "sound"} {
				_ = track.WriteRTP(&rtp.Packet{
					Header:  rtp.Header{Version: 2, SequenceNumber: seq, Timestamp: uint32(seq) * 900},
					Payload: []byte(payload),
				})
			}
		}
	}()
	return f
}

func (f *fakeMedia) onMessage(ctx context.Context, id string, data []byte, reply func([]byte) error) {
	if id != neko.RelayID {
		return
	}
	var m message
	if json.Unmarshal(data, &m) != nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch m.Event {
	case "signal/request":
		f.requests = append(f.requests, m.Event+" "+string(m.Payload))
		pc, err := f.api.NewPeerConnection(webrtc.Configuration{})
		if err != nil {
			f.t.Error(err)
			return
		}
		f.pc = pc
		for _, track := range []*webrtc.TrackLocalStaticRTP{f.audio, f.video} {
			sender, err := pc.AddTrack(track)
			if err != nil {
				f.t.Error(err)
				return
			}
			go func() {
				for {
					packets, _, err := sender.ReadRTCP()
					if err != nil {
						return
					}
					for _, p := range packets {
						if _, ok := p.(*rtcp.PictureLossIndication); ok {
							f.mu.Lock()
							f.keyframes++
							f.mu.Unlock()
						}
					}
				}
			}()
		}
		offer, err := pc.CreateOffer(nil)
		if err != nil {
			f.t.Error(err)
			return
		}
		// As neko: the candidates first, each in a message of its own, then
		// the offer without them.
		gathered := make(chan struct{})
		pc.OnICECandidate(func(c *webrtc.ICECandidate) {
			if c == nil {
				close(gathered)
				return
			}
			out, _ := json.Marshal(map[string]any{"event": "signal/candidate", "payload": c.ToJSON()})
			_ = reply(out)
		})
		if err := pc.SetLocalDescription(offer); err != nil {
			f.t.Error(err)
			return
		}
		<-gathered
		if strings.Contains(offer.SDP, "a=candidate") {
			f.t.Error("the fake neko's offer has its candidates")
		}
		out, _ := json.Marshal(map[string]any{"event": "signal/provide",
			"payload": map[string]any{"sdp": offer.SDP, "video": map[string]any{"id": selected(m.Payload, true)}}})
		_ = reply(out)
	case "signal/video":
		// As neko: the switch takes a moment, then it is confirmed.
		id := selected(m.Payload, false)
		go func() {
			time.Sleep(50 * time.Millisecond)
			f.mu.Lock()
			f.requests = append(f.requests, m.Event+" "+string(m.Payload))
			f.mu.Unlock()
			out, _ := json.Marshal(map[string]any{"event": "signal/video", "payload": map[string]any{"id": id}})
			_ = reply(out)
		}()
	case "signal/answer":
		var p struct {
			SDP string `json:"sdp"`
		}
		_ = json.Unmarshal(m.Payload, &p)
		if err := f.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: p.SDP}); err != nil {
			f.t.Error(err)
		}
	}
}

// selected is the pipeline a signal/request (nested) or signal/video asks for.
func selected(payload []byte, nested bool) string {
	var video struct {
		Selector struct{ ID string } `json:"selector"`
	}
	if nested {
		var req struct {
			Video json.RawMessage `json:"video"`
		}
		_ = json.Unmarshal(payload, &req)
		payload = req.Video
	}
	_ = json.Unmarshal(payload, &video)
	return video.Selector.ID
}

func (f *fakeMedia) seen() (requests []string, keyframes int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...), f.keyframes
}

// browser is a tab's side: it answers the relay's offer and reports the
// payload of the first packet of each kind it gets.
type browser struct {
	pc   *webrtc.PeerConnection
	got  chan string
	done chan struct{}
}

func watch(t *testing.T, ctx context.Context, r *Relay, src Source, req Request) (*Viewer, *browser, string) {
	t.Helper()
	v, offer, err := r.Join(ctx, "home", src, req)
	if err != nil {
		t.Fatal(err)
	}
	var se webrtc.SettingEngine
	se.SetIncludeLoopbackCandidate(true)
	se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	api, err := newAPI(se)
	if err != nil {
		t.Fatal(err)
	}
	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	b := &browser{pc: pc, got: make(chan string, 2), done: make(chan struct{})}
	t.Cleanup(func() { pc.Close() })
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if p, _, err := track.ReadRTP(); err == nil {
			b.got <- track.Kind().String() + " " + string(p.Payload)
		}
	})
	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		if s == webrtc.PeerConnectionStateClosed || s == webrtc.PeerConnectionStateFailed || s == webrtc.PeerConnectionStateDisconnected {
			select {
			case <-b.done:
			default:
				close(b.done)
			}
		}
	})
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offer}); err != nil {
		t.Fatal(err)
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		t.Fatal(err)
	}
	if err := v.Answer(answer.SDP); err != nil {
		t.Fatal(err)
	}
	return v, b, offer
}

func (b *browser) receive(t *testing.T, ctx context.Context, n int) map[string]bool {
	t.Helper()
	got := map[string]bool{}
	for len(got) < n {
		select {
		case s := <-b.got:
			got[s] = true
		case <-ctx.Done():
			t.Fatalf("got %v, waiting for %d kinds of packets", got, n)
		}
	}
	return got
}

func eventually(t *testing.T, ctx context.Context, what string, ok func() bool) {
	t.Helper()
	for !ok() {
		select {
		case <-ctx.Done():
			t.Fatal("never happened: " + what)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestRelayWatchesOnceForAllViewers(t *testing.T) {
	idleAfter = 100 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fake := nekotest.New(t, "secret")
	media := newFakeMedia(t, fake)
	nc, err := neko.NewClient(fake.URL(), "secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	// The announced address stands in for the server's public one.
	r, err := New(Config{Port: freePort(t), PublicIP: netip.MustParseAddr("127.0.0.1"), Loopback: true})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var mu sync.Mutex
	stream := "720p"
	src := Source{Neko: nc, MediaHost: "127.0.0.1", Stream: func() string {
		mu.Lock()
		defer mu.Unlock()
		return stream
	}}

	first, firstTab, offer := watch(t, ctx, r, src, Request{})
	if !strings.Contains(offer, " 127.0.0.1 ") || strings.Contains(offer, "203.0.113.10") {
		t.Fatalf("the relay's offer does not announce its own address only:\n%s", offer)
	}
	if got := firstTab.receive(t, ctx, 2); !got["video picture"] || !got["audio sound"] {
		t.Fatal("first viewer got", got)
	}
	// A viewer who wants no picture gets none.
	second, secondTab, offer := watch(t, ctx, r, src, Request{NoVideo: true})
	if strings.Contains(offer, "m=video") {
		t.Fatal("offer for sound only has video")
	}
	if got := secondTab.receive(t, ctx, 1); !got["audio sound"] {
		t.Fatal("second viewer got", got)
	}
	if n := r.Viewers("home"); n != 2 {
		t.Fatal("viewers:", n)
	}

	// neko was asked once, for the room's stream, and for a keyframe.
	want := `signal/request {"audio":{},"video":{"selector":{"id":"720p","type":"exact"}}}`
	if requests, _ := media.seen(); len(requests) != 1 || requests[0] != want {
		t.Fatal("neko was asked", requests)
	}
	eventually(t, ctx, "a keyframe request", func() bool { _, n := media.seen(); return n > 0 })
	if p, ok := fake.Member(neko.RelayID); !ok || !p.CanWatch || p.CanHost {
		t.Fatalf("the relay's member: %+v, %v", p, ok)
	}

	// The room's stream setting changes: the relay's session follows, and a
	// tab that connects anew is only let in once neko has switched (it
	// would get the old stream's frames first).
	mu.Lock()
	stream = "480p"
	mu.Unlock()
	renewed, renewedTab, _ := watch(t, ctx, r, src, Request{})
	if requests, _ := media.seen(); len(requests) != 2 || requests[1] != `signal/video {"selector":{"id":"480p","type":"exact"}}` {
		t.Fatal("a viewer joined before neko switched streams; neko was asked", requests)
	}
	if got := renewedTab.receive(t, ctx, 2); !got["video picture"] {
		t.Fatal("viewer after the stream change got", got)
	}
	renewed.Close()

	// The stream outlives a viewer but not the last one.
	first.Close()
	select {
	case <-firstTab.done:
	case <-ctx.Done():
		t.Fatal("a closed viewer stayed connected")
	}
	time.Sleep(2 * idleAfter)
	if _, ok := fake.Member(neko.RelayID); !ok {
		t.Fatal("the relay left with a viewer watching")
	}
	second.Close()
	if err := fake.WaitMemberDeleted(ctx, neko.RelayID); err != nil {
		t.Fatal("the relay kept watching an empty room:", err)
	}

	// The next viewer starts it again.
	_, thirdTab, _ := watch(t, ctx, r, src, Request{})
	if got := thirdTab.receive(t, ctx, 2); !got["video picture"] || !got["audio sound"] {
		t.Fatal("third viewer got", got)
	}
}

// The relay's port is reached on any of the machine's addresses, like a
// published port of a container that is on several networks.
func TestRelayAnswersOnEveryAddress(t *testing.T) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	var local []netip.Addr
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && !n.IP.IsLoopback() {
			ip, _ := netip.AddrFromSlice(n.IP.To4())
			local = append(local, ip)
		}
	}
	if len(local) == 0 {
		t.Skip("this machine has no address besides loopback")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fake := nekotest.New(t, "secret")
	newFakeMedia(t, fake)
	nc, err := neko.NewClient(fake.URL(), "secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	src := Source{Neko: nc, MediaHost: "127.0.0.1", Stream: func() string { return "" }}
	// Loopback too, so that the machine has two addresses at least.
	for _, public := range append(local, netip.MustParseAddr("127.0.0.1")) {
		r, err := New(Config{Port: freePort(t), PublicIP: public, Loopback: true})
		if err != nil {
			t.Fatal(err)
		}
		v, tab, offer := watch(t, ctx, r, src, Request{})
		if n := strings.Count(offer, " "+public.String()+" "); n == 0 || n != strings.Count(offer, "a=candidate:") {
			t.Fatalf("offer for %s announces other addresses:\n%s", public, offer)
		}
		if got := tab.receive(t, ctx, 2); !got["video picture"] {
			t.Fatalf("viewer reaching the relay at %s got %v", public, got)
		}
		v.Close()
		r.Close()
	}
}

func TestContinuityAcrossStreams(t *testing.T) {
	var c continuity
	send := func(stream int, seq uint16, timestamp uint32) (uint16, uint32, bool) {
		p := &rtp.Packet{Header: rtp.Header{SequenceNumber: seq, Timestamp: timestamp}}
		ok := c.renumber(stream, p, 90000)
		return p.SequenceNumber, p.Timestamp, ok
	}
	// The first stream goes out as it comes, also around the wrap and with
	// a late packet.
	first := c.begin()
	for _, seq := range []uint16{65534, 65535, 0, 2, 1} {
		if got, ts, ok := send(first, seq, uint32(seq)*3000+7); !ok || got != seq || ts != uint32(seq)*3000+7 {
			t.Fatalf("first stream: %d became %d (timestamp %d), %v", seq, got, ts, ok)
		}
	}
	// The next one starts with lower numbers; viewers see it carry on.
	second := c.begin()
	if _, _, ok := send(first, 3, 9000); ok {
		t.Fatal("a packet of the ended stream was sent")
	}
	seq, ts, ok := send(second, 1000, 50)
	if !ok || seq != 3 || int32(ts-(2*3000+7)) <= 0 {
		t.Fatalf("second stream starts at %d, timestamp %d, %v", seq, ts, ok)
	}
	if next, nextTS, _ := send(second, 1001, 3050); next != 4 || nextTS != ts+3000 {
		t.Fatalf("second stream goes on with %d, timestamp %d after %d", next, nextTS, ts)
	}
}

func TestRewriteCandidate(t *testing.T) {
	for in, want := range map[string]string{
		"candidate:1 1 udp 2130706431 203.0.113.10 52100 typ host":             "candidate:1 1 udp 2130706431 127.0.0.1 52100 typ host",
		"candidate:2 1 tcp 1671430143 10.0.0.5 52100 typ host tcptype passive": "candidate:2 1 tcp 1671430143 127.0.0.1 52100 typ host tcptype passive",
		"candidate:broken": "candidate:broken",
		"":                 "",
	} {
		if got := RewriteCandidate(in, "127.0.0.1"); got != want {
			t.Errorf("RewriteCandidate(%q) = %q, want %q", in, got, want)
		}
	}
	if got := RewriteCandidate("candidate:1 1 udp 1 203.0.113.10 52100 typ host", ""); !strings.Contains(got, "203.0.113.10") {
		t.Error("without a host the candidate changed:", got)
	}
	sdp := "v=0\r\na=candidate:1 1 udp 1 203.0.113.10 52100 typ host\r\na=end-of-candidates\r\n"
	if got := RewriteSDP(sdp, "127.0.0.1"); got != strings.Replace(sdp, "203.0.113.10", "127.0.0.1", 1) {
		t.Errorf("RewriteSDP: %q", got)
	}
}
