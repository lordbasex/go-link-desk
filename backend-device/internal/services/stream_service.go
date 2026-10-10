// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/encoder"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/signalclient"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/testpattern"
)

// RTCSignal is the WebRTC part of the application payload carried inside
// signalhub "signal" messages. signalhub never reads it.
type RTCSignal struct {
	Kind      string                   `json:"kind"` // "offer", "answer" or "candidate"
	SDP       string                   `json:"sdp,omitempty"`
	Candidate *webrtc.ICECandidateInit `json:"candidate,omitempty"`
}

// StreamConfig configures StreamService. Zero values get defaults.
type StreamConfig struct {
	// Width, Height and FPS are the test card's (default 960x720 at 30).
	Width, Height int
	FPS           int
	BitrateKbps   int // default 2500
	// IncludeLoopback also gathers 127.0.0.1 candidates (tests on
	// machines without a network).
	IncludeLoopback bool
	// UDPPort, when set, carries every WebRTC connection on this single
	// UDP port (ICE UDP mux), so one port forward on a router is enough.
	UDPPort int
	// AnnounceIPs are extra addresses where browsers can reach UDPPort
	// (the WAN side of a router that forwards it). Each one is sent as an
	// additional host candidate.
	AnnounceIPs []string
	// API, when set, is a WebRTC API shared with other connections, so all
	// use the same UDP port.
	API *webrtc.API
	// Welcome is what the viewer learns about the host when its control
	// channel opens: the computer's name and OS (the room's public meta).
	Welcome SessionMeta
	// Source names what sessions show, for the viewer ("test-pattern" or
	// "screen").
	Source string
	Logger *slog.Logger
}

// NewWebRTCAPI builds the WebRTC API. udpPort > 0 carries every connection
// on that single UDP port (ICE UDP mux).
func NewWebRTCAPI(udpPort int, includeLoopback bool) (*webrtc.API, error) {
	se := webrtc.SettingEngine{}
	se.SetIncludeLoopbackCandidate(includeLoopback)
	if udpPort > 0 {
		conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: udpPort})
		if err != nil {
			return nil, fmt.Errorf("stream: UDP port %d: %w", udpPort, err)
		}
		se.SetICEUDPMux(webrtc.NewICEUDPMux(nil, conn))
		se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	}
	// Pion's default codecs and interceptors, plus the playout delay
	// (playout_delay.go).
	me := &webrtc.MediaEngine{}
	if err := me.RegisterDefaultCodecs(); err != nil {
		return nil, err
	}
	if err := me.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{URI: playoutDelayURI}, webrtc.RTPCodecTypeVideo); err != nil {
		return nil, err
	}
	reg := &interceptor.Registry{}
	// First, so it sits closest to the network (sim_loss.go; tests only).
	if pct := simLossFromEnv(); pct > 0 {
		reg.Add(&simLossFactory{pct: pct})
	}
	if err := webrtc.RegisterDefaultInterceptors(me, reg); err != nil {
		return nil, err
	}
	reg.Add(playoutDelays)
	return webrtc.NewAPI(webrtc.WithSettingEngine(se), webrtc.WithMediaEngine(me), webrtc.WithInterceptorRegistry(reg)), nil
}

// How a connection that drops is brought back: an ICE restart (a new
// offer with new ICE credentials over signalhub) once it has been
// disconnected for restartAfter, or at once when it fails, up to
// maxRestarts times in a row before the session ends.
const (
	restartAfter = 3 * time.Second
	maxRestarts  = 3
)

// maxControlMessage bounds a viewer's control message (clipboard pieces
// are 16 KiB).
const maxControlMessage = 64 << 10

// maxPendingCandidates bounds the ICE candidates kept while an answer is
// on its way: a browser sends a handful, never hundreds.
const maxPendingCandidates = 50

type viewer struct {
	id      string
	pc      *webrtc.PeerConnection
	control *webrtc.DataChannel
	video   *webrtc.TrackLocalStaticSample
	// keyframe asks this viewer's encoder for a keyframe: a new viewer, a
	// Picture Loss Indication from its browser, or a connection back.
	keyframe atomic.Bool
	playout  playoutLadder // under mu

	mu        sync.Mutex
	remoteSet bool
	pending   []webrtc.ICECandidateInit
	restarts  int         // ICE restarts since the connection was last up
	restart   *time.Timer // a pending restart while disconnected
	closed    bool
}

