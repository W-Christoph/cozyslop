// Package relay is the media relay for rooms on other computers
// (docs/home-hosting.md, "The media relay"). neko sends each viewer its own
// copy of the stream; for a room at someone's home that is the home upload
// times the viewers. The relay instead watches the room once, as one neko
// member, and forwards the RTP packets to every viewer's own WebRTC
// connection with the server: the home uploads one stream.
//
// Nothing is decoded or encoded. Viewers' lost packets are resent from a
// buffer here (pion's NACK responder), and their requests for a keyframe
// are passed on to neko.
package relay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/pion/ice/v4"
	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"

	"cozycast/internal/neko"
)

// idleAfter is how long the stream from neko outlives its last viewer, so
// that a reload or a renewed connection does not restart it.
var idleAfter = 15 * time.Second

// keyframeEvery limits keyframe requests to neko: one per this, whatever
// the number of viewers asking.
const keyframeEvery = 500 * time.Millisecond

// switchWait is how long a joining viewer waits for neko to switch to the
// room's stream at most.
const switchWait = 3 * time.Second

type Config struct {
	Port     int        // UDP and TCP, for every viewer of every room
	PublicIP netip.Addr // announced to viewers; invalid: the host's own addresses
	Loopback bool       // also serve viewers on this machine (tests)
}

type Relay struct {
	viewers  *webrtc.API // to browsers: ICE lite on the shared port
	upstream *webrtc.API // to neko
	public   string      // the address announced to viewers; "" for the machine's own
	udp      ice.UDPMux
	tcp      net.Listener
	log      *slog.Logger

	mu    sync.Mutex
	rooms map[string]*room
}

// New opens the relay's port.
func New(cfg Config) (*Relay, error) {
	// One socket per address of this machine, as neko does: answers then
	// leave from the address the viewer wrote to. Every address stays a
	// candidate of its own, and the public address is written into the
	// offer instead (Join). Telling pion to announce it would make the
	// candidates equal, and pion then serves only the first address: in a
	// container on two networks, not the one viewers arrive on.
	opts := []ice.UDPMuxFromPortOption{ice.UDPMuxFromPortWithNetworks(ice.NetworkTypeUDP4)}
	if cfg.Loopback {
		opts = append(opts, ice.UDPMuxFromPortWithLoopback())
	}
	udp, err := ice.NewMultiUDPMuxFromPort(cfg.Port, opts...)
	if err != nil {
		return nil, err
	}
	tcp, err := net.Listen("tcp4", net.JoinHostPort("", strconv.Itoa(cfg.Port)))
	if err != nil {
		udp.Close()
		return nil, err
	}
	var viewers webrtc.SettingEngine
	viewers.SetLite(true)
	viewers.SetICEUDPMux(udp)
	viewers.SetICETCPMux(webrtc.NewICETCPMux(nil, tcp, 8))
	viewers.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4, webrtc.NetworkTypeTCP4})
	viewers.SetIncludeLoopbackCandidate(cfg.Loopback)
	var upstream webrtc.SettingEngine
	// neko's media is reached on this machine (see Source.MediaHost).
	upstream.SetIncludeLoopbackCandidate(true)
	upstream.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4, webrtc.NetworkTypeTCP4})
	r := &Relay{udp: udp, tcp: tcp, log: slog.With("component", "relay"), rooms: map[string]*room{}}
	if cfg.PublicIP.IsValid() {
		r.public = cfg.PublicIP.String()
	}
	if r.viewers, err = newAPI(viewers); err == nil {
		r.upstream, err = newAPI(upstream)
	}
	if err != nil {
		r.Close()
		return nil, err
	}
	return r, nil
}

func newAPI(se webrtc.SettingEngine) (*webrtc.API, error) {
	me := &webrtc.MediaEngine{}
	if err := me.RegisterDefaultCodecs(); err != nil {
		return nil, err
	}
	ir := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(me, ir); err != nil {
		return nil, err
	}
	return webrtc.NewAPI(webrtc.WithMediaEngine(me), webrtc.WithInterceptorRegistry(ir), webrtc.WithSettingEngine(se)), nil
}

// Close stops every room's stream and closes the port.
func (r *Relay) Close() {
	r.mu.Lock()
	rooms := r.rooms
	r.rooms = map[string]*room{}
	r.mu.Unlock()
	for _, rm := range rooms {
		rm.stop()
	}
	r.udp.Close()
	r.tcp.Close()
}

// Source is how the relay reaches a room's neko.
type Source struct {
	Neko   *neko.Client
	Stream func() string // the room's capture pipeline; "" for neko's default
	// MediaHost is where neko's media port is reached from this machine;
	// neko's candidates are rewritten to it. For a paired room that is the
	// server's own forwarding of the room's media port (127.0.0.1).
	MediaHost string
}

