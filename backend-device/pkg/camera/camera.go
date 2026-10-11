// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package camera captures the computer's camera for a video call:
// AVFoundation on macOS, which hands NV12 pixel buffers the hardware
// encoder takes as they are. Other systems come in phase 7; there Start
// returns ErrUnsupported.
package camera

import (
	"errors"
	"time"
	"unsafe"
)

// ErrUnsupported: no camera capture on this system in this build.
var ErrUnsupported = errors.New("camera: not supported on this system")

// ErrNoCamera: the computer has no camera.
var ErrNoCamera = errors.New("camera: no camera")

// Frame is one picture: a CVPixelBufferRef (NV12) valid only during the
// callback; Retain it to keep it (capture.Retain and capture.Release work
// on it too).
type Frame struct {
	PixelBuffer   unsafe.Pointer
	Width, Height int
	PTS           time.Duration
}

// Config is the size and rate asked for; the camera gives the nearest it
// has.
type Config struct {
	Width, Height, FPS int
}
