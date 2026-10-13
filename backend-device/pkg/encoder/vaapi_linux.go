// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package encoder

/*
#cgo LDFLAGS: -ldl
#include <stdlib.h>
#include "vaapi_linux.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"sync"
	"time"
	"unsafe"
)

// sysEncoder is H.264 from the GPU through VA-API (vaapi_linux.c): Intel's
// and AMD's drivers, where the computer has one and libva is installed.
// Elsewhere (no GPU, a virtual machine, NVIDIA's driver) it does not
// open and sessions use VP8. It runs synchronously: each frame's access
// unit comes back to onFrame before write returns.
type sysEncoder struct {
	mu       sync.Mutex
	e        *C.va_enc
	onFrame  func([]byte, bool, time.Duration)
	frameSz  int
	frame    time.Duration // one frame at the configured rate
	count    int64         // frames written without a time of their own
	last     time.Duration // the last frame's time
	interval time.Duration // the frames' average spacing (0: not known yet)
	params
	hardware int
	name     string
}

// vaErrors names va_open's failures.
var vaErrors = map[int]string{
	-1: "libva is not installed",
	-2: "no GPU whose driver encodes H.264",
	-3: "the driver takes no 4:2:0 pictures",
	-4: "the driver has no bitrate control",
	-5: "the driver refused the configuration",
	-6: "no surfaces",
	-7: "no encoding context",
	-8: "no buffer for the output",
}

func newSys(cfg Config, gop int, onFrame func([]byte, bool, time.Duration)) (*sysEncoder, error) {
	var cerr C.int
	e := C.va_open(C.int(cfg.Width), C.int(cfg.Height), C.int(cfg.FPS), C.int(cfg.BitrateKbps), C.int(gop), cbool(!cfg.Baseline), &cerr)
	if e == nil {
		why := vaErrors[int(cerr)]
		if why == "" {
			why = fmt.Sprint(int(cerr))
		}
		return nil, fmt.Errorf("h264: no VA-API encoder: %s", why)
	}
	return &sysEncoder{
		e: e, onFrame: onFrame,
		frameSz:  cfg.Width*cfg.Height + 2*(cfg.Width/2)*(cfg.Height/2),
		frame:    time.Second / time.Duration(cfg.FPS),
		hardware: 1,
	}, nil
}

func (v *sysEncoder) write(i420 []byte, key bool) error {
	v.mu.Lock()
	pts := time.Duration(v.count) * v.frame
	v.count++
	v.mu.Unlock()
	return v.writeAt(i420, pts, key)
}

// fpsNow is the rate frames come at, from their times: a screen sends
// one when it changes. 0 while not known.
func (v *sysEncoder) fpsNow(pts time.Duration) int {
	if d := pts - v.last; v.last > 0 && d > 0 && d < 5*time.Second {
		if v.interval == 0 {
			v.interval = d
		} else {
			v.interval = (v.interval*7 + d) / 8
		}
	}
	v.last = pts
	if v.interval <= 0 {
		return 0
	}
	return int(time.Second / v.interval)
}

func (v *sysEncoder) writeAt(i420 []byte, pts time.Duration, key bool) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.e == nil {
		return errors.New("h264: closed")
	}
	if len(i420) < v.frameSz {
		return fmt.Errorf("h264: frame is %d bytes, want %d", len(i420), v.frameSz)
	}
	var out *C.uint8_t
	var n C.int
	if C.va_encode(v.e, (*C.uint8_t)(unsafe.Pointer(&i420[0])), cbool(key), C.int(v.fpsNow(pts)), &out, &n) != 0 {
		return errors.New("h264: VA-API encode failed")
	}
	if n > 0 {
		au, isKey := v.withParams(C.GoBytes(unsafe.Pointer(out), n))
		v.onFrame(au, isKey, pts)
	}
	return nil
}

// writePixelBuffer: Linux's capture hands over I420 (WriteAt).
func (v *sysEncoder) writePixelBuffer(unsafe.Pointer, time.Duration, bool) error {
	return errors.New("h264: no pixel buffers on Linux")
}

func (v *sysEncoder) setBitrate(kbps int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.e != nil && kbps > 0 {
		C.va_set_rate(v.e, C.int(kbps))
	}
}

func (v *sysEncoder) close() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.e == nil {
		return
	}
	C.va_close(v.e)
	v.e = nil
}

func cbool(v bool) C.int {
	if v {
		return 1
	}
	return 0
}

const (
	hasSys       = true
	pixelBuffers = false
	// The driver writes the SPS and PPS itself (no packed headers): the
	// encoder is used only once a frame shows it does.
	probeOutput = true
)