// Request is what a viewer asks for (neko's signal/request).
type Request struct{ NoVideo bool }

// Viewer is one browser tab's media connection.
type Viewer struct {
	room *room
	pc   *webrtc.PeerConnection
	once sync.Once
}

// Join connects a tab to the room's stream. The tab gets offer (as neko's
// "signal/provide" would bring it), answers with Answer and may add its
// candidates with Candidate.
func (r *Relay) Join(ctx context.Context, name string, src Source, req Request) (v *Viewer, offer string, err error) {
	rm, err := r.room(name, src)
	if err != nil {
		return nil, "", err
	}
	rm.awaitStream(ctx)
	pc, err := r.viewers.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return nil, "", err
	}
	v = &Viewer{room: rm, pc: pc}
	tracks := []*webrtc.TrackLocalStaticRTP{rm.audio}
	if !req.NoVideo {
		tracks = append(tracks, rm.video)
	}
	for _, track := range tracks {
		sender, err := pc.AddTrack(track)
		if err != nil {
			pc.Close()
			return nil, "", err
		}
		go rm.readRTCP(sender, track == rm.video)
	}
	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		switch s {
		case webrtc.PeerConnectionStateConnected:
			rm.keyframe() // the viewer needs one to start
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
			v.Close()
		}
	})
	desc, err := pc.CreateOffer(nil)
	if err == nil {
		gathered := webrtc.GatheringCompletePromise(pc)
		if err = pc.SetLocalDescription(desc); err == nil {
			select {
			case <-gathered:
			case <-ctx.Done():
				err = ctx.Err()
			}
		}
	}
	if err != nil {
		pc.Close()
		return nil, "", err
	}
	rm.add(v)
	return v, RewriteSDP(pc.LocalDescription().SDP, r.public), nil
}

// Answer completes the connection with the tab's answer.
func (v *Viewer) Answer(sdp string) error {
	return v.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: sdp})
}

// Candidate adds one of the tab's ICE candidates.
func (v *Viewer) Candidate(c webrtc.ICECandidateInit) error {
	return v.pc.AddICECandidate(c)
}

// Close ends the tab's connection; the room's stream stops a little after
// its last viewer.
func (v *Viewer) Close() {
	v.once.Do(func() {
		v.pc.Close()
		v.room.remove(v)
	})
}

// Viewers is the number of tabs watching name through the relay.
func (r *Relay) Viewers(name string) int {
	r.mu.Lock()
	rm := r.rooms[name]
	r.mu.Unlock()
	if rm == nil {
		return 0
	}
	rm.mu.Lock()
	defer rm.mu.Unlock()
	return len(rm.viewers)
}

// Sync has name's stream from neko follow the room's stream setting now,
// not at the next check.
func (r *Relay) Sync(name string) {
	r.mu.Lock()
	rm := r.rooms[name]
	r.mu.Unlock()
	if rm != nil {
		rm.sync()
	}
}

func (r *Relay) room(name string, src Source) (*room, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rm := r.rooms[name]
	if rm == nil {
		video, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{
			MimeType: webrtc.MimeTypeH264, ClockRate: 90000,
			// The rooms' x264 pipelines: constrained baseline.
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
		}, "video", "cozycast")
		if err != nil {
			return nil, err
		}
		audio, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{
			MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2, SDPFmtpLine: "minptime=10;useinbandfec=1",
		}, "audio", "cozycast")
		if err != nil {
			return nil, err
		}
		rm = &room{relay: r, name: name, src: src, video: video, audio: audio, viewers: map[*Viewer]bool{},
			changed: make(chan struct{}), kick: make(chan struct{}, 1), log: r.log.With("room", name)}
		r.rooms[name] = rm
	}
	rm.setSource(src)
	return rm, nil
}

// room is one room's stream from neko, shared by its viewers.
type room struct {
	relay        *Relay
	name         string
	video, audio *webrtc.TrackLocalStaticRTP
	// The numbering of what is written to video and audio.
	videoRTP, audioRTP continuity
	kick               chan struct{} // wakes the stream to check the room's stream setting
	log                *slog.Logger

	mu       sync.Mutex
	src      Source
	viewers  map[*Viewer]bool
	cancel   context.CancelFunc // of the stream from neko; nil while stopped
	done     chan struct{}
	last     chan struct{} // closed when the latest stream, running or not, has ended
	idle     *time.Timer
	pli      func() // asks neko for a keyframe; nil until video arrives
	lastPLI  time.Time
	upstream bool // connected to neko
	// playing is the capture pipeline neko says it sends; "" when unknown
	// or not connected. changed is closed and replaced when it changes.
	playing string
	changed chan struct{}
}

