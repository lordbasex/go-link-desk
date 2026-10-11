// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

package camera

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -fblocks -mmacosx-version-min=13.0
#cgo LDFLAGS: -framework AVFoundation -framework CoreMedia -framework CoreVideo
#include <stdint.h>
#include <stdlib.h>

void *cam_start(int w, int h, int fps, uintptr_t handle, char **err);
void cam_stop(void *ref);
int cam_access(void);
int cam_request(void);
*/
import "C"

import (
	"errors"
	"runtime/cgo"
	"sync"
	"time"
	"unsafe"
)

// Supported reports whether this build captures the camera.
const Supported = true

// Stream is a running camera.
type Stream struct {
	mu      sync.Mutex
	ref     unsafe.Pointer
	handle  cgo.Handle
	stopped bool
}

// Start opens the default camera. onFrame runs on the camera's own
// thread, one frame at a time.
func Start(cfg Config, onFrame func(Frame)) (*Stream, error) {
	h := cgo.NewHandle(onFrame)
	var cerr *C.char
	ref := C.cam_start(C.int(cfg.Width), C.int(cfg.Height), C.int(cfg.FPS), C.uintptr_t(h), &cerr)
	if ref == nil {
		h.Delete()
		if cerr == nil {
			return nil, ErrNoCamera
		}
		defer C.free(unsafe.Pointer(cerr))
		return nil, errors.New("camera: " + C.GoString(cerr))
	}
	return &Stream{ref: ref, handle: h}, nil
}

// Stop closes the camera and waits for the frame in flight.
func (s *Stream) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.stopped = true
	C.cam_stop(s.ref)
	s.handle.Delete()
}

// HasAccess reports whether the app may use the camera (System Settings ›
// Privacy & Security › Camera).
func HasAccess() bool { return C.cam_access() == 1 }

// RequestAccess asks the system; the first time it shows its dialog and
// waits for the answer.
func RequestAccess() bool { return C.cam_request() == 1 }

//export goCameraFrame
func goCameraFrame(handle C.uintptr_t, pb unsafe.Pointer, w, h C.int, pts C.int64_t) {
	fn := cgo.Handle(handle).Value().(func(Frame))
	fn(Frame{PixelBuffer: pb, Width: int(w), Height: int(h), PTS: time.Duration(pts) * time.Microsecond})
}
