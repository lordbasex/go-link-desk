// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux && !headless

package cursor

/*
#cgo LDFLAGS: -lX11 -lXfixes
#include <stdlib.h>
#include <X11/Xlib.h>
#include <X11/extensions/Xfixes.h>

static Display *cur_dpy;

static int cur_open(void) {
	if (!cur_dpy) cur_dpy = XOpenDisplay(NULL);
	return cur_dpy != NULL;
}

static int cur_position(int *x, int *y) {
	if (!cur_open()) return 0;
	Window root, child;
	int wx, wy;
	unsigned int mask;
	return XQueryPointer(cur_dpy, DefaultRootWindow(cur_dpy), &root, &child, x, y, &wx, &wy, &mask);
}

// The pointer's image as straight (not premultiplied) RGBA, w x h.
static unsigned char *cur_image(int *w, int *h, int *hx, int *hy, unsigned long *serial) {
	int ev, er;
	if (!cur_open() || !XFixesQueryExtension(cur_dpy, &ev, &er)) return NULL;
	XFixesCursorImage *ci = XFixesGetCursorImage(cur_dpy);
	if (!ci) return NULL;
	*w = ci->width;
	*h = ci->height;
	*hx = ci->xhot;
	*hy = ci->yhot;
	*serial = ci->cursor_serial;
	unsigned char *out = malloc((size_t)ci->width * ci->height * 4);
	for (int i = 0; i < ci->width * ci->height; i++) {
		unsigned long p = ci->pixels[i];
		int a = (p >> 24) & 0xff, r = (p >> 16) & 0xff, g = (p >> 8) & 0xff, b = p & 0xff;
		if (a > 0 && a < 255) {
			r = r * 255 / a;
			g = g * 255 / a;
			b = b * 255 / a;
		}
		out[i * 4] = r > 255 ? 255 : r;
		out[i * 4 + 1] = g > 255 ? 255 : g;
		out[i * 4 + 2] = b > 255 ? 255 : b;
		out[i * 4 + 3] = a;
	}
	XFree(ci);
	return out;
}
*/
import "C"

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/png"
	"sync"
	"unsafe"
)

// Linux reads the pointer of an X11 desktop: XQueryPointer and XFixes, on
// a display connection of its own.

var mu sync.Mutex

// Position is where the pointer is, in the root window's pixels.
func Position() (x, y float64) {
	mu.Lock()
	defer mu.Unlock()
	var cx, cy C.int
	if C.cur_position(&cx, &cy) == 0 {
		return 0, 0
	}
	return float64(cx), float64(cy)
}

var last struct {
	serial C.ulong
	shape  Shape
}

// Current is the pointer's image now.
func Current() (Shape, error) {
	mu.Lock()
	defer mu.Unlock()
	var w, h, hx, hy C.int
	var serial C.ulong
	pix := C.cur_image(&w, &h, &hx, &hy, &serial)
	if pix == nil {
		return Shape{}, errors.New("cursor: no image")
	}
	defer C.free(unsafe.Pointer(pix))
	if last.shape.ID != "" && serial == last.serial {
		return last.shape, nil
	}
	img := &image.NRGBA{Pix: C.GoBytes(unsafe.Pointer(pix), w*h*4), Stride: int(w) * 4, Rect: image.Rect(0, 0, int(w), int(h))}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return Shape{}, err
	}
	data := buf.Bytes()
	sum := sha256.Sum256(data)
	s := Shape{ID: hex.EncodeToString(sum[:8]), PNG: data, HotX: int(hx), HotY: int(hy), Width: int(w), Height: int(h), PointsWidth: int(w), PointsHeight: int(h)}
	last.serial, last.shape = serial, s
	return s, nil
}

// Supported reports whether this build reads the pointer.
const Supported = true
