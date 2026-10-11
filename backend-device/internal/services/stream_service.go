// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"encoding/base64"
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

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	screencap "github.com/lordbasex/go-link-desk/backend-device/pkg/capture"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/cursor"
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
	// H.264 Constrained High, what the system encoder sends (video.go).
	feedback := []webrtc.RTCPFeedback{{Type: "goog-remb"}, {Type: "ccm", Parameter: "fir"}, {Type: "nack"}, {Type: "nack", Parameter: "pli"}}
	if err := me.RegisterCodec(webrtc.RTPCodecParameters{RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000, SDPFmtpLine: h264HighFmtp, RTCPFeedback: feedback}, PayloadType: 114}, webrtc.RTPCodecTypeVideo); err != nil {
		return nil, err
	}
	if err := me.RegisterCodec(webrtc.RTPCodecParameters{RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeRTX, ClockRate: 90000, SDPFmtpLine: "apt=114"}, PayloadType: 115}, webrtc.RTPCodecTypeVideo); err != nil {
		return nil, err
	}
	if err := me.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{URI: playoutDelayURI}, webrtc.RTPCodecTypeVideo); err != nil {
		return nil, err
	}
	// Interceptors in the order they sit from the network out (the first
	// one added is the closest to it).
	reg := &interceptor.Registry{}
	// The simulated loss (sim_loss.go; tests only), closest to the network.
	if pct := simLossFromEnv(); pct > 0 {
		reg.Add(&simLossFactory{pct: pct})
	}
	// Pion's defaults: retransmissions (NACK), reports, statistics.
	if err := webrtc.RegisterDefaultInterceptors(me, reg); err != nil {
		return nil, err
	}
	// The bandwidth estimate (congestion.go), farther from the network
	// than the retransmissions: those go out on their own SSRC (RTX),
	// which the estimator's writer does not know and would drop.
	bwe, err := newCongestionControl()
	if err != nil {
		return nil, err
	}
	reg.Add(bwe)
	// Transport-wide sequence numbers, stamped before the estimator sees
	// each packet; the browser's feedback on them feeds the estimate.
	if err := webrtc.ConfigureTWCCHeaderExtensionSender(me, reg); err != nil {
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
	audio   *webrtc.TrackLocalStaticSample
	// hears: the viewer may hear the computer's sound (Permissions.Audio).
	hears atomic.Bool
	codec string        // CodecH264High, CodecH264 or CodecVP8
	out   *sampleWriter // its track, with the frames and bytes sent
	// keyframe asks this viewer's encoder for a keyframe: a new viewer, a
	// Picture Loss Indication from its browser, or a connection back.
	keyframe atomic.Bool
	// kbps is the bitrate its encoder should use: the preset's ceiling,
	// or less when the bandwidth estimate says so.
	kbps    atomic.Int64
	playout playoutLadder // under mu

	mu        sync.Mutex
	remoteSet bool
	pending   []webrtc.ICECandidateInit
	restarts  int         // ICE restarts since the connection was last up
	restart   *time.Timer // a pending restart while disconnected
	closed    bool
	cursors   map[string]bool // pointer images this viewer already has
}

