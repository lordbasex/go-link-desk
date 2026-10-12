// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"
	"unsafe"

	screencap "github.com/lordbasex/go-link-desk/backend-device/pkg/capture"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/cursor"
)

// Notices a source sends viewers (control "notice").
const (
	NoticeNoScreenPermission = "no_screen_permission" // showing the test card instead
	NoticeCaptureFailed      = "capture_failed"
	// NoticeWayland: a Linux desktop in a Wayland session, whose screen
	// cannot be read yet; an Xorg session can.
	NoticeWayland = "wayland_session"
)

// When a still screen sends its last frame again: a few times after it
// stops changing (refreshAfter apart, refreshFrames times), so the encoder
// sharpens the picture it left blurry while it moved, and when a viewer
// waits for a keyframe.
const (
	refreshAfter  = 400 * time.Millisecond
	refreshFrames = 4
	keyframeAfter = 100 * time.Millisecond
	// cursorEvery is how often the pointer is read (60 times a second).
	cursorEvery = time.Second / 60
)

// CursorSink takes the host's pointer for the viewers to draw.
type CursorSink interface {
	CursorMoved(x, y float64, visible bool)
	CursorShape(s cursor.Shape)
}

// ScreenSource streams the screen with ScreenCaptureKit: only while
// someone watches (macOS shows its recording indicator only then), at the
// size and rate the viewers need, changed without stopping. Where the
// screen cannot be captured (no permission, another system) it shows the
// fallback, the test card, and tells the viewers why.
type ScreenSource struct {
	Fallback MediaSource
	Log      *slog.Logger
}

type captured struct {
	pb   unsafe.Pointer // retained
	w, h int
	pts  time.Duration
}

// Run captures until ctx ends.
func (src *ScreenSource) Run(ctx context.Context, sink MediaSink) error {
	if src.Log == nil {
		src.Log = slog.Default()
	}
	for ctx.Err() == nil {
		if !waitWatching(ctx, sink) {
			return ctx.Err()
		}
		if !screencap.Supported || !screencap.HasAccess() {
			if screencap.Problem() == screencap.ProblemWayland {
				src.Log.Warn("cannot record the screen of a Wayland session; log in with an Xorg session. Showing the test card")
				sink.Notice(NoticeWayland)
			} else {
				src.Log.Warn("cannot record the screen: no Screen Recording permission; showing the test card")
				sink.Notice(NoticeNoScreenPermission)
			}
			src.fallback(ctx, sink)
			continue
		}
		if err := src.capture(ctx, sink); err != nil {
			src.Log.Error("screen capture failed; showing the test card", "err", err)
			sink.Notice(NoticeCaptureFailed)
			src.fallback(ctx, sink)
		}
	}
	return ctx.Err()
}

// waitWatching waits until a viewer is in (false: ctx ended).
func waitWatching(ctx context.Context, sink MediaSink) bool {
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	for !sink.Watching() {
		select {
		case <-ctx.Done():
			return false
		case <-t.C:
		}
	}
	return true
}

// fallback runs the test card for as long as someone watches.
func (src *ScreenSource) fallback(ctx context.Context, sink MediaSink) {
	fctx, stop := context.WithCancel(ctx)
	defer stop()
	go func() {
		t := time.NewTicker(200 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-fctx.Done():
				return
			case <-t.C:
				if !sink.Watching() {
					stop()
					return
				}
			}
		}
	}()
	if src.Fallback != nil {
		_ = src.Fallback.Run(fctx, sink)
	}
	<-fctx.Done()
}

