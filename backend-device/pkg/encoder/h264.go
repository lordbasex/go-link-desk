// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package encoder

import (
	"errors"
	"fmt"
	"sync"
	"time"
	"unsafe"
)

// H264 encodes I420 frames to H.264 with the operating system's own
// encoder: VideoToolbox on macOS (the GPU's or Apple silicon's media
// engine), Media Foundation's on Windows (the GPU's where it has one,
// else Windows' own software encoder), VA-API on Linux (Intel's and
// AMD's GPUs). No helper program and nothing to
// install: where the system has no encoder, NewH264 fails and the stream
// uses VP8. Each access
// unit (one frame, Annex B) comes back to onFrame; the SPS and PPS ride with
// every keyframe, which is how a new viewer starts the picture.
type H264 struct {
	sys *sysEncoder
}

// HasH264 reports whether this build can encode H.264.
const HasH264 = hasSys

var available = sync.OnceValue(func() bool {
	if !hasSys {
		return false
	}
	// A size every GPU encoder takes (some refuse tiny pictures).
	w, h := 256, 144
	var sps, idr bool
	e, err := NewH264(Config{Width: w, Height: h, FPS: 30, BitrateKbps: 300}, func(au []byte, _ time.Duration) {
		for _, nal := range nalUnits(au) {
			sps = sps || nal[0]&0x1f == 7
			idr = idr || nal[0]&0x1f == 5
		}
	})
	if err != nil {
		return false
	}
	defer e.Close()
	if !probeOutput {
		return true
	}
	// An encoder that writes its own SPS: one frame shows it does.
	frame := make([]byte, FrameSize(w, h))
	for i := range frame {
		frame[i] = 128
	}
	return e.WriteKey(frame, true) == nil && sps && idr
})

// H264Available reports whether the system's encoder opens on this
// computer (tried once): a build may have it while the system does not,
// as Windows' N editions without the Media Feature Pack.
func H264Available() bool { return available() }

// NewH264 starts the system encoder. onFrame gets each encoded frame
// with its presentation time (the one given to WritePixelBuffer, or the
// frame count over the frame rate for Write), one at a time and in order.
// A frame the rate control drops never comes back.
func NewH264(cfg Config, onFrame func(au []byte, pts time.Duration)) (*H264, error) {
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width%2 != 0 || cfg.Height%2 != 0 {
		return nil, fmt.Errorf("h264: bad size %dx%d", cfg.Width, cfg.Height)
	}
	if !hasSys {
		return nil, errors.New("h264: no system encoder on this platform")
	}
	if cfg.FPS <= 0 {
		cfg.FPS = 30
	}
	if cfg.BitrateKbps <= 0 {
		cfg.BitrateKbps = 8000
	}
	gop := cfg.FPS * 2
	if cfg.GOPFrames > 0 {
		gop = cfg.GOPFrames
	}
	sys, err := newSys(cfg, gop, func(au []byte, _ bool, pts time.Duration) { onFrame(au, pts) })
	if err != nil {
		return nil, err
	}
	return &H264{sys: sys}, nil
}

// Write hands one I420 frame to the encoder.
func (e *H264) Write(i420 []byte) error {
	return e.WriteKey(i420, false)
}

// WriteKey is Write, with key asking for a keyframe now (a viewer just
// joined or lost packets).
func (e *H264) WriteKey(i420 []byte, key bool) error {
	return e.sys.write(i420, key)
}

// WriteAt is WriteKey with the frame's presentation time (the capture's
// clock), for frames that come only when the picture changes.
func (e *H264) WriteAt(i420 []byte, pts time.Duration, key bool) error {
	return e.sys.writeAt(i420, pts, key)
}

// PixelBuffers reports whether WritePixelBuffer takes the capture's own
// buffers (macOS); elsewhere frames go as I420.
const PixelBuffers = pixelBuffers

// WritePixelBuffer encodes a CVPixelBufferRef (NV12, the screen capture's
// own buffer) without copying it, at pts from the capture's clock. The
// caller keeps its reference; the encoder retains the buffer while it
// needs it.
func (e *H264) WritePixelBuffer(pb unsafe.Pointer, pts time.Duration, key bool) error {
	return e.sys.writePixelBuffer(pb, pts, key)
}

// SetBitrate changes the target bitrate while encoding.
func (e *H264) SetBitrate(kbps int) { e.sys.setBitrate(kbps) }

// Hardware reports whether the system encoder runs on hardware (it falls
// back to Apple's software encoder where there is none): known is false
// when the encoder does not say, which the low-latency rate control (a
// hardware feature) does not.
func (e *H264) Hardware() (hardware, known bool) { return e.sys.hardware == 1, e.sys.hardware >= 0 }

// Close stops the encoder and waits for the last frame.
func (e *H264) Close() {
	e.sys.close()
}
