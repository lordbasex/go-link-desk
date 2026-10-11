// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package viewer

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/decoder"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/signalclient"
)

// The H.264 profiles the host offers (its video.go): Constrained High and
// Constrained Baseline, packetization mode 1.
const (
	h264HighFmtp     = "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=640c1f"
	h264BaselineFmtp = "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f"
)

// callStreamID is the stream of the host's camera and microphone in a
// call: this viewer does not take calls yet, so they are ignored.
const callStreamID = "go-link-desk-call"

// keyframeEvery bounds how often a keyframe is asked for (PLI).
const keyframeEvery = 300 * time.Millisecond

// rtcSignal is the WebRTC part of a signal payload.
type rtcSignal struct {
	Kind      string                   `json:"kind"`
	SDP       string                   `json:"sdp,omitempty"`
	Candidate *webrtc.ICECandidateInit `json:"candidate,omitempty"`
}

// peer is the session's WebRTC side: it answers the host's offers (the
// first one and ICE restarts), applying signals strictly in order.
type peer struct {
	s  *Session
	pc *webrtc.PeerConnection

	queue chan json.RawMessage
	done  chan struct{}

	mu       sync.Mutex
	remote   bool
	pending  []webrtc.ICECandidateInit
	control  *webrtc.DataChannel
	pointer  *webrtc.DataChannel
	input    *webrtc.DataChannel
	files    *Files
	videoSSR webrtc.SSRC
	lastPLI  time.Time
	closed   bool
	stats    videoStats
}

// newAPI builds the WebRTC stack: the host's codecs, NACK, RTCP reports
// and the transport-wide feedback its congestion control reads.
func newAPI() (*webrtc.API, error) {
	me := &webrtc.MediaEngine{}
	if err := me.RegisterDefaultCodecs(); err != nil {
		return nil, err
	}
	feedback := []webrtc.RTCPFeedback{{Type: "goog-remb"}, {Type: "ccm", Parameter: "fir"}, {Type: "nack"}, {Type: "nack", Parameter: "pli"}, {Type: "transport-cc"}}
	if err := me.RegisterCodec(webrtc.RTPCodecParameters{RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000, SDPFmtpLine: h264HighFmtp, RTCPFeedback: feedback}, PayloadType: 114}, webrtc.RTPCodecTypeVideo); err != nil {
		return nil, err
	}
	if err := me.RegisterCodec(webrtc.RTPCodecParameters{RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeRTX, ClockRate: 90000, SDPFmtpLine: "apt=114"}, PayloadType: 115}, webrtc.RTPCodecTypeVideo); err != nil {
		return nil, err
	}
	reg := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(me, reg); err != nil {
		return nil, err
	}
	return webrtc.NewAPI(webrtc.WithMediaEngine(me), webrtc.WithInterceptorRegistry(reg), webrtc.WithSettingEngine(settingEngine())), nil
}

// settingEngine is replaced by tests (loopback candidates).
var settingEngine = func() webrtc.SettingEngine { return webrtc.SettingEngine{} }

func newPeer(s *Session, ice []signalclient.ICEServer) (*peer, error) {
	api, err := newAPI()
	if err != nil {
		return nil, err
	}
	cfg := webrtc.Configuration{}
	for _, srv := range ice {
		cfg.ICEServers = append(cfg.ICEServers, webrtc.ICEServer{URLs: srv.URLs, Username: srv.Username, Credential: srv.Credential})
	}
	pc, err := api.NewPeerConnection(cfg)
	if err != nil {
		return nil, err
	}
	p := &peer{s: s, pc: pc, queue: make(chan json.RawMessage, 64), done: make(chan struct{})}
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		init := c.ToJSON()
		s.sendSignal(rtcSignal{Kind: "candidate", Candidate: &init})
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		switch state {
		case webrtc.PeerConnectionStateConnected:
			s.connState("connected")
		case webrtc.PeerConnectionStateDisconnected:
			s.connState("reconnecting")
		case webrtc.PeerConnectionStateFailed:
			// The host restarts ICE with a new offer; it gives up and
			// ends the session after its tries.
			s.connState("reconnecting")
		}
	})
	pc.OnTrack(p.onTrack)
	pc.OnDataChannel(p.onDataChannel)
	go p.run()
	go p.report()
	return p, nil
}

