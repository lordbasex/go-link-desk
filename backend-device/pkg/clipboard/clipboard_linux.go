// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux && !headless

package clipboard

/*
#cgo LDFLAGS: -lX11 -lXfixes -lwayland-client -lpthread
#include <stdlib.h>
int clip_start(void);
int clip_changes(void);
char *clip_text(int *len);
int clip_set(const char *text, int len);
int wlc_start(void);
int wlc_changes(void);
char *wlc_text(int *len);
int wlc_set(const char *text, int len);
*/
import "C"

import (
	"unsafe"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/portal"
)

// Supported reports whether this build shares the clipboard.
const Supported = true

// System is the desktop's clipboard (as text): a Wayland desktop's
// through its data-control protocol when it has one (KDE Plasma, Sway,
// Hyprland), else the X11 one (CLIPBOARD), which XWayland mirrors on
// GNOME.
func System() Board {
	if portal.Wayland() && C.wlc_start() != 0 {
		return wayland{}
	}
	C.clip_start()
	return x11{}
}

type wayland struct{}

func (wayland) ChangeCount() int { return int(C.wlc_changes()) }

func (wayland) Text() (string, bool) {
	var n C.int
	t := C.wlc_text(&n)
	if t == nil {
		return "", false
	}
	defer C.free(unsafe.Pointer(t))
	return C.GoStringN(t, n), true
}

func (wayland) SetText(s string) int {
	cs := C.CString(s)
	defer C.free(unsafe.Pointer(cs))
	return int(C.wlc_set(cs, C.int(len(s))))
}

type x11 struct{}

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