// viewerEnc is one viewer's encoder, owned by the source goroutine.
type viewerEnc struct {
	vp8          *encoder.VP8
	w, h, kbps   int
	sent, bytes  int // in the current one-second window
	fps, kbpsOut float64
}

// StreamService sends the session's video to its viewer over WebRTC, with
// a "control" DataChannel (reliable, ordered, JSON) for everything else.
// Each viewer has its own track and encoder, so a keyframe one viewer
// needs never costs the others anything.
type StreamService struct {
	cfg  StreamConfig
	log  *slog.Logger
	ice  *ICEStore
	api  *webrtc.API
	kbps atomic.Int64
	// sendErrAt is when a control message that could not be sent was last
	// logged (unix nanoseconds); see sendText.
	sendErrAt atomic.Int64

	// Owned by the source goroutine (Run).
	encs   map[string]*viewerEnc
	window time.Time

	mu        sync.Mutex
	sender    Sender
	viewers   map[string]*viewer
	source    MediaSource
	pings     map[uint64]time.Time
	pingSeq   uint64
	rtt       map[string]int // the last round trip per viewer, ms
	stats     map[string]streamStats
	onControl func(peerID, kind string, data []byte)
	onClosed  func(peerID, reason string)
	onStats   func(models.StreamStatus)
}

// NewStreamService builds the service. Call Run to start the source.
func NewStreamService(cfg StreamConfig, ice *ICEStore) (*StreamService, error) {
	if cfg.Width <= 0 || cfg.Height <= 0 {
		cfg.Width, cfg.Height = 960, 720
	}
	if cfg.FPS <= 0 {
		cfg.FPS = 30
	}
	if cfg.BitrateKbps <= 0 {
		cfg.BitrateKbps = 2500
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Source == "" {
		cfg.Source = "test-pattern"
	}
	api := cfg.API
	if api == nil {
		var err error
		if api, err = NewWebRTCAPI(cfg.UDPPort, cfg.IncludeLoopback); err != nil {
			return nil, err
		}
	}
	s := &StreamService{
		cfg:     cfg,
		log:     cfg.Logger,
		ice:     ice,
		api:     api,
		encs:    make(map[string]*viewerEnc),
		viewers: make(map[string]*viewer),
		pings:   make(map[uint64]time.Time),
		rtt:     make(map[string]int),
		stats:   make(map[string]streamStats),
	}
	s.kbps.Store(int64(cfg.BitrateKbps))
	return s, nil
}

// SetSender wires the signaling client.
func (s *StreamService) SetSender(sender Sender) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sender = sender
}

// OnControl receives every control message from a viewer other than the
// stream's own (pong, client_report): kind is its "type".
func (s *StreamService) OnControl(fn func(peerID, kind string, data []byte)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onControl = fn
}

// OnClosed is told when the stream drops a viewer by itself: its
// connection failed and could not be brought back (reason EndNetwork).
func (s *StreamService) OnClosed(fn func(peerID, reason string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onClosed = fn
}

// OnStats receives what is being sent, every two seconds while anyone
// watches (and once with zeros when nobody does any more).
func (s *StreamService) OnStats(fn func(models.StreamStatus)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onStats = fn
}

// SetBitrate changes the target bitrate; each encoder starts again with
// it on its next frame (with a keyframe).
func (s *StreamService) SetBitrate(kbps int) {
	if kbps > 0 {
		s.kbps.Store(int64(kbps))
	}
}

// Bitrate is the target bitrate in kbps.
func (s *StreamService) Bitrate() int { return int(s.kbps.Load()) }

// MediaSink receives raw media from a MediaSource.
type MediaSink interface {
	// VideoFrame takes a packed I420 frame of w x h shown for dur.
	VideoFrame(i420 []byte, w, h int, dur time.Duration)
	// Controls is what the test card shows about the stream.
	Controls() testpattern.Controls
	// Watching reports whether anyone receives the frames: a source can
	// rest while nobody does.
	Watching() bool
}

// MediaSource produces the picture: the test card, or the screen.
type MediaSource interface {
	Run(ctx context.Context, sink MediaSink) error
}

// SetSource picks what Run streams (default: the test card). Call it
// before Run.
func (s *StreamService) SetSource(src MediaSource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.source = src
}

// Run streams the source until ctx ends, and pings viewers and tells them
// what is sent every two seconds.
func (s *StreamService) Run(ctx context.Context) error {
	defer func() {
		for id, e := range s.encs {
			e.vp8.Close()
			delete(s.encs, id)
		}
	}()
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.pingAll()
				s.sendStreamStats()
			}
		}
	}()
	s.mu.Lock()
	src := s.source
	s.mu.Unlock()
	if src == nil {
		src = &TestCardSource{Width: s.cfg.Width, Height: s.cfg.Height, FPS: s.cfg.FPS}
	}
	s.window = time.Now()
	err := src.Run(ctx, s)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// Watching reports whether any viewer is connected. Like VideoFrame, it