// StreamService sends the session's video to its viewer over WebRTC, with
// a "control" DataChannel (reliable, ordered, JSON) for everything else.
// Each viewer has its own track and encoder (H.264 from the system's
// encoder, or VP8), so a keyframe one viewer needs never costs the others
// anything. Frames only come when the picture changes.
type StreamService struct {
	cfg StreamConfig
	log *slog.Logger
	ice *ICEStore
	api *webrtc.API
	// sendErrAt is when a control message that could not be sent was last
	// logged (unix nanoseconds); see sendText.
	sendErrAt atomic.Int64

	// Owned by the source goroutine (Run): the encoders, and the bitrate
	// each one was last given.
	encs     map[string]videoEncoder
	encKbps  map[string]int
	encRetry map[string]time.Time // an encoder that failed is tried again then
	sound    audioPipe

	mu        sync.Mutex
	sender    Sender
	viewers   map[string]*viewer
	source    MediaSource
	pings     map[uint64]time.Time
	pingSeq   uint64
	rtt       map[string]int  // the last round trip per viewer, ms
	hearing   map[string]bool // the viewers that may hear the sound
	stats     map[string]streamStats
	sizes     map[string][2]int // each viewer's picture size
	want      VideoWant         // the latest viewer's wishes
	display   uint32            // the display chosen (0: the main one)
	displays  []screencap.Display
	beacon    uint16 // the latency beacon's state, flipped by the viewer
	rung      int    // the bandwidth ladder's rung (len(bandwidthLadder): no limit)
	changed   chan struct{}
	onControl func(peerID, kind string, data []byte)
	onClosed  func(peerID, reason string)
	onStats   func(models.StreamStatus)
	onBeacon  func(on bool, d screencap.Display)
	onInput   func(peerID, channel string, data []byte)
	onStall   func(peerID string)
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
	return &StreamService{
		cfg:      cfg,
		log:      cfg.Logger,
		ice:      ice,
		api:      api,
		encs:     make(map[string]videoEncoder),
		encKbps:  make(map[string]int),
		encRetry: make(map[string]time.Time),
		viewers:  make(map[string]*viewer),
		pings:    make(map[uint64]time.Time),
		rtt:      make(map[string]int),
		hearing:  make(map[string]bool),
		stats:    make(map[string]streamStats),
		sizes:    make(map[string][2]int),
		want:     VideoWant{Preset: PresetBalanced},
		changed:  make(chan struct{}, 1),
	}, nil
}

// SetSender wires the signaling client.
func (s *StreamService) SetSender(sender Sender) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sender = sender
}

// OnControl receives every control message from a viewer other than the
// stream's own: kind is its "type".
func (s *StreamService) OnControl(fn func(peerID, kind string, data []byte)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onControl = fn
}

// OnInput receives a viewer's input packets: channel is "pointer" or
// "input".
func (s *StreamService) OnInput(fn func(peerID, channel string, data []byte)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onInput = fn
}

func (s *StreamService) input(peerID, channel string, data []byte) {
	s.mu.Lock()
	fn := s.onInput
	s.mu.Unlock()
	if fn != nil {
		fn(peerID, channel, data)
	}
}