// setSource follows the room to a new neko (its address or token changed).
func (rm *room) setSource(src Source) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	changed := rm.src.Neko != src.Neko
	rm.src = src
	if changed && rm.cancel != nil {
		rm.stopLocked()
		if len(rm.viewers) > 0 {
			rm.startLocked() // begins when the stopped one has ended
		}
	}
}

func (rm *room) add(v *Viewer) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.viewers[v] = true
	if rm.idle != nil {
		rm.idle.Stop()
		rm.idle = nil
	}
	if rm.cancel == nil {
		rm.startLocked()
	}
}

func (rm *room) remove(v *Viewer) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	delete(rm.viewers, v)
	if len(rm.viewers) > 0 || rm.cancel == nil || rm.idle != nil {
		return
	}
	var idle *time.Timer
	idle = time.AfterFunc(idleAfter, func() {
		// Checked and stopped under one lock: a viewer joining now either
		// keeps this stream or starts the next.
		rm.mu.Lock()
		if rm.idle != idle || len(rm.viewers) > 0 {
			rm.mu.Unlock()
			return
		}
		rm.idle = nil
		wait := rm.stopLocked()
		rm.mu.Unlock()
		rm.log.Info("no viewers left; stopping the room's stream")
		wait()
	})
	rm.idle = idle
}

func (rm *room) startLocked() {
	ctx, cancel := context.WithCancel(context.Background())
	done, before := make(chan struct{}), rm.last
	rm.cancel, rm.done, rm.last = cancel, done, done
	go func() {
		defer close(done)
		if before != nil {
			<-before // both are the same neko member: one at a time
		}
		rm.run(ctx)
	}()
}

// stop ends the stream from neko and waits for it.
func (rm *room) stop() {
	rm.mu.Lock()
	wait := rm.stopLocked()
	rm.mu.Unlock()
	wait()
}

// stopLocked ends the stream from neko; wait returns once it has ended.
func (rm *room) stopLocked() (wait func()) {
	cancel, done := rm.cancel, rm.done
	rm.cancel, rm.done = nil, nil
	if cancel == nil {
		return func() {}
	}
	cancel()
	return func() { <-done }
}

func (rm *room) sync() {
	select {
	case rm.kick <- struct{}{}:
	default: // a check is due already
	}
}

func (rm *room) setPlaying(stream string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if rm.playing != stream {
		rm.playing = stream
		close(rm.changed)
		rm.changed = make(chan struct{})
	}
}

// awaitStream waits while neko sends another stream than the room's. A tab
// that renews its connection because the frames get bigger must not get
// the smaller ones first (web/src/neko/client.ts, renewPeer).
func (rm *room) awaitStream(ctx context.Context) {
	timeout := time.NewTimer(switchWait)
	defer timeout.Stop()
	for {
		rm.mu.Lock()
		src, playing, changed := rm.src, rm.playing, rm.changed
		rm.mu.Unlock()
		if want := src.Stream(); want == "" || playing == "" || playing == want {
			return
		}
		rm.sync()
		select {
		case <-changed:
		case <-timeout.C:
			return
		case <-ctx.Done():
			return
		}
	}
}

// keyframe asks neko for one, at most once per keyframeEvery. neko's x264
// also sends one every two seconds, so a lost request only delays.
func (rm *room) keyframe() {
	rm.mu.Lock()
	pli := rm.pli
	if pli == nil || time.Since(rm.lastPLI) < keyframeEvery {
		rm.mu.Unlock()
		return
	}
	rm.lastPLI = time.Now()
	rm.mu.Unlock()
	pli()
}

// readRTCP reads a viewer's reports (which runs its NACK responder) and
// passes keyframe requests on.
func (rm *room) readRTCP(sender *webrtc.RTPSender, video bool) {
	for {
		packets, _, err := sender.ReadRTCP()
		if err != nil {
			return
		}
		if !video {
			continue
		}
		for _, p := range packets {
			switch p.(type) {
			case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
				rm.keyframe()
			}
		}
	}
}

// run keeps the stream from neko up until ctx ends.
func (rm *room) run(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		start := time.Now()
		err := rm.connect(ctx)
		if ctx.Err() != nil {
			return
		}
		rm.log.Warn("stream from neko ended, reconnecting", "err", err)
		if time.Since(start) > time.Minute {
			backoff = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 10*time.Second)
	}
}

