// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux && !headless

package clipboard

/*
#cgo LDFLAGS: -lX11 -lXfixes -lpthread
#include <stdlib.h>
int clip_start(void);
int clip_changes(void);
char *clip_text(int *len);
int clip_set(const char *text, int len);
*/
import "C"

import "unsafe"

// Supported reports whether this build shares the clipboard.
const Supported = true

type x11 struct{}

// System is the X11 desktop's clipboard (CLIPBOARD, as text).
func System() Board {
	C.clip_start()
	return x11{}
}

func (x11) ChangeCount() int {
	if C.clip_start() == 0 {
		return 0
	}
	return int(C.clip_changes())
}

func (x11) Text() (string, bool) {
	if C.clip_start() == 0 {
		return "", false
	}
	var n C.int
	t := C.clip_text(&n)
	if t == nil {
		return "", false
	}
	defer C.free(unsafe.Pointer(t))
	return C.GoStringN(t, n), true
}

func (x11) SetText(s string) int {
	if C.clip_start() == 0 {
		return 0
	}
	cs := C.CString(s)
	defer C.free(unsafe.Pointer(cs))
	return int(C.clip_set(cs, C.int(len(s))))
}
