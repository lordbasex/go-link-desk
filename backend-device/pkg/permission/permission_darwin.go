// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

package permission

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=13.0
#cgo LDFLAGS: -framework AppKit -framework ApplicationServices -framework CoreGraphics
#include <stdlib.h>
int perm_screen(void);
int perm_request_screen(void);
int perm_accessibility(void);
int perm_request_accessibility(void);
void perm_open_settings(const char *pane);
*/
import "C"

import "unsafe"

func screenRecording() bool        { return C.perm_screen() != 0 }
func requestScreenRecording() bool { return C.perm_request_screen() != 0 }
func accessibility() bool          { return C.perm_accessibility() != 0 }
func requestAccessibility() bool   { return C.perm_request_accessibility() != 0 }

func openSettings(pane string) {
	cs := C.CString(pane)
	defer C.free(unsafe.Pointer(cs))
	C.perm_open_settings(cs)
}

const needed = true