// signal queues one of the host's signals.
func (p *peer) signal(payload json.RawMessage) {
	select {
	case p.queue <- payload:
	case <-p.done:
	}
}

func (p *peer) run() {
	for {
		select {
		case <-p.done:
			return
		case payload := <-p.queue:
			p.apply(payload)
		}
	}
}

func (p *peer) apply(payload json.RawMessage) {
	var sig rtcSignal
	if json.Unmarshal(payload, &sig) != nil {
		return
	}
	switch sig.Kind {
	case "offer":
		if err := p.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: sig.SDP}); err != nil {
			p.s.log.Warn("viewer offer", "err", err)
			p.s.connState("failed")
			return
		}
		answer, err := p.pc.CreateAnswer(nil)
		if err != nil {
			p.s.connState("failed")
			return
		}
		if err := p.pc.SetLocalDescription(answer); err != nil {
			p.s.connState("failed")
			return
		}
		p.s.sendSignal(rtcSignal{Kind: "answer", SDP: answer.SDP})
		p.mu.Lock()
		p.remote = true
		pending := p.pending
		p.pending = nil
		p.mu.Unlock()
		for _, c := range pending {
			_ = p.pc.AddICECandidate(c)
		}
	case "candidate":
		if sig.Candidate == nil {
			return
		}
		p.mu.Lock()
		if !p.remote {
			if len(p.pending) < 50 {
				p.pending = append(p.pending, *sig.Candidate)
			}
			p.mu.Unlock()
			return
		}
		p.mu.Unlock()
		_ = p.pc.AddICECandidate(*sig.Candidate)
	}
}

func (p *peer) onTrack(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
	if track.StreamID() == callStreamID {
		// A call's media: not taken by this viewer yet. Drain it.
		go drain(track)
		return
	}
	if track.Kind() == webrtc.RTPCodecTypeAudio {
		go p.readAudio(track)
		return
	}
	p.mu.Lock()
	p.videoSSR = track.SSRC()
	p.mu.Unlock()
	go p.readVideo(track)
}

func drain(track *webrtc.TrackRemote) {
	buf := make([]byte, 1500)
	for {
		if _, _, err := track.Read(buf); err != nil {
			return
		}
	}
}

func (p *peer) readVideo(track *webrtc.TrackRemote) {
	codec := "vp8"
	if strings.EqualFold(track.Codec().MimeType, webrtc.MimeTypeH264) {
		codec = "h264"
	}
	asm := newAssembler(codec)
	for {
		pkt, _, err := track.ReadRTP()
		if err != nil {
			return
		}
		frames, lost := asm.push(pkt, time.Now())
		p.mu.Lock()
		p.stats.packets++
		if lost > 0 {
			p.stats.lost += lost
		}
		p.mu.Unlock()
		if asm.wantKeyframe() {
			p.keyframe()
		}
		for _, f := range frames {
			p.mu.Lock()
			p.stats.frames++
			p.mu.Unlock()
			if fn := p.s.opts.Events.Video; fn != nil {
				fn(f)
			}
		}
	}
}

// keyframe asks the host for one (PLI), at most once a second.
func (p *peer) keyframe() {
	p.mu.Lock()
	if time.Since(p.lastPLI) < keyframeEvery || p.videoSSR == 0 {
		p.mu.Unlock()
		return
	}
	p.lastPLI = time.Now()
	ssrc := uint32(p.videoSSR)
	p.mu.Unlock()
	_ = p.pc.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: ssrc}})
}

