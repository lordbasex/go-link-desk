// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"sync/atomic"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

// callStreamID groups a call's tracks apart from the screen and its sound,
// so the viewer tells the host's camera from its screen.
const callStreamID = "go-link-desk-call"

// callTracks are a viewer's call media, all in its first offer so that a
// call starts and ends without a new negotiation: the host's camera and
// microphone (sent only while a call is on), and the viewer's camera and
// microphone (received, and used only while a call is on).
type callTracks struct {
	camera *webrtc.TrackLocalStaticSample
	cam    *sampleWriter
	codec  string // the camera's codec: the viewer's screen codec
	mic    *webrtc.TrackLocalStaticSample
	// camKey asks the host's camera encoder for a keyframe (a PLI from the
	// viewer, or a call starting).
	camKey atomic.Bool
	// remoteVideoSSRC is the viewer's camera stream, for asking it for a
	// keyframe.
	remoteVideoSSRC atomic.Uint32
}

// h264Receive is what the host decodes from a viewer's camera: H.264 in
// packetization mode 1, Constrained Baseline first (what every browser's
// camera encoder sends), with retransmissions. Without this list Chrome
// would pick VP8, the first in the offer.
var h264Receive = []webrtc.RTPCodecParameters{
	{RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000, SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f"}, PayloadType: 106},
	{RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000, SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f"}, PayloadType: 102},
	{RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeRTX, ClockRate: 90000, SDPFmtpLine: "apt=106"}, PayloadType: 107},
	{RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeRTX, ClockRate: 90000, SDPFmtpLine: "apt=102"}, PayloadType: 103},
}

// addCallTracks puts a viewer's call media in its peer connection.
func (s *StreamService) addCallTracks(v *viewer) error {
	c := &callTracks{codec: v.codec}
	cam, err := newVideoTrackID(trackCodec(v.codec), "camera", callStreamID)
	if err != nil {
		return err
	}
	c.camera, c.cam = cam, &sampleWriter{track: cam}
	camSender, err := v.pc.AddTrack(cam)
	if err != nil {
		return err
	}
	go func() { // a PLI from the viewer: a keyframe of the host's camera
		buf := make([]byte, 1500)
		for {
			n, _, err := camSender.Read(buf)
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
					c.camKey.Store(true)
				}
			}
		}
	}()
	mic, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2}, "mic", callStreamID)
	if err != nil {
		return err
	}
	c.mic = mic
	micSender, err := v.pc.AddTrack(mic)
	if err != nil {
		return err
	}
	go drainRTCP(micSender)

	recvVideo, err := v.pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly})
	if err != nil {
		return err
	}
	if err := recvVideo.SetCodecPreferences(h264Receive); err != nil {
		return err
	}
	if _, err := v.pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}); err != nil {
		return err
	}
	v.pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if track.Kind() == webrtc.RTPCodecTypeVideo {
			c.remoteVideoSSRC.Store(uint32(track.SSRC()))
		}
		s.mu.Lock()
		fn := s.onCallTrack
		s.mu.Unlock()
		if fn == nil {
			go drainTrack(track)
			return
		}
		fn(v.id, track)
	})
	v.call = c
	return nil
}

// OnCallTrack receives each track a viewer sends for a call (its camera
// and its microphone); the function reads it until it ends.
func (s *StreamService) OnCallTrack(fn func(peerID string, track *webrtc.TrackRemote)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onCallTrack = fn
}

// CallOut is where the host's camera and microphone go for a viewer.
type CallOut struct {
	Camera *sampleWriter
	Codec  string
	Mic    *webrtc.TrackLocalStaticSample
	// Keyframe reports (and clears) whether the viewer asked for a camera
	// keyframe.
	Keyframe func() bool
	// AskKeyframe asks the viewer's camera for a keyframe.
	AskKeyframe func()
}

// CallOut gives a viewer's call tracks (false if it has none).
func (s *StreamService) CallOut(peerID string) (CallOut, bool) {
	s.mu.Lock()
	v := s.viewers[peerID]
	s.mu.Unlock()
	if v == nil || v.call == nil {
		return CallOut{}, false
	}
	c := v.call
	return CallOut{
		Camera:   c.cam,
		Codec:    c.codec,
		Mic:      c.mic,
		Keyframe: func() bool { return c.camKey.Swap(false) },
		AskKeyframe: func() {
			if ssrc := c.remoteVideoSSRC.Load(); ssrc != 0 {
				_ = v.pc.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: ssrc}})
			}
		},
	}, true
}

// drainRTCP reads a sender's RTCP so the interceptors get it.
func drainRTCP(sender *webrtc.RTPSender) {
	buf := make([]byte, 1500)
	for {
		if _, _, err := sender.Read(buf); err != nil {
			return
		}
	}
}

// drainTrack reads a track nobody uses, so its buffers do not fill.
func drainTrack(track *webrtc.TrackRemote) {
	buf := make([]byte, 1500)
	for {
		if _, _, err := track.Read(buf); err != nil {
			return
		}
	}
}
