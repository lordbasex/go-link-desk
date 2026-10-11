// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

package clipboard

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit
#include <stdlib.h>

long clip_change_count(void);
char *clip_text(void);
long clip_set_text(const char *s);
*/
import "C"

import "unsafe"

// Supported reports whether this build shares the clipboard.
const Supported = true

type mac struct{}

// System is the computer's clipboard.
func System() Board { return mac{} }

func (mac) ChangeCount() int { return int(C.clip_change_count()) }

func (mac) Text() (string, bool) {
	p := C.clip_text()
	if p == nil {
		return "", false
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoString(p), true
}

func (mac) SetText(s string) int {
	cs := C.CString(s)
	defer C.free(unsafe.Pointer(cs))
	return int(C.clip_set_text(cs))
}