// capture runs one capture, from the first viewer to the last.
func (src *ScreenSource) capture(ctx context.Context, sink MediaSink) error {
	frames := make(chan captured, 2)
	stopped := make(chan error, 1)
	req := sink.Capture()
	cfg := screencap.Config{Display: req.Display.ID, Width: req.Width, Height: req.Height, FPS: req.FPS, ShowCursor: req.ShowCursor, Audio: req.Audio}
	stream, err := screencap.Start(cfg, func(f screencap.Frame) {
		screencap.Retain(f.PixelBuffer)
		select {
		case frames <- captured{f.PixelBuffer, f.Width, f.Height, f.PTS}:
		default:
			screencap.Release(f.PixelBuffer) // the encoders are behind: skip it
		}
	}, func(err error) {
		select {
		case stopped <- err:
		default:
		}
	})
	if err != nil {
		return err
	}
	stream.OnAudio(sink.AudioSamples)
	src.Log.Info("screen capture started", "display", req.Display.Name, "size", cfgSize(cfg), "fps", cfg.FPS)
	var last captured
	defer func() {
		stream.Stop()
		close(frames)
		for f := range frames {
			screencap.Release(f.pb)
		}
		if last.pb != nil {
			screencap.Release(last.pb)
		}
		src.Log.Info("screen capture stopped")
	}()

	cctx, stopCursor := context.WithCancel(ctx)
	defer stopCursor()
	if cs, ok := sink.(CursorSink); ok && cursor.Supported {
		go followCursor(cctx, cs, sink)
	}

	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	lastAt, refreshed := time.Now(), refreshFrames
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-stopped:
			return err
		case f := <-frames:
			if last.pb != nil {
				screencap.Release(last.pb)
			}
			last, lastAt, refreshed = f, time.Now(), 0
			sink.VideoFrame(Frame{PixelBuffer: f.pb, Width: f.w, Height: f.h, PTS: f.pts})
		case <-sink.Changed():
			req := sink.Capture()
			next := screencap.Config{Display: req.Display.ID, Width: req.Width, Height: req.Height, FPS: req.FPS, ShowCursor: req.ShowCursor, Audio: req.Audio}
			if next != stream.Config() {
				if err := stream.Update(next); err != nil {
					return err
				}
				src.Log.Info("screen capture changed", "display", req.Display.Name, "size", cfgSize(next), "fps", next.FPS, "cursor", next.ShowCursor, "audio", next.Audio)
			}
		case <-tick.C:
			if !sink.Watching() {
				return nil
			}
			quiet := time.Since(lastAt)
			if last.pb == nil || quiet < keyframeAfter {
				continue
			}
			// A still screen sends nothing: send the last picture again for
			// a viewer that needs a keyframe, and once to sharpen it.
			due := refreshed < refreshFrames && quiet >= time.Duration(refreshed+1)*refreshAfter
			if sink.NeedsKeyframe() || due {
				if due {
					refreshed++
				}
				sink.VideoFrame(Frame{PixelBuffer: last.pb, Width: last.w, Height: last.h, PTS: last.pts + quiet})
			}
		}
	}
}

func cfgSize(c screencap.Config) string { return fmt.Sprintf("%dx%d", c.Width, c.Height) }

// followCursor reads the pointer 60 times a second and hands its moves and
// shapes to the viewers, relative to the display being captured.
func followCursor(ctx context.Context, cs CursorSink, sink MediaSink) {
	t := time.NewTicker(cursorEvery)
	defer t.Stop()
	var lastX, lastY float64 = -1, -1
	lastVisible := false
	shapeID := ""
	for n := 0; ; n++ {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		d := sink.Capture().Display
		if d.PointsWidth <= 0 || d.PointsHeight <= 0 {
			continue
		}
		px, py := cursor.Position()
		x := (px - float64(d.X)) / float64(d.PointsWidth)
		y := (py - float64(d.Y)) / float64(d.PointsHeight)
		visible := x >= 0 && x < 1 && y >= 0 && y < 1
		x, y = math.Round(x*10000)/10000, math.Round(y*10000)/10000
		if visible != lastVisible || (visible && (x != lastX || y != lastY)) {
			cs.CursorMoved(x, y, visible)
			lastX, lastY, lastVisible = x, y, visible
		}
		if n%6 == 0 { // the shape, ten times a second
			if s, err := cursor.Current(); err == nil && s.ID != shapeID {
				shapeID = s.ID
				cs.CursorShape(s)
			}
		}
	}
}
