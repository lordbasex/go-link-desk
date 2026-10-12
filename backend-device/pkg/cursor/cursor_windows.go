// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build windows && !headless

package cursor

/*
#cgo LDFLAGS: -lgdi32 -luser32
#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

static int cur_position(int *x, int *y, int *shown) {
	CURSORINFO ci;
	memset(&ci, 0, sizeof ci);
	ci.cbSize = sizeof ci;
	if (!GetCursorInfo(&ci)) return 0;
	*x = ci.ptScreenPos.x;
	*y = ci.ptScreenPos.y;
	*shown = (ci.flags & CURSOR_SHOWING) != 0;
	return 1;
}

// Draws the cursor at its size on a 32-bit picture of one color.
static uint8_t *cur_draw(HCURSOR c, int w, int h, uint32_t fill) {
	BITMAPINFO bi;
	memset(&bi, 0, sizeof bi);
	bi.bmiHeader.biSize = sizeof bi.bmiHeader;
	bi.bmiHeader.biWidth = w;
	bi.bmiHeader.biHeight = -h;
	bi.bmiHeader.biPlanes = 1;
	bi.bmiHeader.biBitCount = 32;
	bi.bmiHeader.biCompression = BI_RGB;
	void *bits = NULL;
	HBITMAP b = CreateDIBSection(NULL, &bi, DIB_RGB_COLORS, &bits, NULL, 0);
	if (!b) return NULL;
	HDC dc = CreateCompatibleDC(NULL);
	HGDIOBJ old = SelectObject(dc, b);
	for (int i = 0; i < w * h; i++) ((uint32_t *)bits)[i] = fill;
	DrawIconEx(dc, 0, 0, c, w, h, 0, NULL, DI_NORMAL);
	GdiFlush();
	uint8_t *out = malloc((size_t)w * h * 4);
	if (out) memcpy(out, bits, (size_t)w * h * 4);
	SelectObject(dc, old);
	DeleteDC(dc);
	DeleteObject(b);
	return out;
}

// The pointer's image as straight RGBA, w x h; id is its handle (one per
// shape). NULL while it is hidden or cannot be read.
static uint8_t *cur_image(int *w, int *h, int *hx, int *hy, uintptr_t *id) {
	CURSORINFO ci;
	memset(&ci, 0, sizeof ci);
	ci.cbSize = sizeof ci;
	if (!GetCursorInfo(&ci) || !(ci.flags & CURSOR_SHOWING) || !ci.hCursor) return NULL;
	ICONINFO ii;
	if (!GetIconInfo(ci.hCursor, &ii)) return NULL;
	BITMAP bm;
	int cw = 0, ch = 0;
	if (ii.hbmColor && GetObject(ii.hbmColor, sizeof bm, &bm)) {
		cw = bm.bmWidth;
		ch = bm.bmHeight;
	} else if (ii.hbmMask && GetObject(ii.hbmMask, sizeof bm, &bm)) {
		// A monochrome cursor: its AND and XOR masks one over the other.
		cw = bm.bmWidth;
		ch = bm.bmHeight / 2;
	}
	*hx = (int)ii.xHotspot;
	*hy = (int)ii.yHotspot;
	if (ii.hbmMask) DeleteObject(ii.hbmMask);
	if (ii.hbmColor) DeleteObject(ii.hbmColor);
	if (cw <= 0 || ch <= 0 || cw > 256 || ch > 256) return NULL;
	// Drawn on black and on white: what changes between the two is how
	// see-through each pixel is, whatever kind of cursor it is.
	uint8_t *black = cur_draw(ci.hCursor, cw, ch, 0xFF000000), *white = cur_draw(ci.hCursor, cw, ch, 0xFFFFFFFF);
	uint8_t *out = malloc((size_t)cw * ch * 4);
	if (!black || !white || !out) {
		free(black);
		free(white);
		free(out);
		return NULL;
	}
	for (int i = 0; i < cw * ch; i++) {
		const uint8_t *k = black + i * 4, *v = white + i * 4; // BGRA
		int diff = ((v[0] - k[0]) + (v[1] - k[1]) + (v[2] - k[2])) / 3;
		uint8_t *o = out + i * 4; // RGBA
		if (diff < 0) {
			// Inverts what is under it (the text cursor): black, which
			// reads on the light pages it is used over.
			o[0] = o[1] = o[2] = 0;
			o[3] = 255;
			continue;
		}
		int a = 255 - diff;
		o[3] = (uint8_t)a;
		for (int c = 0; c < 3; c++) {
			int value = a > 0 ? k[2 - c] * 255 / a : 0;
			o[c] = (uint8_t)(value > 255 ? 255 : value);
		}
	}
	free(black);
	free(white);
	*w = cw;
	*h = ch;
	*id = (uintptr_t)ci.hCursor;
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

// Windows reads the pointer with GetCursorInfo, in the virtual screen's
// pixels (the process is per-monitor DPI aware, see pkg/capture), and its
// image by drawing it, which works for every kind of cursor: color ones
// with their alpha, monochrome ones, and the text cursor that inverts what
// is under it (drawn black).

var mu sync.Mutex

// Position is where the pointer is (off every display while it is hidden,
// as when a program hides it to type).
func Position() (x, y float64) {
	var cx, cy, shown C.int
	if C.cur_position(&cx, &cy, &shown) == 0 {
		return 0, 0
	}
	if shown == 0 {
		return -1e6, -1e6
	}
	return float64(cx), float64(cy)
}

var last struct {
	id    C.uintptr_t
	shape Shape
}

// Current is the pointer's image now.
func Current() (Shape, error) {
	mu.Lock()
	defer mu.Unlock()
	var w, h, hx, hy C.int
	var id C.uintptr_t
	pix := C.cur_image(&w, &h, &hx, &hy, &id)
	if pix == nil {
		return Shape{}, errors.New("cursor: no image")
	}
	defer C.free(unsafe.Pointer(pix))
	if last.shape.ID != "" && id == last.id {
		return last.shape, nil
	}
	rgba := C.GoBytes(unsafe.Pointer(pix), w*h*4)
	img := &image.NRGBA{Pix: rgba, Stride: int(w) * 4, Rect: image.Rect(0, 0, int(w), int(h))}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return Shape{}, err
	}
	data := buf.Bytes()
	sum := sha256.Sum256(data)
	s := Shape{ID: hex.EncodeToString(sum[:8]), PNG: data, HotX: int(hx), HotY: int(hy), Width: int(w), Height: int(h), PointsWidth: int(w), PointsHeight: int(h)}
	last.id, last.shape = id, s
	return s, nil
}

// Supported reports whether this build reads the pointer.
const Supported = true