// is called from the source's goroutine, which owns the encoders: the
// last viewer's encoder is closed here once nobody watches.
func (s *StreamService) Watching() bool {
	if s.ViewerCount() > 0 {
		return true
	}
	for id, e := range s.encs {
		e.vp8.Close()
		delete(s.encs, id)
	}
	return false
}

// Controls is what the test card shows: the frames per second sent to
// the first viewer.
func (s *StreamService) Controls() testpattern.Controls {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := testpattern.Controls{}
	for _, st := range s.stats {
		c.FPS = max(c.FPS, st.FPS)
	}
	return c
}

// VideoFrame encodes one frame for every viewer and sends it. Encoders
// follow the frame size; nothing is encoded while nobody watches.
func (s *StreamService) VideoFrame(i420 []byte, w, h int, dur time.Duration) {
	s.mu.Lock()
	viewers := slices.Collect(maps.Values(s.viewers))
	s.mu.Unlock()
	// Encoders of viewers that left are closed here, by their owner.
	for id, e := range s.encs {
		if !slices.ContainsFunc(viewers, func(v *viewer) bool { return v.id == id }) {
			e.vp8.Close()
			delete(s.encs, id)
		}
	}
	if elapsed := time.Since(s.window); elapsed >= time.Second {
		s.countWindow(elapsed)
	}
	kbps := s.Bitrate()
	for _, v := range viewers {
		e := s.encs[v.id]
		if e == nil || e.w != w || e.h != h || e.kbps != kbps {
			if e != nil {
				e.vp8.Close()
			}
			fps := max(int(time.Second/dur), 1)
			enc, err := encoder.NewVP8(encoder.Config{Width: w, Height: h, FPS: fps, BitrateKbps: kbps})
			if err != nil {
				s.log.Error("video encoder", "err", err)
				delete(s.encs, v.id)
				continue
			}
			resized := e == nil || e.w != w || e.h != h
			e = &viewerEnc{vp8: enc, w: w, h: h, kbps: kbps}
			s.encs[v.id] = e
			v.keyframe.Store(true)
			if resized {
				s.log.Info("video encoder", "peer_id", v.id, "codec", "vp8", "size", fmt.Sprintf("%dx%d", w, h), "kbps", kbps)
			}
		}
		data, _, err := e.vp8.Encode(i420, v.keyframe.Swap(false))
		if err != nil {
			s.log.Error("encode failed", "err", err)
			continue
		}
		if len(data) == 0 {
			continue // the rate control dropped this frame
		}
		if err := v.video.WriteSample(media.Sample{Data: data, Duration: dur}); err != nil && !errors.Is(err, errClosedPipe) {
			s.log.Debug("write sample", "err", err)
		}
		e.sent++
		e.bytes += len(data)
	}
}

var errClosedPipe = errors.New("io: read/write on closed pipe")

// countWindow closes a one-second window of each encoder's output.
func (s *StreamService) countWindow(elapsed time.Duration) {
	s.window = time.Now()
	stats := make(map[string]streamStats, len(s.encs))
	for id, e := range s.encs {
		e.fps = float64(e.sent) / elapsed.Seconds()
		e.kbpsOut = float64(e.bytes) * 8 / 1000 / elapsed.Seconds()
		e.sent, e.bytes = 0, 0
		stats[id] = streamStats{Type: "stream_stats", FPS: round1(e.fps), Width: e.w, Height: e.h, Kbps: round1(e.kbpsOut), Codec: "VP8"}
	}
	s.mu.Lock()
	s.stats = stats
	s.mu.Unlock()
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }

// streamStats is the stream_stats message: what the host sends this
// viewer, to compare with what its browser receives.
type streamStats struct {
	Type   string  `json:"type"`
	FPS    float64 `json:"fps"`
	Width  int     `json:"width"`
	Height int     `json:"height"`
	Kbps   float64 `json:"kbps"`
	Codec  string  `json:"codec"`
}

// Stats is what is being streamed now (zero while nobody watches).
func (s *StreamService) Stats() models.StreamStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := models.StreamStatus{VideoViewers: len(s.viewers)}
	for _, st := range s.stats {
		out.FPS = max(out.FPS, st.FPS)
		out.Width, out.Height = max(out.Width, st.Width), max(out.Height, st.Height)
		out.VideoKbps += st.Kbps
	}
	if out.VideoViewers == 0 {
		return models.StreamStatus{}
	}
	return out
}

// sendStreamStats tells each viewer what it is being sent, and the host's
// own listener what is sent overall.
func (s *StreamService) sendStreamStats() {
	type target struct {
		dc  *webrtc.DataChannel
		msg []byte
	}
	s.mu.Lock()
	var targets []target
	for id, v := range s.viewers {
		st, ok := s.stats[id]
		if ok && v.control != nil && v.control.ReadyState() == webrtc.DataChannelStateOpen {
			msg, _ := json.Marshal(st)
			targets = append(targets, target{v.control, msg})
		}
	}
	fn := s.onStats
	s.mu.Unlock()
	for _, t := range targets {
		s.sendText(t.dc, t.msg)
	}
	if fn != nil {
		fn(s.Stats())
	}
}

func (s *StreamService) iceServers() []webrtc.ICEServer {
	var out []webrtc.ICEServer
	for _, srv := range s.ice.Get() {
		out = append(out, webrtc.ICEServer{URLs: srv.URLs, Username: srv.Username, Credential: srv.Credential})
	}
	return out
}

func (s *StreamService) sendSignal(to string, sig RTCSignal) {
	s.mu.Lock()
	sender := s.sender
	s.mu.Unlock()
	if sender == nil {
		return
	}
	payload, err := json.Marshal(sig)
	if err != nil {
		return
	}
	if err := sender.Send(signalclient.Envelope{Type: signalclient.TypeSignal, To: to, Payload: payload}); err != nil {
		s.log.Warn("signal not sent", "to", to, "kind", sig.Kind, "err", err)
	}
}

// AddViewer opens a peer connection to an accepted viewer and sends it an
// offer: its own video track and the control channel.
func (s *StreamService) AddViewer(peerID string) error {
	pc, err := s.api.NewPeerConnection(webrtc.Configuration{ICEServers: s.iceServers()})
	if err != nil {
		return err
	}
	fail := func(err error) error {
		_ = pc.Close()
		return err
	}
	v := &viewer{id: peerID, pc: pc}
	video, err := newVideoTrack(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000})
	if err != nil {
		return fail(err)
	}
	v.video = video
	rtpSender, err := pc.AddTrack(video)
	if err != nil {
		return fail(err)
	}
	// Read RTCP: a Picture Loss Indication means this viewer cannot decode
	// and needs a keyframe (only this viewer's encoder makes one).
	go func() {
		buf := make([]byte, 1500)
		for {
			n, _, err := rtpSender.Read(buf)
			if err != nil {
				return
			}
			pkts, err := rtcp.Unmarshal(buf[:n])
			if err != nil {
				continue
			}
			for _, p := range pkts {
				switch p.(type) {
				case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
					v.keyframe.Store(true)
				}
			}
		}
	}()

	ordered := true
	control, err := pc.CreateDataChannel("control", &webrtc.DataChannelInit{Ordered: &ordered})
	if err != nil {
		return fail(err)
	}
	v.control = control
	control.OnOpen(func() {
		welcome, _ := json.Marshal(struct {
			Type   string      `json:"type"`
			Host   SessionMeta `json:"host"`
			Source string      `json:"source"`
		}{"welcome", s.cfg.Welcome, s.cfg.Source})
		s.sendText(control, welcome)
	})
	control.OnMessage(func(msg webrtc.DataChannelMessage) {
		if msg.IsString && len(msg.Data) <= maxControlMessage {
			s.handleControl(v, msg.Data)
		}
	})
	return s.offer(v)
}

