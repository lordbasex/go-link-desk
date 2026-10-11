// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"log/slog"
	"sync"
	"time"

	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/samplebuilder"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/decoder"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/encoder"
)

// A call's states, as the "call_state" message names them.
const (
	CallIdle     = "idle"
	CallRinging  = "ringing"
	CallActive   = "active"
	CallDeclined = "declined"
	CallEnded    = "ended"
)

// The host's camera in a call: small, as a face needs, and light on the
// network next to the screen.
const (
	callCamWidth  = 640
	callCamHeight = 480
	callCamFPS    = 24
	callCamKbps   = 600
	callMicKbps   = 32
)

// CallAnswer is what the person at the host says to a call: whether to
// take it, and with their camera and microphone on.
type CallAnswer struct {
	Accept, Camera, Mic bool
}

// CallConsent asks the person at the host whether to take a call.
type CallConsent interface {
	AskCall(ctx context.Context, name string, video bool) (CallAnswer, error)
}

// CallConsentFunc adapts a function to CallConsent.
type CallConsentFunc func(ctx context.Context, name string, video bool) (CallAnswer, error)

// AskCall calls f.
func (f CallConsentFunc) AskCall(ctx context.Context, name string, video bool) (CallAnswer, error) {
	return f(ctx, name, video)
}

// AutoAcceptCall takes every call with the camera and microphone on: the
// end-to-end tests and measurement builds.
var AutoAcceptCall = CallConsentFunc(func(context.Context, string, bool) (CallAnswer, error) {
	return CallAnswer{Accept: true, Camera: true, Mic: true}, nil
})

// CallMedia is the host's camera and call sound: the computer's own, or a
// test card and a tone for the tests.
type CallMedia interface {
	// StartCamera calls onFrame with each picture until stop.
	StartCamera(onFrame func(Frame)) (stop func(), err error)
	// StartVoice opens the speaker and, with mic, the microphone, whose
	// sound goes to onMic (16-bit mono at 48 kHz).
	StartVoice(mic bool, onMic func([]int16)) (CallVoice, error)
}

// CallVoice is a running call sound.
type CallVoice interface {
	Play(samples []int16) // the other side's voice, 16-bit mono at 48 kHz
	Mute(muted bool)
	Stop()
}

// CallStatus is the call as the window shows it.
type CallStatus struct {
	State        string
	Peer, Name   string
	HostCamera   bool
	HostMic      bool
	ViewerCamera bool
	ViewerMic    bool
}

// CallService is the video call between the viewer in session and the
// person at the host. The viewer asks ("call_request"), the person at the
// host takes it or not, and only then does anything of either side's
// camera or microphone travel or show: the viewer's media is dropped
// unless a call is on, and the host's is not captured at all.
type CallService struct {
	stream *StreamService
	media  CallMedia
	log    *slog.Logger

	mu       sync.Mutex
	consent  CallConsent
	status   CallStatus
	cancel   context.CancelFunc // a pending question
	stopCam  func()
	voice    CallVoice
	camEnc   videoEncoder
	micEnc   *encoder.Opus
	micBuf   []int16
	onStatus func(CallStatus)
	onFrame  func(*image.RGBA)
	seenCam  bool // the viewer's camera showed in this call (logged once)
	seenMic  bool
}

// NewCallService builds the service. Until SetConsent, every call is
// turned down: nobody can answer.
func NewCallService(stream *StreamService, media CallMedia, log *slog.Logger) *CallService {
	if log == nil {
		log = slog.Default()
	}
	c := &CallService{stream: stream, media: media, log: log, status: CallStatus{State: CallIdle}}
	stream.OnCallTrack(c.track)
	return c
}

// SetConsent sets who answers calls (nil: nobody, every call is declined).
func (c *CallService) SetConsent(consent CallConsent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.consent = consent
}

// OnStatus is told whenever the call changes.
func (c *CallService) OnStatus(fn func(CallStatus)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onStatus = fn
}

// OnRemoteFrame receives the viewer's camera while a call is on (on the
// decoding goroutine; the picture is the callee's to keep).
func (c *CallService) OnRemoteFrame(fn func(*image.RGBA)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onFrame = fn
}

// Status is the call now.
func (c *CallService) Status() CallStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

// SetPeer follows the session: a call belongs to one session, and ends
// with it.
func (c *CallService) SetPeer(peerID, name string) {
	c.mu.Lock()
	same := c.status.Peer == peerID
	c.mu.Unlock()
	if same && peerID != "" {
		c.mu.Lock()
		c.status.Name = name
		c.mu.Unlock()
		return
	}
	c.End()
	c.mu.Lock()
	c.status = CallStatus{State: CallIdle, Peer: peerID, Name: name}
	c.mu.Unlock()
}

