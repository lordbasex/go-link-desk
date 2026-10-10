// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package encoder

import (
	"errors"
	"fmt"
)

// H264 encodes I420 frames to H.264 with the operating system's own
// encoder: VideoToolbox on macOS (the GPU's or Apple silicon's media
// engine). There is no software fallback and no helper program: where the
// system has no encoder, NewH264 fails and the stream uses VP8. Each access
// unit (one frame, Annex B) comes back to onFrame; the SPS and PPS ride with
// every keyframe, which is how a new viewer starts the picture.
type H264 struct {
	vt *vtEncoder
}

// HasH264 reports whether this build can encode H.264.
const HasH264 = hasVT

// NewH264 starts the system encoder. onFrame gets each encoded frame.
func NewH264(cfg Config, onFrame func([]byte)) (*H264, error) {
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width%2 != 0 || cfg.Height%2 != 0 {
		return nil, fmt.Errorf("h264: bad size %dx%d", cfg.Width, cfg.Height)
	}
	if !hasVT {
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
	vt, err := newVT(cfg, gop, onFrame)
	if err != nil {
		return nil, err
	}
	return &H264{vt: vt}, nil
}

// Write hands one I420 frame to the encoder.
func (e *H264) Write(i420 []byte) error {
	return e.WriteKey(i420, false)
}

// WriteKey is Write, with key asking for a keyframe now (a viewer just
// joined or lost packets).
func (e *H264) WriteKey(i420 []byte, key bool) error {
	return e.vt.write(i420, key)
}

// Close stops the encoder and waits for the last frame.
func (e *H264) Close() {
	e.vt.close()
}
