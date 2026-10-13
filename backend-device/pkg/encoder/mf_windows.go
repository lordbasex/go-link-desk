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
	"sync/atomic"
	"time"
	"unsafe"
)

// sysEncoder is H.264 from Media Foundation (mf_windows.c): the GPU's
// encoder (AMD's, Intel's, NVIDIA's) where the computer has one that
// works, else Windows' own in software. Nothing to install, and the
// encoder's licensing comes with the system or the driver. Each frame's
// access unit comes back to onFrame before write returns.
type sysEncoder struct {
	mu      sync.Mutex
	e       *C.mf_enc
	onFrame func([]byte, bool, time.Duration)
	frameSz int
	frame   time.Duration // one frame at the configured rate
	count   int64         // frames written without a time of their own
	params
	hardware int
	name     string
	cfg      Config
	gop      int
	kbps     int // the bitrate now
}

// The GPU's encoder is tried once with a frame before it is used, and
// left for good when it fails that or later: gpuChecked (it made a
// keyframe with its SPS and PPS), gpuBroken (it did not, or it stopped).
var (
	gpuMu      sync.Mutex
	gpuChecked bool
	gpuBroken  atomic.Bool
)

func openMF(cfg Config, gop int, gpu bool) (*C.mf_enc, error) {
	var cerr C.int
	e := C.mf_open(C.int(cfg.Width), C.int(cfg.Height), C.int(cfg.FPS), C.int(cfg.BitrateKbps), C.int(gop), cbool(!cfg.Baseline), cbool(gpu), &cerr)
	if e == nil {
		return nil, fmt.Errorf("h264: no Media Foundation encoder (%d)", int(cerr))
	}
	return e, nil
}

func newSys(cfg Config, gop int, onFrame func([]byte, bool, time.Duration)) (*sysEncoder, error) {
	gpu := !gpuBroken.Load() && gpuWorks(cfg, gop)
	e, err := openMF(cfg, gop, gpu)
	if err != nil {
		return nil, err
	}
	v := &sysEncoder{
		onFrame: onFrame,
		frameSz: cfg.Width*cfg.Height + 2*(cfg.Width/2)*(cfg.Height/2),
		frame:   time.Second / time.Duration(cfg.FPS),
		cfg:     cfg, gop: gop, kbps: cfg.BitrateKbps,
	}
	v.use(e)
	return v, nil
}

func (v *sysEncoder) use(e *C.mf_enc) {
	v.e = e
	v.hardware = int(C.mf_hardware(e))
	v.name = C.GoString(C.mf_name(e))
}

// gpuWorks tries the GPU's encoder once, on a frame of its own: a driver
// that opens and then makes nothing (or no SPS) is not used.
func gpuWorks(cfg Config, gop int) bool {
	gpuMu.Lock()
	defer gpuMu.Unlock()
	if gpuChecked {
		return !gpuBroken.Load()
	}
	e, err := openMF(cfg, gop, true)
	if err != nil {
		return false // none at all: the software one fails too
	}
	if C.mf_hardware(e) == 0 {
		// None, or none for this size (some refuse small pictures): asked
		// again with the next encoder.
		C.mf_close(e)
		return false
	}
	var sps, idr bool
	test := &sysEncoder{
		e: e, frameSz: cfg.Width*cfg.Height + 2*(cfg.Width/2)*(cfg.Height/2),
		onFrame: func(au []byte, _ bool, _ time.Duration) {
			for _, nal := range nalUnits(au) {
				sps = sps || nal[0]&0x1f == 7
				idr = idr || nal[0]&0x1f == 5
			}
		},
	}
	frame := make([]byte, test.frameSz)
	for i := range frame {
		frame[i] = 128
	}
	err = test.encode(frame, 0, true)
	for i := 1; err == nil && !(sps && idr) && i < 3; i++ {
		err = test.encode(frame, time.Duration(i)*time.Second/30, false) // a driver that holds one back
	}
	C.mf_close(e)
	gpuChecked = true
	if err != nil || !sps || !idr {
		gpuBroken.Store(true)
		return false
	}
	return true
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
	err := v.encode(i420, pts, key)
	if err == nil || v.hardware != 1 {
		return err
	}
	// The GPU's encoder stopped (a driver reset, the GPU taken away):
	// Windows' own takes over, from a keyframe, and the session goes on.
	gpuBroken.Store(true)
	C.mf_close(v.e)
	v.e = nil
	cfg := v.cfg
	cfg.BitrateKbps = v.kbps
	e, oerr := openMF(cfg, v.gop, false)
	if oerr != nil {
		return fmt.Errorf("%w; then %v", err, oerr)
	}
	v.use(e)
	return v.encode(i420, pts, true)
}

// encode hands a frame to the encoder and its output to onFrame (under
// v.mu).
func (v *sysEncoder) encode(i420 []byte, pts time.Duration, key bool) error {
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
		v.kbps = kbps
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