func (s *StreamService) offer(v *viewer) error {
	pc, peerID := v.pc, v.id
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return // gathering finished
		}
		init := c.ToJSON()
		s.sendSignal(peerID, RTCSignal{Kind: "candidate", Candidate: &init})
		// The same port behind a forwarding router: announce it there too.
		if c.Typ == webrtc.ICECandidateTypeHost && s.cfg.UDPPort > 0 && int(c.Port) == s.cfg.UDPPort {
			for _, extra := range announcedCandidates(init, s.cfg.AnnounceIPs) {
				s.sendSignal(peerID, RTCSignal{Kind: "candidate", Candidate: &extra})
			}
		}
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		s.log.Info("viewer connection", "peer_id", peerID, "state", state.String())
		switch state {
		case webrtc.PeerConnectionStateConnected:
			v.mu.Lock()
			v.restarts = 0
			if v.restart != nil {
				v.restart.Stop()
				v.restart = nil
			}
			v.mu.Unlock()
			v.keyframe.Store(true)
			go s.logPath(peerID, pc)
		case webrtc.PeerConnectionStateDisconnected:
			// Often back on its own in a moment; if not, restart ICE.
			v.mu.Lock()
			if v.restart == nil && !v.closed {
				v.restart = time.AfterFunc(restartAfter, func() { s.restartICE(v) })
			}
			v.mu.Unlock()
		case webrtc.PeerConnectionStateFailed:
			go s.restartICE(v) // not from inside Pion's callback
		}
	})

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		_ = pc.Close()
		return err
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		_ = pc.Close()
		return err
	}
	s.mu.Lock()
	old := s.viewers[peerID]
	s.viewers[peerID] = v
	s.mu.Unlock()
	if old != nil {
		old.close()
	}
	v.keyframe.Store(true)
	s.sendSignal(peerID, RTCSignal{Kind: "offer", SDP: offer.SDP})
	return nil
}

// restartICE offers again with new ICE credentials, so a session survives
// a network change (Wi-Fi to cable, a new address). After maxRestarts
// failed tries in a row the viewer is dropped and the session ends.
func (s *StreamService) restartICE(v *viewer) {
	v.mu.Lock()
	v.restart = nil
	if v.closed || v.pc.ConnectionState() == webrtc.PeerConnectionStateConnected {
		v.mu.Unlock()
		return
	}
	v.restarts++
	tries := v.restarts
	if tries > maxRestarts {
		v.mu.Unlock()
		s.log.Warn("viewer connection lost", "peer_id", v.id, "restarts", maxRestarts)
		s.drop(v.id, models.EndNetwork)
		return
	}
	offer, err := v.pc.CreateOffer(&webrtc.OfferOptions{ICERestart: true})
	if err == nil {
		err = v.pc.SetLocalDescription(offer)
	}
	if err != nil {
		v.mu.Unlock()
		s.log.Warn("ICE restart failed", "peer_id", v.id, "err", err)
		s.drop(v.id, models.EndNetwork)
		return
	}
	// Candidates for the new credentials wait for the new answer.
	v.remoteSet = false
	v.pending = nil
	// A restart that gets nowhere is tried again.
	v.restart = time.AfterFunc(4*restartAfter, func() { s.restartICE(v) })
	v.mu.Unlock()
	s.log.Info("ICE restart", "peer_id", v.id, "try", tries)
	s.sendSignal(v.id, RTCSignal{Kind: "offer", SDP: offer.SDP})
}

// drop removes a viewer the stream gave up on and says so.
func (s *StreamService) drop(peerID, reason string) {
	s.mu.Lock()
	_, ok := s.viewers[peerID]
	fn := s.onClosed
	s.mu.Unlock()
	if !ok {
		return
	}
	s.RemoveViewer(peerID)
	if fn != nil {
		fn(peerID, reason)
	}
}

// close ends a viewer's connection; it is safe to call more than once.
func (v *viewer) close() {
	v.mu.Lock()
	if v.closed {
		v.mu.Unlock()
		return
	}
	v.closed = true
	if v.restart != nil {
		v.restart.Stop()
		v.restart = nil
	}
	v.mu.Unlock()
	_ = v.pc.Close()
}

