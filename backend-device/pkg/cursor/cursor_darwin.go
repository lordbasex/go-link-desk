// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

package cursor

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=13.0
#cgo LDFLAGS: -framework AppKit -framework CoreGraphics
#include <stdint.h>
#include <stdlib.h>
void cur_position(double *x, double *y);
int cur_shape(uint8_t **png, int *len, int *hx, int *hy, int *w, int *h, int *pw, int *ph);
*/
import "C"

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"unsafe"
)

// Position is where the pointer is, in the desktop's points.
func Position() (x, y float64) {
	var cx, cy C.double
	C.cur_position(&cx, &cy)
	return float64(cx), float64(cy)
}

// Current is the pointer's image now.
func Current() (Shape, error) {
	var png *C.uint8_t
	var n, hx, hy, w, h, pw, ph C.int
	if C.cur_shape(&png, &n, &hx, &hy, &w, &h, &pw, &ph) != 0 {
		return Shape{}, errors.New("cursor: no image")
	}
	defer C.free(unsafe.Pointer(png))
	data := C.GoBytes(unsafe.Pointer(png), n)
	sum := sha256.Sum256(data)
	return Shape{ID: hex.EncodeToString(sum[:8]), PNG: data, HotX: int(hx), HotY: int(hy), Width: int(w), Height: int(h), PointsWidth: int(pw), PointsHeight: int(ph)}, nil
}

// Supported reports whether this build reads the pointer.
const Supported = true
