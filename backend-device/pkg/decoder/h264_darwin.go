// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

package decoder

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=13.0
#cgo LDFLAGS: -framework Foundation -framework VideoToolbox -framework CoreMedia -framework CoreVideo -framework CoreFoundation
#include <stdint.h>
#include <stdlib.h>

void *vdec_new(uintptr_t handle);
int vdec_decode(void *ref, const uint8_t *sps, int spsLen, const uint8_t *pps, int ppsLen, const uint8_t *avcc, int len, int64_t ptsMicros);
void vdec_free(void *ref);
*/
import "C"

import (
	"errors"
	"image"
	"runtime/cgo"
	"sync"
	"time"
	"unsafe"
)

// H264 decodes H.264 with VideoToolbox into RGBA pictures. It is not safe
// for concurrent use.
type H264 struct {
	mu       sync.Mutex
	ref      unsafe.Pointer
	handle   cgo.Handle
	sps, pps []byte
	ready    bool // a keyframe with its parameter sets arrived
	closed   bool
}

// NewH264 opens a decoder; onPicture receives each frame (valid only
// during the call) on the decoding goroutine.
func NewH264(onPicture func(Picture)) *H264 {
	h := cgo.NewHandle(onPicture)
	return &H264{ref: C.vdec_new(C.uintptr_t(h)), handle: h}
}

// Decode takes one Annex B access unit. Until a keyframe with its
// parameter sets arrives, frames are dropped (ErrNeedKeyframe), and the
// caller should ask the sender for one.
func (d *H264) Decode(annexB []byte, pts time.Duration) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return errors.New("decoder: closed")
	}
	au := parseAccessUnit(annexB)
	if au.sps != nil && au.pps != nil {
		d.sps, d.pps = append([]byte(nil), au.sps...), append([]byte(nil), au.pps...)
		d.ready = d.ready || au.key
	}
	if !d.ready || d.sps == nil || len(au.avcc) == 0 {
		return ErrNeedKeyframe
	}
	r := C.vdec_decode(d.ref,
		(*C.uint8_t)(unsafe.Pointer(&d.sps[0])), C.int(len(d.sps)),
		(*C.uint8_t)(unsafe.Pointer(&d.pps[0])), C.int(len(d.pps)),
		(*C.uint8_t)(unsafe.Pointer(&au.avcc[0])), C.int(len(au.avcc)), C.int64_t(pts.Microseconds()))
	if r != 0 {
		d.ready = false // wait for the next keyframe
		return ErrNeedKeyframe
	}
	return nil
}

// Close frees the decoder.
func (d *H264) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	d.closed = true
	C.vdec_free(d.ref)
	d.handle.Delete()
}

// HasH264 reports whether this build decodes H.264.
const HasH264 = true

//export goDecoded
func goDecoded(handle C.uintptr_t, bgra *C.uint8_t, w, h, stride C.int) {
	fn := cgo.Handle(handle).Value().(func(Picture))
	img := image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
	src := unsafe.Slice((*byte)(unsafe.Pointer(bgra)), int(stride)*int(h))
	for y := 0; y < int(h); y++ {
		row := src[y*int(stride):]
		dst := img.Pix[y*img.Stride:]
		for x := 0; x < int(w); x++ {
			dst[4*x], dst[4*x+1], dst[4*x+2], dst[4*x+3] = row[4*x+2], row[4*x+1], row[4*x], 255
		}
	}
	fn(img)
}
