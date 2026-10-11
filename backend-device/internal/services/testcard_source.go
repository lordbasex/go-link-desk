// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/testpattern"
)

// TestCardSource streams the test card: the session's diagnostics source,
// also used by CI and the end-to-end tests (--source testpattern).
type TestCardSource struct {
	Width, Height, FPS int
	// Picture, when set, is shown instead of the card, silently: a still
	// I420 frame of Width x Height (screenshots of a desktop, --test-picture).
	Picture []byte
}

// Run produces frames at FPS, and the 1 kHz tone for viewers that may
// hear it, while anyone watches, until ctx ends.
func (t *TestCardSource) Run(ctx context.Context, sink MediaSink) error {
	if t.Picture == nil {
		go tone(ctx, sink)
	}
	pattern := testpattern.New(t.Width, t.Height)
	interval := time.Second / time.Duration(t.FPS)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	start := time.Now()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		if !sink.Watching() {
			continue
		}
		now := time.Now()
		frame := t.Picture
		if frame == nil {
			frame = pattern.Frame(now, sink.Controls())
		}
		sink.VideoFrame(Frame{I420: frame, Width: t.Width, Height: t.Height, PTS: now.Sub(start)})
	}
}

// tone plays the test card's 1 kHz tone, 20 ms at a time.
func tone(ctx context.Context, sink MediaSink) {
	gen := testpattern.NewTone(1000)
	t := time.NewTicker(20 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if sink.Hearing() {
				sink.AudioSamples(stereo(gen.Frame()))
			}
		}
	}
}
