// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"

	screencap "github.com/lordbasex/go-link-desk/backend-device/pkg/capture"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/encoder"
)

// audioKbps is the sound's bitrate: stereo Opus at 96 kbps carries music
// and voices without artifacts, a small share of a desktop session.
const audioKbps = 96

// audioFrame is one 20 ms Opus frame of interleaved stereo samples.
const audioFrame = encoder.OpusFrameSamples * screencap.AudioChannels

// audioPipe cuts the source's sound into 20 ms frames, encodes them once
// with Opus, and hands each frame to the viewers allowed to hear it. Sound
// is the same for every viewer, so one encoder serves them all.
type audioPipe struct {
	mu  sync.Mutex
	enc *encoder.Opus
	buf []int16
	err time.Time // when opening the encoder last failed
}

// write adds samples (interleaved stereo at 48 kHz) and sends every whole
// frame to the tracks out returns. With nobody to hear it, the sound is
// dropped and the encoder closed.
func (p *audioPipe) write(samples []int16, out func() []*webrtc.TrackLocalStaticSample) {
	p.mu.Lock()
	defer p.mu.Unlock()
	tracks := out()
	if len(tracks) == 0 {
		p.reset()
		return
	}
	if p.enc == nil {
		if time.Since(p.err) < 2*time.Second {
			return
		}
		enc, err := encoder.NewOpus(screencap.AudioChannels, audioKbps*1000)
		if err != nil {
			p.err = time.Now()
			return
		}
		p.enc = enc
	}
	p.buf = append(p.buf, samples...)
	for len(p.buf) >= audioFrame {
		pkt, err := p.enc.Encode(p.buf[:audioFrame])
		p.buf = p.buf[audioFrame:]
		if err != nil {
			continue
		}
		for _, t := range tracks {
			_ = t.WriteSample(media.Sample{Data: pkt, Duration: 20 * time.Millisecond}) // a closed viewer is dropped elsewhere
		}
	}
	// Keep the leftover at the front, so the buffer does not grow.
	p.buf = append(p.buf[:0:0], p.buf...)
}

// reset drops what is buffered and closes the encoder.
func (p *audioPipe) reset() {
	p.buf = p.buf[:0]
	if p.enc != nil {
		p.enc.Close()
		p.enc = nil
	}
}

// newAudioTrack is a viewer's sound: Opus, in the same stream as its
// video, so the browser plays them together.
func newAudioTrack() (*webrtc.TrackLocalStaticSample, error) {
	return webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2}, "audio", "go-link-desk")
}

// stereo doubles mono samples into interleaved stereo.
func stereo(mono []int16) []int16 {
	out := make([]int16, 0, len(mono)*2)
	for _, s := range mono {
		out = append(out, s, s)
	}
	return out
}