type message struct {
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// connect watches the room as neko member RelayID until something fails.
func (rm *room) connect(ctx context.Context) error {
	rm.mu.Lock()
	src := rm.src
	rm.mu.Unlock()
	nc := src.Neko
	password := randomHex(16)
	_ = nc.DeleteMember(ctx, neko.RelayID)
	err := nc.CreateMember(ctx, neko.RelayID, password, neko.Profile{
		Name: "CozyCast relay", CanLogin: true, CanConnect: true, CanWatch: true,
	})
	if err != nil {
		return err
	}
	token, err := nc.Login(ctx, neko.RelayID, password)
	if err != nil {
		return err
	}
	dialCtx, cancelDial := context.WithTimeout(ctx, 10*time.Second)
	conn, _, err := websocket.Dial(dialCtx, nc.SocketURL(token), &websocket.DialOptions{HTTPClient: nc.StreamClient()})
	cancelDial()
	if err != nil {
		return err
	}
	conn.SetReadLimit(4 << 20)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer conn.CloseNow()
	defer func() {
		// Not with ctx: it has ended when the stream is stopped.
		dctx, dcancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer dcancel()
		_ = nc.DeleteMember(dctx, neko.RelayID)
	}()

	send := func(event string, payload any) error {
		data, _ := json.Marshal(map[string]any{"event": event, "payload": payload})
		wctx, wcancel := context.WithTimeout(ctx, 10*time.Second)
		defer wcancel()
		return conn.Write(wctx, websocket.MessageText, data)
	}
	selector := func(stream string) any {
		if stream == "" {
			return nil
		}
		return map[string]string{"id": stream, "type": "exact"}
	}
	stream := src.Stream()
	if err := send("signal/request", map[string]any{"video": map[string]any{"selector": selector(stream)}, "audio": map[string]any{}}); err != nil {
		return err
	}

	messages := make(chan message)
	readErr := make(chan error, 1)
	go func() {
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				readErr <- err
				return
			}
			var m message
			if json.Unmarshal(data, &m) == nil {
				select {
				case messages <- m:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	var pc *webrtc.PeerConnection
	// neko sends its candidates before the offer they belong to.
	var pending []webrtc.ICECandidateInit
	defer func() {
		if pc != nil {
			pc.Close()
		}
		rm.mu.Lock()
		rm.pli, rm.upstream = nil, false
		rm.mu.Unlock()
		rm.setPlaying("")
	}()
	failed := make(chan error, 1)
	answer := func(sdp string) error {
		err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: RewriteSDP(sdp, src.MediaHost)})
		if err != nil {
			return err
		}
		for _, c := range pending {
			_ = pc.AddICECandidate(c)
		}
		pending = nil
		ans, err := pc.CreateAnswer(nil)
		if err != nil {
			return err
		}
		if err := pc.SetLocalDescription(ans); err != nil {
			return err
		}
		return send("signal/answer", map[string]string{"sdp": ans.SDP})
	}
	// follow keeps to the room's stream setting: neko switches the relay's
	// session, which every viewer watches.
	follow := func() error {
		s := src.Stream()
		if s == stream {
			return nil
		}
		stream = s
		return send("signal/video", map[string]any{"selector": selector(stream)})
	}
	check := time.NewTicker(time.Second)
	defer check.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readErr:
			return err
		case err := <-failed:
			return err
		case <-check.C:
			if err := follow(); err != nil {
				return err
			}
		case <-rm.kick:
			if err := follow(); err != nil {
				return err
			}
		case m := <-messages:
			switch m.Event {
			case "signal/provide":
				var p struct {
					SDP   string `json:"sdp"`
					Video struct {
						ID string `json:"id"`
					} `json:"video"`
				}
				if err := json.Unmarshal(m.Payload, &p); err != nil {
					return err
				}
				if pc != nil {
					pc.Close()
				}
				if pc, err = rm.relay.upstream.NewPeerConnection(webrtc.Configuration{}); err != nil {
					return err
				}
				current := pc
				pc.OnICECandidate(func(c *webrtc.ICECandidate) {
					if c != nil {
						_ = send("signal/candidate", c.ToJSON())
					}
				})
				pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
					switch s {
					case webrtc.PeerConnectionStateConnected:
						rm.mu.Lock()
						rm.upstream = true
						rm.mu.Unlock()
						rm.log.Info("watching the room for its viewers")
					case webrtc.PeerConnectionStateFailed:
						select {
						case failed <- errors.New("media connection to neko failed"):
						default:
						}
					}
				})
				pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
					rm.forward(current, track)
				})
				if err := answer(p.SDP); err != nil {
					return err
				}
				rm.setPlaying(p.Video.ID)
			case "signal/video":
				// neko has switched the stream.
				var p struct {
					ID string `json:"id"`
				}
				if json.Unmarshal(m.Payload, &p) == nil {
					rm.setPlaying(p.ID)
				}
			case "signal/offer":
				var p struct {
					SDP string `json:"sdp"`
				}
				if pc == nil || json.Unmarshal(m.Payload, &p) != nil {
					continue
				}
				if err := answer(p.SDP); err != nil {
					return err
				}
			case "signal/candidate":
				var c webrtc.ICECandidateInit
				if json.Unmarshal(m.Payload, &c) != nil {
					continue
				}
				c.Candidate = RewriteCandidate(c.Candidate, src.MediaHost)
				if pc == nil || pc.RemoteDescription() == nil {
					pending = append(pending, c)
					continue
				}
				_ = pc.AddICECandidate(c)
			case "system/disconnect":
				return fmt.Errorf("neko disconnected the relay: %s", m.Payload)
			}
		}
	}
}

