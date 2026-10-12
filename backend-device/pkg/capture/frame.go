// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build (linux || windows) && !headless

package capture

/*
#cgo LDFLAGS: -lyuv
#include "frame.h"
*/
import "C"

import "unsafe"

// scaleI420 is dk_scale for Go: the picture src (w x h, 32 bits a pixel,
// BGRx or RGBx) fitted in ow x oh as packed I420.
func scaleI420(src []byte, w, h, ow, oh int, rgb bool) []byte {
	r := C.int(0)
	if rgb {
		r = 1
	}
	f := C.dk_scale((*C.uint8_t)(unsafe.Pointer(&src[0])), C.int(w*4), C.int(w), C.int(h), C.int(ow), C.int(oh), r)
	defer C.dk_release(f)
	n := int(f.w) * int(f.h) * 3 / 2
	return C.GoBytes(unsafe.Pointer(f.data), C.int(n))
}