// OnStall is told when a viewer's connection stops answering (the input
// it holds must be let go: it may never send the releases).
func (s *StreamService) OnStall(fn func(peerID string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onStall = fn
}

// Displays lists the host's displays in the order the viewers know them.
func (s *StreamService) Displays() []screencap.Display {
	s.mu.Lock()
	ds := s.displays
	s.mu.Unlock()
	if len(ds) == 0 {
		ds = s.refreshDisplays()
	}
	return ds
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

// MediaSink receives the picture from a MediaSource. Its methods are
// called from the source's goroutine.
type MediaSink interface {
	// VideoFrame encodes one frame for every viewer.
	VideoFrame(f Frame)
	// Controls is what the test card shows about the stream.
	Controls() testpattern.Controls
	// Watching reports whether anyone receives the frames: a source rests
	// (and the screen capture stops) while nobody does.
	Watching() bool
	// Capture is what a screen source should capture now; Changed is
	// signalled when that changes.
	Capture() CaptureRequest
	Changed() <-chan struct{}
	// NeedsKeyframe reports whether a viewer waits for a keyframe, which a
	// still screen would never send: the source sends its last frame again.
	NeedsKeyframe() bool
	// Notice tells the viewers something about the source (no permission
	// to record the screen, back to the test card).
	Notice(code string)
	// AudioSamples sends the computer's sound to the viewers allowed to
	// hear it: interleaved 16-bit stereo at 48 kHz, any length. It may be
	// called from its own goroutine.
	AudioSamples(samples []int16)
	// Hearing reports whether any viewer may hear the sound.
	Hearing() bool
}

// CaptureRequest is the screen capture the viewers need.
type CaptureRequest struct {
	Display       screencap.Display
	Width, Height int
	FPS           int
	ShowCursor    bool
	Audio         bool // a viewer may hear the sound: capture it
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

// Run streams the source until ctx ends; every half second it follows
// each viewer's bandwidth estimate, and every two seconds it pings the
// viewers and tells them what is sent.
func (s *StreamService) Run(ctx context.Context) error {
	defer s.closeEncoders(nil)
	go func() {
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		for n := 1; ; n++ {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.followBandwidth()
				if n%4 == 0 {
					s.pingAll()
					s.sendStreamStats(2 * time.Second)
				}
			}
		}
	}()
	s.mu.Lock()
	src := s.source
	s.mu.Unlock()
	if src == nil {
		src = &TestCardSource{Width: s.cfg.Width, Height: s.cfg.Height, FPS: s.cfg.FPS}
	}
	err := src.Run(ctx, s)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// closeEncoders closes the encoders of viewers not in keep (all, for nil).
// Only the source's goroutine calls it.
func (s *StreamService) closeEncoders(keep map[string]*viewer) {
	for id, e := range s.encs {
		if _, ok := keep[id]; !ok {
			e.close()
			delete(s.encs, id)
			delete(s.encKbps, id)
			delete(s.encRetry, id)
		}
	}
}

// Watching reports whether any viewer is connected. Like VideoFrame, it
// is called from the source's goroutine, which owns the encoders: the
// last viewer's encoder is closed here once nobody watches.
func (s *StreamService) Watching() bool {
	if s.ViewerCount() > 0 {
		return true
	}
	s.closeEncoders(nil)
	return false
}

// Controls is what the test card shows: the frames per second sent, and
// the latency beacon.
func (s *StreamService) Controls() testpattern.Controls {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := testpattern.Controls{Beacon: s.beacon%2 == 1}
	for _, st := range s.stats {
		c.FPS = max(c.FPS, st.FPS)
	}
	return c
}

// Changed is signalled when the capture the viewers need changes.
func (s *StreamService) Changed() <-chan struct{} { return s.changed }

func (s *StreamService) signalChanged() {
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

// NeedsKeyframe reports whether a viewer waits for a keyframe.
func (s *StreamService) NeedsKeyframe() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.viewers {
		if v.keyframe.Load() {
			return true
		}
	}
	return false
}

// Capture is the screen capture the viewers need: the chosen display,
// sized for the viewer's window and preset, at the preset's frame rate,
// both lowered when the bandwidth is short (bandwidthLadder).
func (s *StreamService) Capture() CaptureRequest {
	s.mu.Lock()
	want, id, rung := s.want, s.display, s.rung
	displays := s.displays
	s.mu.Unlock()
	if len(displays) == 0 {
		displays = s.refreshDisplays()
	}
	d := pickDisplay(displays, id)
	w, h := captureSize(d, want)
	fps := presetLimits[ParsePreset(string(want.Preset))].fps
	if rung < len(bandwidthLadder) {
		r := bandwidthLadder[rung]
		fps = min(fps, r.fps)
		if r.maxWidth > 0 && w > r.maxWidth {
			h = (r.maxWidth * h / w) &^ 1
			w = r.maxWidth
		}
	}
	return CaptureRequest{Display: d, Width: w, Height: h, FPS: fps, ShowCursor: want.CursorInVideo, Audio: s.Hearing()}
}

// AudioSamples encodes the sound once and sends it to every viewer that
// may hear it.
func (s *StreamService) AudioSamples(samples []int16) {
	s.sound.write(samples, s.listeners)
}

// Hearing reports whether any viewer may hear the sound.
func (s *StreamService) Hearing() bool { return len(s.listeners()) > 0 }

// listeners are the audio tracks of the viewers that may hear the sound.
func (s *StreamService) listeners() []*webrtc.TrackLocalStaticSample {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*webrtc.TrackLocalStaticSample
	for _, v := range s.viewers {
		if v.hears.Load() && v.audio != nil {
			out = append(out, v.audio)
		}
	}
	return out
}

// SetAudio lets a viewer hear the computer's sound, or stops it. It may
// come before the viewer's stream starts (the session's first grant).
func (s *StreamService) SetAudio(peerID string, on bool) {
	s.mu.Lock()
	if on {
		s.hearing[peerID] = true
	} else {
		delete(s.hearing, peerID)
	}
	v := s.viewers[peerID]
	s.mu.Unlock()
	if v == nil || v.hears.Swap(on) == on {
		return
	}
	s.signalChanged() // the capture starts or stops the sound
}

// pickDisplay finds a display by id, else the main one.
func pickDisplay(ds []screencap.Display, id uint32) screencap.Display {
	for _, d := range ds {
		if d.ID == id {
			return d
		}
	}
	for _, d := range ds {
		if d.Primary {
			return d
		}
	}
	if len(ds) > 0 {
		return ds[0]
	}
	return screencap.Display{Width: 1920, Height: 1080, PointsWidth: 1920, PointsHeight: 1080}
}

// refreshDisplays lists the screens again (none where there is no
// capture).
func (s *StreamService) refreshDisplays() []screencap.Display {
	if !screencap.Supported || s.cfg.Source != "screen" {
		return nil
	}
	ds, err := screencap.Displays()
	if err != nil {
		s.log.Warn("cannot list the displays", "err", err)
		return nil
	}
	s.mu.Lock()
	s.displays = ds
	s.mu.Unlock()
	return ds
}

// Notice tells every viewer something about the source.
func (s *StreamService) Notice(code string) {
	b, _ := json.Marshal(struct {
		Type string `json:"type"`
		Code string `json:"code"`
	}{"notice", code})
	s.mu.Lock()
	ids := slices.Collect(maps.Keys(s.viewers))
	s.mu.Unlock()
	for _, id := range ids {
		s.SendControl(id, b)
	}
}

// VideoFrame encodes one frame for every viewer and sends it. Encoders
// follow the frame size; nothing is encoded while nobody watches.
func (s *StreamService) VideoFrame(f Frame) {
	s.mu.Lock()
	viewers := maps.Clone(s.viewers)
	s.mu.Unlock()
	s.closeEncoders(viewers) // of viewers that left
	for _, v := range viewers {
		e := s.encs[v.id]
		if e != nil {
			if w, h := e.size(); w != f.Width || h != f.Height {
				e.close()
				e = nil
			}
		}
		if e == nil {
			if time.Now().Before(s.encRetry[v.id]) {
				continue // it failed a moment ago: not on every frame
			}
			var err error
			if e, err = s.newEncoder(v, f.Width, f.Height); err != nil {
				s.log.Error("video encoder", "peer_id", v.id, "codec", v.codec, "size", fmt.Sprintf("%dx%d", f.Width, f.Height), "err", err)
				delete(s.encs, v.id)
				s.encRetry[v.id] = time.Now().Add(2 * time.Second)
				continue
			}
			delete(s.encRetry, v.id)
			s.encs[v.id] = e
			s.encKbps[v.id] = int(v.kbps.Load())
			s.mu.Lock()
			s.sizes[v.id] = [2]int{f.Width, f.Height}
			s.mu.Unlock()
			v.keyframe.Store(true)
		}
		if kbps := int(v.kbps.Load()); kbps != s.encKbps[v.id] {
			e.setBitrate(kbps)
			s.encKbps[v.id] = kbps
		}
		if err := e.encode(f, v.keyframe.Swap(false)); err != nil {
			s.log.Debug("encode failed", "peer_id", v.id, "err", err)
		}
	}
}

// newEncoder starts a viewer's encoder for its codec.
func (s *StreamService) newEncoder(v *viewer, w, h int) (videoEncoder, error) {
	fps := presetLimits[ParsePreset(string(s.wantNow().Preset))].fps
	kbps := int(v.kbps.Load())
	var e videoEncoder
	var err error
	if v.codec == CodecVP8 {
		e, err = newVP8Encoder(w, h, fps, kbps, v.out)
	} else {
		var h264 *h264Encoder
		h264, err = newH264Encoder(w, h, fps, kbps, v.codec == CodecH264, v.out)
		if err == nil {
			hw, known := h264.enc.Hardware()
			if known && !hw {
				s.log.Warn("H.264 runs on the software encoder: this computer has no hardware one", "peer_id", v.id)
			}
		}
		e = h264
	}
	if err != nil {
		return nil, err
	}
	s.log.Info("video encoder", "peer_id", v.id, "codec", v.codec, "size", fmt.Sprintf("%dx%d", w, h), "kbps", kbps)
	return e, nil
}

func (s *StreamService) wantNow() VideoWant {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.want
}

// followBandwidth sets each viewer's bitrate: the preset's ceiling, or 90%
// of its connection's estimate when that is lower; and moves the session
// on the bandwidth ladder by the worst estimate, which changes the
// capture.
func (s *StreamService) followBandwidth() {
	s.mu.Lock()
	ceiling := presetLimits[ParsePreset(string(s.want.Preset))].kbps
	worst := 0
	for _, v := range s.viewers {
		kbps := ceiling
		if est := estimateKbps(v.pc); est > 0 {
			kbps = max(min(kbps, est*9/10), minKbps)
			if worst == 0 || est < worst {
				worst = est
			}
		}
		v.kbps.Store(int64(kbps)) // the encoder takes it with the next frame
	}
	if len(s.viewers) == 0 {
		s.rung = len(bandwidthLadder)
	}
	before := s.rung
	s.rung = ladderRung(worst, s.rung)
	after := s.rung
	s.mu.Unlock()
	if after != before {
		s.log.Info("bandwidth", "estimate_kbps", worst, "rung", after)
		s.signalChanged()
	}
}

var errClosedPipe = errors.New("io: read/write on closed pipe")

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
	// TargetKbps is the bitrate the encoder aims at: the preset's
	// ceiling, or the bandwidth estimate when lower.
	TargetKbps int `json:"target_kbps"`
	// EstimateKbps is what the congestion control thinks the path carries
	// (0: no estimate yet).
	EstimateKbps int `json:"estimate_kbps"`
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

// sendStreamStats closes a window of what each viewer was sent, tells each
// viewer, and the host's own listener what is sent overall. A still
// screen sends nothing, and says so.
func (s *StreamService) sendStreamStats(window time.Duration) {
	type target struct {
		dc  *webrtc.DataChannel
		msg []byte
	}
	s.mu.Lock()
	var targets []target
	stats := make(map[string]streamStats, len(s.viewers))
	for id, v := range s.viewers {
		frames, bytes := v.out.counts()
		st := streamStats{
			Type: "stream_stats", FPS: round1(float64(frames) / window.Seconds()),
			Kbps: round1(float64(bytes) * 8 / 1000 / window.Seconds()), Codec: codecLabel(v.codec),
			TargetKbps: int(v.kbps.Load()), EstimateKbps: estimateKbps(v.pc),
		}
		st.Width, st.Height = s.sizes[id][0], s.sizes[id][1]
		stats[id] = st
		if v.control != nil && v.control.ReadyState() == webrtc.DataChannelStateOpen {
			msg, _ := json.Marshal(st)
			targets = append(targets, target{v.control, msg})
		}
	}
	s.stats = stats
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
// offer: its own video track and the control channel. codecs are the
// video codecs its browser said it takes (the knock); the session uses
// H.264 from the system's encoder when it can, else VP8.
func (s *StreamService) AddViewer(peerID string, codecs []string) error {
	pc, err := s.api.NewPeerConnection(webrtc.Configuration{ICEServers: s.iceServers()})
	if err != nil {
		return err
	}
	fail := func(err error) error {
		_ = pc.Close()
		return err
	}
	v := &viewer{id: peerID, pc: pc, codec: pickCodec(codecs, encoder.HasH264)}
	v.kbps.Store(int64(presetLimits[ParsePreset(string(s.wantNow().Preset))].kbps))
	video, err := newVideoTrack(trackCodec(v.codec))
	if err != nil {
		return fail(err)
	}
	v.video = video
	v.out = &sampleWriter{track: video}
	rtpSender, err := pc.AddTrack(video)
	if err != nil {
		return fail(err)
	}
	// The sound: always in the offer, so allowing it later needs no new
	// negotiation; nothing is sent on it until the viewer may hear.
	audio, err := newAudioTrack()
	if err != nil {
		return fail(err)
	}
	v.audio = audio
	audioSender, err := pc.AddTrack(audio)
	if err != nil {
		return fail(err)
	}
	go func() { // read its RTCP so the interceptors get it
		buf := make([]byte, 1500)
		for {
			if _, _, err := audioSender.Read(buf); err != nil {
				return
			}
		}
	}()
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
		s.sendDisplays(v)
	})
	control.OnMessage(func(msg webrtc.DataChannelMessage) {
		if msg.IsString && len(msg.Data) <= maxControlMessage {
			s.handleControl(v, msg.Data)
		}
	})
	// Input: pointer moves where a lost one does not matter (unordered,
	// never resent), and buttons, keys and the rest where every one does
	// (ordered, reliable). A lost key release would leave a key stuck.
	unordered, noRetransmits := false, uint16(0)
	pointer, err := pc.CreateDataChannel("pointer", &webrtc.DataChannelInit{Ordered: &unordered, MaxRetransmits: &noRetransmits})
	if err != nil {
		return fail(err)
	}
	pointer.OnMessage(func(msg webrtc.DataChannelMessage) {
		if !msg.IsString {
			s.input(v.id, "pointer", msg.Data)
		}
	})
	inputDC, err := pc.CreateDataChannel("input", &webrtc.DataChannelInit{Ordered: &ordered})
	if err != nil {
		return fail(err)
	}
	inputDC.OnMessage(func(msg webrtc.DataChannelMessage) {
		if !msg.IsString && len(msg.Data) <= maxControlMessage {
			s.input(v.id, "input", msg.Data)
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
			s.mu.Lock()
			stall := s.onStall
			s.mu.Unlock()
			if stall != nil {
				go stall(peerID)
			}
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
	v.hears.Store(s.hearing[peerID])
	s.mu.Unlock()
	if v.hears.Load() {
		s.signalChanged()
	}
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
	case "video_want", "quality", "display_select", "cursor_mode", "beacon":
		s.handleVideoControl(v, msg.Type, data)
	default:
		s.mu.Lock()
		fn := s.onControl
		s.mu.Unlock()
		if fn != nil {
			fn(v.id, msg.Type, data)
		}
	}
}

// handleVideoControl applies a viewer's wishes about the picture: the size
// it shows it at, the preset and Retina, the display, where the pointer is
// drawn, and the latency beacon.
func (s *StreamService) handleVideoControl(v *viewer, kind string, data []byte) {
	var m struct {
		Width   int    `json:"width"`
		Height  int    `json:"height"`
		Preset  string `json:"preset"`
		Retina  bool   `json:"retina"`
		ID      uint32 `json:"id"`
		InVideo bool   `json:"in_video"`
		Seq     uint16 `json:"seq"`
	}
	if json.Unmarshal(data, &m) != nil {
		return
	}
	s.mu.Lock()
	before, display := s.want, s.display
	switch kind {
	case "video_want":
		// The size the viewer shows the picture at, in device pixels:
		// bounded, so a viewer cannot ask for absurd sizes.
		s.want.Width, s.want.Height = min(max(m.Width, 0), 16384), min(max(m.Height, 0), 16384)
	case "quality":
		s.want.Preset, s.want.Retina = ParsePreset(m.Preset), m.Retina
	case "cursor_mode":
		s.want.CursorInVideo = m.InVideo
	case "display_select":
		s.display = m.ID
	case "beacon":
		s.beacon = m.Seq
	}
	changed := s.want != before || s.display != display
	presetChanged := s.want.Preset != before.Preset
	s.mu.Unlock()
	if kind == "beacon" {
		s.mu.Lock()
		fn := s.onBeacon
		s.mu.Unlock()
		if fn != nil {
			fn(m.Seq%2 == 1, s.Capture().Display)
		}
		return
	}
	if presetChanged {
		s.followBandwidth()
	}
	if kind == "display_select" && s.display != display {
		for _, viewer := range s.viewerList() {
			viewer.keyframe.Store(true)
			s.sendDisplays(viewer)
		}
	}
	if changed {
		s.log.Info("video wish", "peer_id", v.id, "kind", kind, "want", fmt.Sprintf("%+v", s.wantNow()))
		s.signalChanged()
	}
}

// CursorMoved tells the viewers where the host's pointer is, normalized
// over the display they see, unless they see it drawn in the picture.
func (s *StreamService) CursorMoved(x, y float64, visible bool) {
	if s.wantNow().CursorInVideo {
		return
	}
	b, _ := json.Marshal(struct {
		Type    string  `json:"type"`
		X       float64 `json:"x"`
		Y       float64 `json:"y"`
		Visible bool    `json:"visible"`
	}{"cursor_pos", x, y, visible})
	for _, v := range s.viewerList() {
		s.SendControl(v.id, b)
	}
}

// CursorShape tells the viewers what the pointer looks like. Each viewer
// gets an image once; after that its id is enough.
func (s *StreamService) CursorShape(c cursor.Shape) {
	type msg struct {
		Type   string `json:"type"`
		ID     string `json:"id"`
		PNG    string `json:"png,omitempty"` // base64
		HotX   int    `json:"hot_x"`
		HotY   int    `json:"hot_y"`
		Width  int    `json:"width"` // in points: the size it shows at
		Height int    `json:"height"`
		Pixels int    `json:"pixels"` // the image's width in pixels
	}
	full, _ := json.Marshal(msg{"cursor", c.ID, base64.StdEncoding.EncodeToString(c.PNG), c.HotX, c.HotY, c.PointsWidth, c.PointsHeight, c.Width})
	short, _ := json.Marshal(msg{"cursor", c.ID, "", c.HotX, c.HotY, c.PointsWidth, c.PointsHeight, c.Width})
	for _, v := range s.viewerList() {
		v.mu.Lock()
		if v.cursors == nil {
			v.cursors = make(map[string]bool)
		}
		seen := v.cursors[c.ID]
		if !seen && len(v.cursors) < 64 {
			v.cursors[c.ID] = true
		}
		v.mu.Unlock()
		if seen {
			s.SendControl(v.id, short)
		} else {
			s.SendControl(v.id, full)
		}
	}
}

// OnBeacon is told when the viewer flips the latency beacon: a screen
// source shows it on the display (the test card draws its own).
func (s *StreamService) OnBeacon(fn func(on bool, d screencap.Display)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onBeacon = fn
}

func (s *StreamService) viewerList() []*viewer {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Collect(maps.Values(s.viewers))
}

// sendDisplays tells a viewer the host's displays and which one it sees.
func (s *StreamService) sendDisplays(v *viewer) {
	ds := s.refreshDisplays()
	cur := pickDisplay(ds, s.displayNow())
	if ds == nil {
		ds = []screencap.Display{}
	}
	b, _ := json.Marshal(struct {
		Type     string              `json:"type"`
		Displays []screencap.Display `json:"displays"`
		Current  uint32              `json:"current"`
	}{"displays", ds, cur.ID})
	if v.control != nil && v.control.ReadyState() == webrtc.DataChannelStateOpen {
		s.sendText(v.control, b)
	}
}

func (s *StreamService) displayNow() uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.display
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
	delete(s.sizes, peerID)
	delete(s.hearing, peerID)
	empty := len(s.viewers) == 0
	fn := s.onStats
	s.mu.Unlock()
	if v == nil {
		return
	}
	v.close()
	forgetEstimate(v.pc)
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
