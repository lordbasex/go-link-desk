// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

package capture

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -fblocks -mmacosx-version-min=13.0
#cgo LDFLAGS: -framework ScreenCaptureKit -framework AppKit -framework CoreGraphics -framework CoreMedia -framework CoreVideo
#include <stdlib.h>
#include "capture_darwin.h"
*/
import "C"

import (
	"errors"
	"runtime/cgo"
	"sync"
	"time"
	"unsafe"
)

// Displays lists the screens, the main one first.
func Displays() ([]Display, error) {
	var arr [16]C.cap_display
	var cerr *C.char
	n := C.cap_displays(&arr[0], C.int(len(arr)), &cerr)
	if n < 0 {
		return nil, takeErr(cerr)
	}
	out := make([]Display, 0, int(n))
	for i := range int(n) {
		d := arr[i]
		out = append(out, Display{
			ID: uint32(d.id), Name: C.GoString(&d.name[0]),
			Width: int(d.pixelsW), Height: int(d.pixelsH),
			PointsWidth: int(d.pointsW), PointsHeight: int(d.pointsH),
			X: int(d.x), Y: int(d.y), Primary: d.primary != 0,
		})
	}
	for i, d := range out { // the main display first
		if d.Primary && i > 0 {
			out[0], out[i] = out[i], out[0]
		}
	}
	return out, nil
}

// Stream is a running capture.
type Stream struct {
	mu      sync.Mutex
	ref     unsafe.Pointer
	handle  cgo.Handle
	cfg     Config
	stopped bool
}

type callbacks struct {
	frame func(Frame)
	stop  func(error)
}

// Start captures cfg.Display. onFrame runs on the capture's own thread,
// one frame at a time, only for frames whose picture changed; onStop runs
// if the system stops the capture (the display went away, the permission
// was taken back).
func Start(cfg Config, onFrame func(Frame), onStop func(error)) (*Stream, error) {
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, errors.New("capture: no size")
	}
	h := cgo.NewHandle(&callbacks{frame: onFrame, stop: onStop})
	var cerr *C.char
	ref := C.cap_start(C.uint32_t(cfg.Display), C.int(cfg.Width), C.int(cfg.Height), C.int(cfg.FPS), cbool(cfg.ShowCursor), C.uintptr_t(h), &cerr)
	if ref == nil {
		h.Delete()
		return nil, takeErr(cerr)
	}
	return &Stream{ref: ref, handle: h, cfg: cfg}, nil
}

// Update applies a new size, frame rate, cursor setting or display to the
// running capture, without stopping it.
func (s *Stream) Update(cfg Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return errors.New("capture: stopped")
	}
	display := uint32(0)
	if cfg.Display != s.cfg.Display {
		display = cfg.Display
	}
	var cerr *C.char
	if C.cap_update(s.ref, C.uint32_t(display), C.int(cfg.Width), C.int(cfg.Height), C.int(cfg.FPS), cbool(cfg.ShowCursor), &cerr) != 0 {
		return takeErr(cerr)
	}
	s.cfg = cfg
	return nil
}

// Config is the capture's current setting.
func (s *Stream) Config() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// Stop ends the capture and waits for the frame in flight.
func (s *Stream) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.stopped = true
	C.cap_stop(s.ref)
	s.handle.Delete()
}

//export goCaptureFrame
func goCaptureFrame(handle C.uintptr_t, pb unsafe.Pointer, w, h C.int, pts C.int64_t) {
	cb := cgo.Handle(handle).Value().(*callbacks)
	cb.frame(Frame{PixelBuffer: pb, Width: int(w), Height: int(h), PTS: time.Duration(pts) * time.Microsecond})
}

//export goCaptureStopped
func goCaptureStopped(handle C.uintptr_t, msg *C.char) {
	err := takeErr(msg)
	defer func() { _ = recover() }() // the handle is gone if Stop already ran
	if cb := cgo.Handle(handle).Value().(*callbacks); cb.stop != nil {
		cb.stop(err)
	}
}

// Retain keeps a frame's pixel buffer beyond its callback.
func Retain(pb unsafe.Pointer) { C.cap_retain(pb) }

// Release lets go of a retained pixel buffer.
func Release(pb unsafe.Pointer) { C.cap_release(pb) }

// ToI420 unpacks an NV12 pixel buffer of w x h into dst, packed I420
// (w*h*3/2 bytes), for the VP8 encoder.
func ToI420(pb unsafe.Pointer, dst []byte, w, h int) error {
	if len(dst) < w*h*3/2 {
		return errors.New("capture: buffer too small")
	}
	if C.cap_to_i420(pb, (*C.uint8_t)(unsafe.Pointer(&dst[0])), C.int(w), C.int(h)) != 0 {
		return errors.New("capture: frame size changed")
	}
	return nil
}

// HasAccess reports whether the app may record the screen (System
// Settings › Privacy & Security › Screen Recording).
func HasAccess() bool { return C.cap_has_access() != 0 }

// RequestAccess asks the system for the permission: the first time it
// shows its dialog; after that it only reports. A grant usually takes
// effect when the app starts again.
func RequestAccess() bool { return C.cap_request_access() != 0 }

// Supported reports whether this build captures the screen.
const Supported = true

func takeErr(msg *C.char) error {
	if msg == nil {
		return errors.New("capture: unknown error")
	}
	defer C.free(unsafe.Pointer(msg))
	return errors.New("capture: " + C.GoString(msg))
}

func cbool(v bool) C.int {
	if v {
		return 1
	}
	return 0
}