// forward copies a track from neko to the room's track all viewers share.
func (rm *room) forward(pc *webrtc.PeerConnection, track *webrtc.TrackRemote) {
	local, numbering := rm.audio, &rm.audioRTP
	if track.Kind() == webrtc.RTPCodecTypeVideo {
		local, numbering = rm.video, &rm.videoRTP
		ssrc := uint32(track.SSRC())
		rm.mu.Lock()
		rm.pli = func() {
			_ = pc.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: ssrc}})
		}
		rm.mu.Unlock()
		rm.keyframe()
	}
	stream, clockRate := numbering.begin(), track.Codec().ClockRate
	go func() {
		for {
			p, _, err := track.ReadRTP()
			if err != nil || !numbering.renumber(stream, p, clockRate) {
				return
			}
			// neko's header extensions are numbered as agreed between neko
			// and the relay, which is not what a viewer agreed to.
			p.Extension, p.Extensions = false, nil
			// An error is of single viewers, on their way out; the others
			// got the packet.
			_ = local.WriteRTP(p)
		}
	}()
}

// continuity keeps a track's sequence numbers and timestamps going up when
// the stream from neko starts again. The viewers' connections outlive it,
// and would take a new stream's lower numbers for packets from the past.
type continuity struct {
	mu     sync.Mutex
	stream int  // counts the streams from neko
	fresh  bool // the stream's first packet is still to come
	sent   bool
	// The highest sequence number sent, its timestamp and when.
	seq       uint16
	timestamp uint32
	at        time.Time
	// Added to the stream's own numbers; both wrap around.
	seqOffset       uint16
	timestampOffset uint32
}

// begin starts a stream from neko and returns its number for renumber.
func (c *continuity) begin() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stream++
	c.fresh = true
	return c.stream
}

// renumber changes p to follow what was sent before; ok is false if a
// later stream has begun.
func (c *continuity) renumber(stream int, p *rtp.Packet, clockRate uint32) (ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if stream != c.stream {
		return false
	}
	if c.fresh {
		c.fresh = false
		if c.sent {
			// Right after the last packet, and as much later as it is.
			gap := uint32(time.Since(c.at).Seconds()*float64(clockRate)) + 1
			c.seqOffset = c.seq + 1 - p.SequenceNumber
			c.timestampOffset = c.timestamp + gap - p.Timestamp
		}
	}
	p.SequenceNumber += c.seqOffset
	p.Timestamp += c.timestampOffset
	if !c.sent || int16(p.SequenceNumber-c.seq) > 0 {
		c.sent, c.seq, c.timestamp, c.at = true, p.SequenceNumber, p.Timestamp, time.Now()
	}
	return true
}

// RewriteSDP points the candidates in an SDP at host: neko's at where the
// relay reaches it (Source.MediaHost), the relay's own at its public
// address.
func RewriteSDP(sdp, host string) string {
	if host == "" {
		return sdp
	}
	lines := strings.Split(sdp, "\r\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "a=candidate:") {
			lines[i] = "a=" + RewriteCandidate(strings.TrimPrefix(line, "a="), host)
		}
	}
	return strings.Join(lines, "\r\n")
}

// RewriteCandidate replaces a candidate's address with host, keeping its
// port: "candidate:1 1 udp 2130706431 203.0.113.10 52100 typ host".
func RewriteCandidate(candidate, host string) string {
	fields := strings.Fields(candidate)
	if host == "" || len(fields) < 8 || !strings.HasPrefix(fields[0], "candidate:") || fields[6] != "typ" {
		return candidate
	}
	fields[4] = host
	return strings.Join(fields, " ")
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