// HandleSignal applies an answer or ICE candidate from a viewer.
func (s *StreamService) HandleSignal(from string, payload json.RawMessage) error {
	s.mu.Lock()
	v := s.viewers[from]
	s.mu.Unlock()
	if v == nil {
		return errors.New("stream: signal from unknown viewer")
	}
	var sig RTCSignal
	if err := json.Unmarshal(payload, &sig); err != nil {
		return err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	switch sig.Kind {
	case "answer":
		if err := v.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: sig.SDP}); err != nil {
			return err
		}
		v.remoteSet = true
		for _, c := range v.pending {
			_ = v.pc.AddICECandidate(c)
		}
		v.pending = nil
	case "candidate":
		if sig.Candidate == nil {
			return nil
		}
		if !v.remoteSet {
			if len(v.pending) < maxPendingCandidates {
				v.pending = append(v.pending, *sig.Candidate) // arrived before the answer
			}
			return nil
		}
		return v.pc.AddICECandidate(*sig.Candidate)
	default:
		return errors.New("stream: unknown signal kind " + sig.Kind)
	}
	return nil
}

// controlMessage is the part of a control message the stream reads. The
// id stays raw: a pong carries a number, other messages may carry text.
type controlMessage struct {
	Type string          `json:"type"`
	ID   json.RawMessage `json:"id,omitempty"`
}

func (s *StreamService) handleControl(v *viewer, data []byte) {
	var msg controlMessage
	if json.Unmarshal(data, &msg) != nil || msg.Type == "" {
		return
	}
	switch msg.Type {
	case "pong":
		var id uint64
		if json.Unmarshal(msg.ID, &id) != nil {
			return
		}
		s.mu.Lock()
		sent, ok := s.pings[id]
		delete(s.pings, id)
		if ok {
			s.rtt[v.id] = int(time.Since(sent).Milliseconds())
		}
		s.mu.Unlock()
	case "client_report":
		s.noteClientReport(v, data)
	default:
		s.mu.Lock()
		fn := s.onControl
		s.mu.Unlock()
		if fn != nil {
			fn(v.id, msg.Type, data)
		}
	}
}

// noteClientReport moves the viewer's playout delay from what its browser
// measured (playout_delay.go): a smooth picture over the lowest latency
// only while the network needs it.
func (s *StreamService) noteClientReport(v *viewer, data []byte) {
	var rep struct {
		Metrics map[string]float64 `json:"metrics"`
	}
	if json.Unmarshal(data, &rep) != nil || rep.Metrics == nil {
		return
	}
	v.mu.Lock()
	changed, why := v.playout.note(rep.Metrics)
	ms := v.playout.maxMs()
	v.mu.Unlock()
	if !changed {
		return
	}
	if i := playoutDelays.forPC(v.pc); i != nil {
		i.SetMax(ms)
	}
	s.log.Info("playout delay", "peer_id", v.id, "max_ms", ms, "why", why)
}

// RTT is the last control-channel round trip to a viewer, in ms (-1:
// not measured yet).
func (s *StreamService) RTT(peerID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ms, ok := s.rtt[peerID]; ok {
		return ms
	}
	return -1
}

// pingAll sends a ping to every open control channel. The browser echoes
// it, which measures the round trip over the peer-to-peer path.
func (s *StreamService) pingAll() {
	s.mu.Lock()
	now := time.Now()
	for id, t := range s.pings { // forget pings that never came back
		if now.Sub(t) > 10*time.Second {
			delete(s.pings, id)
		}
	}
	var targets []*webrtc.DataChannel
	var ids []uint64
	for _, v := range s.viewers {
		if v.control != nil && v.control.ReadyState() == webrtc.DataChannelStateOpen {
			s.pingSeq++
			s.pings[s.pingSeq] = now
			targets = append(targets, v.control)
			ids = append(ids, s.pingSeq)
		}
	}
	s.mu.Unlock()
	for i, dc := range targets {
		b, _ := json.Marshal(struct {
			Type string `json:"type"`
			ID   uint64 `json:"id"`
		}{"ping", ids[i]})
		s.sendText(dc, b)
	}
}

// SendControl sends a JSON message on one viewer's control channel.
func (s *StreamService) SendControl(peerID string, msg []byte) bool {
	s.mu.Lock()
	v := s.viewers[peerID]
	s.mu.Unlock()
	if v == nil || v.control == nil || v.control.ReadyState() != webrtc.DataChannelStateOpen {
		return false
	}
	return s.sendText(v.control, msg)
}

