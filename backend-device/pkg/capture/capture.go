// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package capture captures the screen: ScreenCaptureKit on macOS, which
// hands every changed frame over as an NV12 pixel buffer the hardware
// encoder takes as it is; XShm on an X11 desktop and the remote desktop
// portal's PipeWire streams on a Wayland one, as I420 (Linux); the Desktop
// Duplication API, or GDI, as I420 (Windows). Elsewhere Start returns
// ErrUnsupported and sessions show the test card.
package capture

import (
	"errors"
	"time"
	"unsafe"
)

// ProblemWayland (Problem): the desktop is a Wayland session, which lets
// no program read the screen without its portal (Linux).
const ProblemWayland = "wayland"

// ErrDenied: the person at the computer did not allow sharing the screen
// (a Wayland desktop's portal asks them).
var ErrDenied = errors.New("capture: sharing the screen was not allowed")

// ErrUnsupported: this system has no screen capture in this build.
var ErrUnsupported = errors.New("capture: not supported on this system")

// Display is one screen.
type Display struct {
	ID   uint32 `json:"id"`
	Name string `json:"name"`
	// Width and Height are its native size in pixels (Retina: twice its
	// size in points).
	Width  int `json:"width"`
	Height int `json:"height"`
	// PointsWidth and PointsHeight are its size, and X and Y its top-left
	// corner, in the desktop's coordinates (points).
	PointsWidth  int  `json:"points_width"`
	PointsHeight int  `json:"points_height"`
	X            int  `json:"-"`
	Y            int  `json:"-"`
	Primary      bool `json:"primary"`
}

// Config is what to capture and how.
type Config struct {
	Display uint32 // 0: the main display
	// Width and Height are the frames' size: ScreenCaptureKit scales on
	// the GPU, keeping the aspect (black bars only if it differs).
	Width, Height int
	FPS           int  // the most frames per second; still screens send none
	ShowCursor    bool // draw the pointer into the picture
	// Audio captures the computer's sound too (not the host's own); see
	// Stream.OnAudio.
	Audio bool
}

// The sound's format: what Opus and WebRTC use.
const (
	AudioRate     = 48000
	AudioChannels = 2
)

// Frame is one captured picture: a CVPixelBufferRef (NV12) that is valid
// only during the callback. To keep it, Retain it (and Release it later).
type Frame struct {
	PixelBuffer   unsafe.Pointer
	Width, Height int
	PTS           time.Duration // on the capture's clock
}
