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
}

// Run produces frames at FPS, while anyone watches, until ctx ends.
func (t *TestCardSource) Run(ctx context.Context, sink MediaSink) error {
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
		sink.VideoFrame(Frame{I420: pattern.Frame(now, sink.Controls()), Width: t.Width, Height: t.Height, PTS: now.Sub(start)})
	}
}