// HandleControl takes a viewer's call messages.
func (c *CallService) HandleControl(peerID, kind string, data []byte) {
	var m struct {
		Video bool `json:"video"`
		Audio bool `json:"audio"`
	}
	_ = json.Unmarshal(data, &m)
	c.mu.Lock()
	if peerID != c.status.Peer {
		c.mu.Unlock()
		return
	}
	state := c.status.State
	c.mu.Unlock()
	switch kind {
	case "call_request":
		if state == CallIdle || state == CallEnded || state == CallDeclined {
			c.ring(peerID, m.Video, m.Audio)
		}
	case "call_media":
		c.mu.Lock()
		c.status.ViewerCamera, c.status.ViewerMic = m.Video, m.Audio
		c.mu.Unlock()
		c.changed()
	case "call_end":
		c.End()
	}
}

func (c *CallService) ring(peerID string, video, audio bool) {
	ctx, cancel := context.WithTimeout(context.Background(), ConsentTimeout)
	c.mu.Lock()
	consent := c.consent
	c.status.State = CallRinging
	c.status.ViewerCamera, c.status.ViewerMic = video, audio
	c.cancel = cancel
	name := c.status.Name
	c.mu.Unlock()
	c.changed()
	go func() {
		defer cancel()
		ans := CallAnswer{}
		var err error = errors.New("nobody to answer")
		if consent != nil {
			ans, err = consent.AskCall(ctx, name, video)
		}
		c.mu.Lock()
		still := c.status.Peer == peerID && c.status.State == CallRinging
		c.cancel = nil
		c.mu.Unlock()
		if !still {
			return
		}
		if err != nil || !ans.Accept {
			c.mu.Lock()
			c.status.State = CallDeclined
			c.mu.Unlock()
			c.log.Info("call declined", "peer_id", peerID)
			c.changed()
			return
		}
		c.start(peerID, ans)
	}()
}

// start begins a call the person at the host took.
func (c *CallService) start(peerID string, ans CallAnswer) {
	out, ok := c.stream.CallOut(peerID)
	if !ok {
		return
	}
	voice, err := c.media.StartVoice(true, c.micSamples(out))
	if err != nil {
		c.log.Warn("call sound unavailable", "err", err)
	}
	c.mu.Lock()
	c.status.State = CallActive
	c.status.HostMic = ans.Mic && voice != nil
	c.voice = voice
	c.seenCam, c.seenMic = false, false
	c.mu.Unlock()
	if voice != nil {
		voice.Mute(!ans.Mic)
	}
	out.AskKeyframe()
	c.log.Info("call started", "peer_id", peerID, "camera", ans.Camera, "mic", ans.Mic)
	c.SetCamera(ans.Camera)
	c.changed()
}

// micSamples encodes the host's microphone for the viewer, 20 ms at a time.
func (c *CallService) micSamples(out CallOut) func([]int16) {
	return func(samples []int16) {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.status.State != CallActive || !c.status.HostMic {
			c.micBuf = c.micBuf[:0]
			return
		}
		if c.micEnc == nil {
			enc, err := encoder.NewOpus(1, callMicKbps*1000)
			if err != nil {
				return
			}
			c.micEnc = enc
		}
		c.micBuf = append(c.micBuf, samples...)
		for len(c.micBuf) >= encoder.OpusFrameSamples {
			pkt, err := c.micEnc.Encode(c.micBuf[:encoder.OpusFrameSamples])
			c.micBuf = c.micBuf[encoder.OpusFrameSamples:]
			if err == nil {
				_ = out.Mic.WriteSample(media.Sample{Data: pkt, Duration: 20 * time.Millisecond})
			}
		}
		c.micBuf = append(c.micBuf[:0:0], c.micBuf...)
	}
}

// SetCamera turns the host's camera on or off during a call.
func (c *CallService) SetCamera(on bool) {
	c.mu.Lock()
	active := c.status.State == CallActive
	peer := c.status.Peer
	stop := c.stopCam
	c.stopCam = nil
	enc := c.camEnc
	c.camEnc = nil
	c.mu.Unlock()
	if stop != nil {
		stop()
	}
	if enc != nil {
		enc.close()
	}
	if !on || !active {
		c.mu.Lock()
		c.status.HostCamera = false
		c.mu.Unlock()
		c.changed()
		return
	}
	out, ok := c.stream.CallOut(peer)
	if !ok {
		return
	}
	var encMu sync.Mutex
	stopCam, err := c.media.StartCamera(func(f Frame) {
		encMu.Lock()
		defer encMu.Unlock()
		c.mu.Lock()
		e := c.camEnc
		c.mu.Unlock()
		if e != nil {
			if w, h := e.size(); w != f.Width || h != f.Height {
				e.close()
				e = nil
			}
		}
		if e == nil {
			var err error
			if out.Codec == CodecVP8 {
				e, err = newVP8Encoder(f.Width, f.Height, callCamFPS, callCamKbps, out.Camera)
			} else {
				e, err = newH264Encoder(f.Width, f.Height, callCamFPS, callCamKbps, out.Codec == CodecH264, out.Camera)
			}
			if err != nil {
				return
			}
			c.mu.Lock()
			if c.status.State != CallActive || !c.status.HostCamera {
				c.mu.Unlock()
				e.close()
				return
			}
			c.camEnc = e
			c.mu.Unlock()
			_ = e.encode(f, true)
			return
		}
		_ = e.encode(f, out.Keyframe())
	})
	if err != nil {
		c.log.Warn("camera unavailable", "err", err)
		c.mu.Lock()
		c.status.HostCamera = false
		c.mu.Unlock()
		c.changed()
		return
	}
	c.mu.Lock()
	c.stopCam = stopCam
	c.status.HostCamera = true
	c.mu.Unlock()
	c.changed()
}

