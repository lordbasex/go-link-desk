// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

package beacon

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -fblocks -mmacosx-version-min=13.0
#cgo LDFLAGS: -framework AppKit
void bea_show(int on, double x, double y, double side);
void bea_hide(void);
*/
import "C"

func show(on bool, x, y, side float64) {
	v := C.int(0)
	if on {
		v = 1
	}
	C.bea_show(v, C.double(x), C.double(y), C.double(side))
}

func hide() { C.bea_hide() }
