// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless && darwin

package gui

/*
#cgo CFLAGS: -x objective-c -fblocks
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>
void deskFloat(const char *title, int focus);
*/
import "C"

import "unsafe"

// floatWindow keeps the window titled title above the others, on every
// Space; with focus it also comes to the front.
func floatWindow(title string, focus bool) {
	cs := C.CString(title)
	defer C.free(unsafe.Pointer(cs))
	f := C.int(0)
	if focus {
		f = 1
	}
	C.deskFloat(cs, f)
}