// SetMic mutes or unmutes the host's microphone during a call.
func (c *CallService) SetMic(on bool) {
	c.mu.Lock()
	v := c.voice
	c.status.HostMic = on && v != nil
	c.mu.Unlock()
	if v != nil {
		v.Mute(!on)
	}
	c.changed()
}

// End hangs up the call, if any (either side, or the session ending).
func (c *CallService) End() {
	c.mu.Lock()
	cancel, stop, voice, enc, mic := c.cancel, c.stopCam, c.voice, c.camEnc, c.micEnc
	c.cancel, c.stopCam, c.voice, c.camEnc, c.micEnc = nil, nil, nil, nil, nil
	was := c.status.State
	if was == CallRinging || was == CallActive {
		c.status.State = CallEnded
	}
	c.status.HostCamera, c.status.HostMic, c.status.ViewerCamera, c.status.ViewerMic = false, false, false, false
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if stop != nil {
		stop()
	}
	if enc != nil {
		enc.close()
	}
	if voice != nil {
		voice.Stop()
	}
	if mic != nil {
		mic.Close()
	}
	if was == CallRinging || was == CallActive {
		c.log.Info("call ended")
		c.changed()
	}
}

// changed tells the viewer and the window what the call is now.
func (c *CallService) changed() {
	c.mu.Lock()
	st := c.status
	fn := c.onStatus
	c.mu.Unlock()
	if st.Peer != "" {
		b, _ := json.Marshal(struct {
			Type      string `json:"type"`
			State     string `json:"state"`
			HostVideo bool   `json:"host_video"`
			HostAudio bool   `json:"host_audio"`
		}{"call_state", st.State, st.HostCamera, st.HostMic})
		c.stream.SendControl(st.Peer, b)
	}
	if fn != nil {
		fn(st)
	}
}

// active reports whether a call with peerID is on.
func (c *CallService) active(peerID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status.Peer == peerID && c.status.State == CallActive
}

// track reads what a viewer sends for a call: its camera is decoded and
// shown, its microphone played, both only while a call is on.
func (c *CallService) track(peerID string, track *webrtc.TrackRemote) {
	if track.Kind() == webrtc.RTPCodecTypeVideo {
		c.readCamera(peerID, track)
		return
	}
	c.readMic(peerID, track)
}

func (c *CallService) readCamera(peerID string, track *webrtc.TrackRemote) {
	if !decoder.HasH264 || track.Codec().MimeType != webrtc.MimeTypeH264 {
		drainTrack(track)
		return
	}
	sb := samplebuilder.New(128, &codecs.H264Packet{}, 90000)
	dec := decoder.NewH264(func(p decoder.Picture) {
		c.mu.Lock()
		fn := c.onFrame
		first := !c.seenCam
		c.seenCam = true
		c.mu.Unlock()
		if first {
			c.log.Info("call: the viewer's camera shows", "peer_id", peerID, "size", p.Bounds().Size().String())
		}
		if fn != nil {
			fn(p)
		}
	})
	defer dec.Close()
	var asked time.Time
	var pts time.Duration
	for {
		pkt, _, err := track.ReadRTP()
		if err != nil {
			return
		}
		if !c.active(peerID) {
			continue
		}
		sb.Push(pkt)
		for s := sb.Pop(); s != nil; s = sb.Pop() {
			pts += s.Duration
			if err := dec.Decode(s.Data, pts); errors.Is(err, decoder.ErrNeedKeyframe) && time.Since(asked) > time.Second {
				asked = time.Now()
				if out, ok := c.stream.CallOut(peerID); ok {
					out.AskKeyframe()
				}
			}
		}
	}
}

func (c *CallService) readMic(peerID string, track *webrtc.TrackRemote) {
	if track.Codec().MimeType != webrtc.MimeTypeOpus {
		drainTrack(track)
		return
	}
	dec, err := decoder.NewOpus(1)
	if err != nil {
		drainTrack(track)
		return
	}
	defer dec.Close()
	for {
		pkt, _, err := track.ReadRTP()
		if err != nil {
			return
		}
		if !c.active(peerID) || len(pkt.Payload) == 0 {
			continue
		}
		pcm, err := dec.Decode(pkt.Payload)
		if err != nil {
			continue
		}
		c.mu.Lock()
		v := c.voice
		first := !c.seenMic
		c.seenMic = true
		c.mu.Unlock()
		if first {
			c.log.Info("call: the viewer's voice plays", "peer_id", peerID)
		}
		if v != nil {
			v.Play(pcm)
		}
	}
}