// sendText sends a control message and logs, at most once a minute, the
// ones that do not go out: one larger than the browser takes (Chrome: 256
// KiB a message) is refused whole, and the browser never knows.
func (s *StreamService) sendText(dc *webrtc.DataChannel, msg []byte) bool {
	err := dc.SendText(string(msg))
	if err == nil {
		return true
	}
	now := time.Now().UnixNano()
	if last := s.sendErrAt.Load(); now-last > int64(time.Minute) && s.sendErrAt.CompareAndSwap(last, now) {
		var head struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(msg, &head)
		s.log.Warn("control message not sent", "type", head.Type, "bytes", len(msg), "err", err)
	}
	return false
}

// Hangup sends a last control message (session_end) and closes the
// viewer's connection once it is out, or after a short wait.
func (s *StreamService) Hangup(peerID string, last []byte) {
	s.mu.Lock()
	v := s.viewers[peerID]
	s.mu.Unlock()
	if v == nil {
		return
	}
	if last != nil && s.SendControl(peerID, last) {
		for end := time.Now().Add(500 * time.Millisecond); time.Now().Before(end) && v.control.BufferedAmount() > 0; {
			time.Sleep(10 * time.Millisecond)
		}
		time.Sleep(50 * time.Millisecond) // the bytes left the buffer, not yet the wire
	}
	s.RemoveViewer(peerID)
}

// RemoveViewer closes the peer connection of peerID, if any.
func (s *StreamService) RemoveViewer(peerID string) {
	s.mu.Lock()
	v := s.viewers[peerID]
	delete(s.viewers, peerID)
	delete(s.stats, peerID)
	delete(s.rtt, peerID)
	empty := len(s.viewers) == 0
	fn := s.onStats
	s.mu.Unlock()
	if v == nil {
		return
	}
	v.close()
	if empty && fn != nil {
		fn(models.StreamStatus{})
	}
}

// CloseAll drops every viewer (the signaling link was lost).
func (s *StreamService) CloseAll() {
	s.mu.Lock()
	ids := slices.Collect(maps.Keys(s.viewers))
	s.mu.Unlock()
	for _, id := range ids {
		s.RemoveViewer(id)
	}
}

// ViewerCount returns how many peer connections exist.
func (s *StreamService) ViewerCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.viewers)
}

// announcedCandidates copies a host candidate for each announced address:
// same port and transport, another address and foundation, and a slightly
// lower priority than the real host candidate.
func announcedCandidates(host webrtc.ICECandidateInit, ips []string) []webrtc.ICECandidateInit {
	fields := strings.Fields(host.Candidate)
	// candidate:<foundation> <component> <transport> <priority> <address> <port> typ host ...
	if len(fields) < 8 || !strings.HasPrefix(fields[0], "candidate:") || fields[6] != "typ" {
		return nil
	}
	prio, err := strconv.ParseUint(fields[3], 10, 32)
	if err != nil {
		return nil
	}
	var out []webrtc.ICECandidateInit
	for i, ip := range ips {
		if net.ParseIP(ip) == nil || ip == fields[4] {
			continue
		}
		f := slices.Clone(fields)
		f[0] = fmt.Sprintf("candidate:%d", 4000000000+i)
		f[3] = strconv.FormatUint(prio-uint64(i+1)*256, 10)
		f[4] = ip
		c := host
		c.Candidate = strings.Join(f, " ")
		out = append(out, c)
	}
	return out
}

// logPath records how a viewer reached the host: directly (host, srflx,
// prflx) or through the TURN relay, which adds the round trip to the
// relay server.
func (s *StreamService) logPath(peerID string, pc *webrtc.PeerConnection) {
	sctp := pc.SCTP()
	if sctp == nil {
		return
	}
	pair, err := sctp.Transport().ICETransport().GetSelectedCandidatePair()
	if err != nil || pair == nil {
		return
	}
	path := "direct"
	if pair.Local.Typ == webrtc.ICECandidateTypeRelay || pair.Remote.Typ == webrtc.ICECandidateTypeRelay {
		path = "relay"
	}
	s.log.Info("viewer path", "peer_id", peerID, "path", path,
		"local", fmt.Sprintf("%s %s:%d", pair.Local.Typ, pair.Local.Address, pair.Local.Port),
		"remote", fmt.Sprintf("%s %s:%d", pair.Remote.Typ, pair.Remote.Address, pair.Remote.Port))
}
