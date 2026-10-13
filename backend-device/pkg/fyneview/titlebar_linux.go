// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux && !headless

package fyneview

/*
#cgo LDFLAGS: -lX11
#include <stdlib.h>
#include <string.h>
#include <X11/Xlib.h>
#include <X11/Xatom.h>

static int dk_ignore(Display *d, XErrorEvent *e) { return 0; }

// dk_frame sets the window's frame hints: GTK's theme variant (GNOME's
// mutter draws a dark or light title bar) and KDE's color scheme file
// (KWin draws the title bar in its colors). An error (a window gone) is
// ignored: Xlib's default handler would end the program.
static void dk_frame(unsigned long win, const char *variant, const char *scheme) {
	Display *d = XOpenDisplay(NULL);
	if (!d) return;
	XErrorHandler prev = XSetErrorHandler(dk_ignore);
	XChangeProperty(d, win, XInternAtom(d, "_GTK_THEME_VARIANT", False), XInternAtom(d, "UTF8_STRING", False),
		8, PropModeReplace, (const unsigned char *)variant, strlen(variant));
	if (scheme[0])
		XChangeProperty(d, win, XInternAtom(d, "_KDE_NET_WM_COLOR_SCHEME", False), XA_STRING,
			8, PropModeReplace, (const unsigned char *)scheme, strlen(scheme));
	XSync(d, False);
	XSetErrorHandler(prev);
	XCloseDisplay(d);
}
*/
import "C"

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
)

// On Linux the window manager draws the frame, its buttons included. The
// window asks it for the app's look: KDE Plasma takes the exact colors,
// from a color scheme file written next to the settings; GNOME up to 43
// a dark or light title bar (its theme's own). GNOME 44 and later draw
// every window's title bar in the system's style (Settings, Appearance).

func init() {
	// KDE Plasma on Wayland would get Fyne's window as a Wayland one,
	// whose title bar takes no colors from here: XWayland's does, through
	// the color scheme hint. (Fyne already uses XWayland on GNOME, which
	// draws no title bars for Wayland windows.)
	if kde() && os.Getenv("WAYLAND_DISPLAY") != "" && os.Getenv("DISPLAY") != "" && os.Getenv("FYNE_PLATFORM") == "" {
		os.Setenv("FYNE_PLATFORM", "x11")
	}
}

func kde() bool { return strings.Contains(strings.ToUpper(os.Getenv("XDG_CURRENT_DESKTOP")), "KDE") }

// paintFrame asks the window manager for a dark or light frame and its
// colors.
func paintFrame(w fyne.Window, night bool, caption, title, border color.NRGBA) {
	nw, ok := w.(driver.NativeWindow)
	if !ok {
		return
	}
	variant := "light"
	if night {
		variant = "dark"
	}
	scheme := ""
	if kde() {
		scheme = kdeScheme(variant, caption, title)
	}
	nw.RunNative(func(ctx any) {
		xc, ok := ctx.(driver.X11WindowContext)
		if !ok || xc.WindowHandle == 0 {
			return
		}
		cv, cs := C.CString(variant), C.CString(scheme)
		defer C.free(unsafe.Pointer(cv))
		defer C.free(unsafe.Pointer(cs))
		C.dk_frame(C.ulong(xc.WindowHandle), cv, cs)
	})
}

// kdeScheme writes a KDE color scheme with the title bar's colors and
// returns its path ("" when it cannot be written). KWin reads the title
// bar from the Header colors (Plasma 5.25 and later) or the WM ones.
func kdeScheme(variant string, caption, title color.NRGBA) string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	dir = filepath.Join(dir, "go-link-desk", "kde")
	if os.MkdirAll(dir, 0o700) != nil {
		return ""
	}
	rgb := func(c color.NRGBA) string { return fmt.Sprintf("%d,%d,%d", c.R, c.G, c.B) }
	dim := color.NRGBA{uint8((int(title.R) + int(caption.R)) / 2), uint8((int(title.G) + int(caption.G)) / 2), uint8((int(title.B) + int(caption.B)) / 2), 0xff}
	name := fmt.Sprintf("go-link-desk-%s-%02x%02x%02x", variant, caption.R, caption.G, caption.B)
	body := fmt.Sprintf(`[General]
Name=go-link Desk
ColorScheme=%[1]s

[Colors:Header]
BackgroundNormal=%[2]s
BackgroundAlternate=%[2]s
ForegroundNormal=%[3]s
ForegroundInactive=%[4]s

[Colors:Header][Inactive]
BackgroundNormal=%[2]s
BackgroundAlternate=%[2]s
ForegroundNormal=%[4]s
ForegroundInactive=%[4]s

[WM]
activeBackground=%[2]s
activeBlend=%[2]s
activeForeground=%[3]s
inactiveBackground=%[2]s
inactiveBlend=%[2]s
inactiveForeground=%[4]s
`, name, rgb(caption), rgb(title), rgb(dim))
	path := filepath.Join(dir, name+".colors")
	if old, err := os.ReadFile(path); err == nil && string(old) == body {
		return path
	}
	if os.WriteFile(path, []byte(body), 0o600) != nil {
		return ""
	}
	return path
}
