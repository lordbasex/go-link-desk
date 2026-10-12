// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package encoder

/*
#cgo LDFLAGS: -lmfuuid -lole32 -loleaut32 -luuid -lstrmiids
#include <stdlib.h>
#include "mf_windows.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"sync"
	"time"
	"unsafe"
)

// sysEncoder is H.264 from Media Foundation (mf_windows.c), Windows' own
// encoder: nothing to install, and the encoder's licensing comes with the
// system. It runs synchronously: each frame's access unit comes back to
// onFrame before write returns.
type sysEncoder struct {
	mu      sync.Mutex
	e       *C.mf_enc
	onFrame func([]byte, bool, time.Duration)
	frameSz int
	frame   time.Duration // one frame at the configured rate
	count   int64         // frames written without a time of their own
	params
	hardware int
}

func newSys(cfg Config, gop int, onFrame func([]byte, bool, time.Duration)) (*sysEncoder, error) {
	var cerr C.int
	e := C.mf_open(C.int(cfg.Width), C.int(cfg.Height), C.int(cfg.FPS), C.int(cfg.BitrateKbps), C.int(gop), cbool(!cfg.Baseline), &cerr)
	if e == nil {
		return nil, fmt.Errorf("h264: no Media Foundation encoder (%d)", int(cerr))
	}
	return &sysEncoder{
		e: e, onFrame: onFrame,
		frameSz:  cfg.Width*cfg.Height + 2*(cfg.Width/2)*(cfg.Height/2),
		frame:    time.Second / time.Duration(cfg.FPS),
		hardware: int(C.mf_hardware(e)),
	}, nil
}

func (v *sysEncoder) write(i420 []byte, key bool) error {
	v.mu.Lock()
	pts := time.Duration(v.count) * v.frame
	v.count++
	v.mu.Unlock()
	return v.writeAt(i420, pts, key)
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
	in := (*C.uint8_t)(unsafe.Pointer(&i420[0]))
	st := C.mf_encode(v.e, in, C.int64_t(pts/100), cbool(key))
	if st == 1 {
		// Its output first, then the frame again.
		if err := v.drain(); err != nil {
			return err
		}
		st = C.mf_encode(v.e, in, C.int64_t(pts/100), cbool(key))
	}
	if st < 0 {
		return fmt.Errorf("h264: Media Foundation encode failed (%d)", int(st))
	}
	return v.drain()
}

// drain hands every access unit the encoder has to onFrame (under v.mu).
func (v *sysEncoder) drain() error {
	for {
		var data *C.uint8_t
		var n C.int
		var pts C.int64_t
		switch C.mf_output(v.e, &data, &n, &pts) {
		case 0:
			return nil
		case 1:
			au := C.GoBytes(unsafe.Pointer(data), n)
			au, key := v.withParams(au)
			v.onFrame(au, key, time.Duration(pts)*100)
		default:
			return errors.New("h264: Media Foundation output failed")
		}
	}
}

// writePixelBuffer: Windows' capture hands over I420 (WriteAt).
func (v *sysEncoder) writePixelBuffer(unsafe.Pointer, time.Duration, bool) error {
	return errors.New("h264: no pixel buffers on Windows")
}

func (v *sysEncoder) setBitrate(kbps int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.e != nil && kbps > 0 {
		C.mf_set_rate(v.e, C.int(kbps))
	}
}

func (v *sysEncoder) close() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.e == nil {
		return
	}
	C.mf_close(v.e)
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
	probeOutput  = false
)