func (p *peer) readAudio(track *webrtc.TrackRemote) {
	dec, err := decoder.NewOpus(1)
	if err != nil {
		drain(track)
		return
	}
	defer dec.Close()
	for {
		pkt, _, err := track.ReadRTP()
		if err != nil {
			return
		}
		if len(pkt.Payload) == 0 {
			continue
		}
		samples, err := dec.Decode(pkt.Payload)
		if err != nil || len(samples) == 0 {
			continue
		}
		if fn := p.s.opts.Events.Audio; fn != nil {
			out := make([]int16, len(samples))
			copy(out, samples)
			fn(out)
		}
	}
}

func (p *peer) onDataChannel(dc *webrtc.DataChannel) {
	switch dc.Label() {
	case "control":
		p.mu.Lock()
		p.control = dc
		p.mu.Unlock()
		dc.OnOpen(func() {
			for _, b := range p.s.takePending() {
				_ = dc.SendText(string(b))
			}
		})
		dc.OnMessage(func(m webrtc.DataChannelMessage) {
			if !m.IsString || len(m.Data) > 64<<10 {
				return
			}
			p.s.onControl(m.Data)
		})
	case "pointer":
		p.mu.Lock()
		p.pointer = dc
		p.mu.Unlock()
	case "input":
		p.mu.Lock()
		p.input = dc
		p.mu.Unlock()
	case "files":
		f := newFiles(dc)
		p.mu.Lock()
		p.files = f
		p.mu.Unlock()
		dc.OnOpen(func() { p.s.filesOpen(f) })
		dc.OnMessage(f.onMessage)
		dc.OnClose(f.closed)
	}
}

// sendControl sends now, or after the channel opens.
func (p *peer) sendControl(b []byte) {
	p.mu.Lock()
	ch := p.control
	p.mu.Unlock()
	if ch == nil || ch.ReadyState() != webrtc.DataChannelStateOpen {
		p.s.mu.Lock()
		if len(p.s.pendingControl) < 20 {
			p.s.pendingControl = append(p.s.pendingControl, b)
		}
		p.s.mu.Unlock()
		// The channel may have opened (and flushed) meanwhile.
		if ch != nil && ch.ReadyState() == webrtc.DataChannelStateOpen {
			for _, m := range p.s.takePending() {
				_ = ch.SendText(string(m))
			}
		}
		return
	}
	_ = ch.SendText(string(b))
}

// sendPointer sends a move; dropped while the channel is not open or is
// backed up (a newer move follows).
func (p *peer) sendPointer(b []byte) {
	p.mu.Lock()
	ch := p.pointer
	p.mu.Unlock()
	if ch == nil || ch.ReadyState() != webrtc.DataChannelStateOpen || ch.BufferedAmount() > 64<<10 {
		return
	}
	_ = ch.Send(b)
}

func (p *peer) sendInput(b []byte) {
	p.mu.Lock()
	ch := p.input
	p.mu.Unlock()
	if ch == nil || ch.ReadyState() != webrtc.DataChannelStateOpen {
		return
	}
	_ = ch.Send(b)
}

// report sends the client_report every 2 seconds: the host moves its
// playout delay with it.
func (p *peer) report() {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-p.done:
			return
		case <-t.C:
		}
		p.mu.Lock()
		st := p.stats
		p.stats = videoStats{}
		p.mu.Unlock()
		loss := 0.0
		if st.packets+st.lost > 0 {
			loss = float64(st.lost) * 100 / float64(st.packets+st.lost)
		}
		b, _ := json.Marshal(map[string]any{"type": "client_report", "metrics": map[string]any{
			"freeze_ms": 0, "video_loss_pct": loss, "jitter_ms": 0, "hidden": p.s.hidden(),
		}})
		p.sendControl(b)
	}
}

func (p *peer) close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	ch := p.control
	f := p.files
	p.mu.Unlock()
	close(p.done)
	if f != nil {
		f.closed()
	}
	// Let a last message (session_end) leave before closing.
	if ch != nil && ch.BufferedAmount() > 0 {
		time.AfterFunc(200*time.Millisecond, func() { _ = p.pc.Close() })
		return
	}
	go func() { _ = p.pc.Close() }()
}

// videoStats count a report window.
type videoStats struct {
	packets int
	lost    int
	frames  int
}
