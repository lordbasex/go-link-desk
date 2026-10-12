// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux && !headless

#include "capture_linux.h"

#include <X11/Xlib.h>
#include <X11/Xutil.h>
#include <X11/extensions/XShm.h>
#include <X11/extensions/Xfixes.h>
#include <X11/extensions/Xrandr.h>
#include <stdlib.h>
#include <string.h>
#include <sys/ipc.h>
#include <sys/shm.h>

// The screen of an X11 desktop: XShm reads the root window into shared
// memory (no copy through the socket), the area of one monitor at a time.
struct dk_cap {
	Display *dpy;
	Window root;
	XImage *img;
	XShmSegmentInfo shm;
	int iw, ih;         // the shared image's size
	uint8_t *prev;      // the last grab, to skip still screens
	size_t prev_len;
	int prev_w, prev_h;
	int has_fixes;
};

int dk_x11_ready(void) {
	const char *type = getenv("XDG_SESSION_TYPE");
	if (type && strcmp(type, "wayland") == 0) return 0;
	if (getenv("WAYLAND_DISPLAY") && !getenv("DISPLAY")) return 0;
	Display *d = XOpenDisplay(NULL);
	if (!d) return 0;
	XCloseDisplay(d);
	return 1;
}

int dk_monitors(dk_monitor *out, int max) {
	Display *d = XOpenDisplay(NULL);
	if (!d) return -1;
	Window root = DefaultRootWindow(d);
	int n = 0;
	XRRMonitorInfo *m = XRRGetMonitors(d, root, True, &n);
	int count = 0;
	if (m && n > 0) {
		for (int i = 0; i < n && count < max; i++) {
			dk_monitor *o = &out[count++];
			memset(o, 0, sizeof *o);
			char *name = m[i].name ? XGetAtomName(d, m[i].name) : NULL;
			if (name) {
				strncpy(o->name, name, sizeof o->name - 1);
				XFree(name);
			}
			o->x = m[i].x;
			o->y = m[i].y;
			o->w = m[i].width;
			o->h = m[i].height;
			o->primary = m[i].primary;
		}
	}
	if (m) XRRFreeMonitors(m);
	if (count == 0 && max > 0) {
		// No XRandR monitors: the whole screen is one.
		memset(&out[0], 0, sizeof out[0]);
		strcpy(out[0].name, "Screen");
		out[0].w = DisplayWidth(d, DefaultScreen(d));
		out[0].h = DisplayHeight(d, DefaultScreen(d));
		out[0].primary = 1;
		count = 1;
	}
	XCloseDisplay(d);
	return count;
}

static void drop_image(dk_cap *c) {
	if (!c->img) return;
	XShmDetach(c->dpy, &c->shm);
	XDestroyImage(c->img);
	shmdt(c->shm.shmaddr);
	c->img = NULL;
}

static int ensure_image(dk_cap *c, int w, int h) {
	if (c->img && c->iw == w && c->ih == h) return 1;
	drop_image(c);
	int scr = DefaultScreen(c->dpy);
	c->img = XShmCreateImage(c->dpy, DefaultVisual(c->dpy, scr), DefaultDepth(c->dpy, scr), ZPixmap, NULL, &c->shm, w, h);
	if (!c->img) return 0;
	c->shm.shmid = shmget(IPC_PRIVATE, (size_t)c->img->bytes_per_line * h, IPC_CREAT | 0600);
	if (c->shm.shmid < 0) {
		XDestroyImage(c->img);
		c->img = NULL;
		return 0;
	}
	c->shm.shmaddr = c->img->data = shmat(c->shm.shmid, NULL, 0);
	c->shm.readOnly = False;
	XShmAttach(c->dpy, &c->shm);
	XSync(c->dpy, False);
	// Marked for removal now: it goes away with the last process using it.
	shmctl(c->shm.shmid, IPC_RMID, NULL);
	c->iw = w;
	c->ih = h;
	return 1;
}

dk_cap *dk_open(void) {
	Display *d = XOpenDisplay(NULL);
	if (!d || !XShmQueryExtension(d)) {
		if (d) XCloseDisplay(d);
		return NULL;
	}
	dk_cap *c = calloc(1, sizeof *c);
	c->dpy = d;
	c->root = DefaultRootWindow(d);
	int ev, er;
	c->has_fixes = XFixesQueryExtension(d, &ev, &er);
	return c;
}

void dk_close(dk_cap *c) {
	if (!c) return;
	drop_image(c);
	free(c->prev);
	XCloseDisplay(c->dpy);
	free(c);
}

// Blends the pointer (XFixes, premultiplied ARGB) into the BGRA image at
// its place in the grabbed area.
static void draw_cursor(dk_cap *c, int ax, int ay) {
	XFixesCursorImage *ci = XFixesGetCursorImage(c->dpy);
	if (!ci) return;
	int ox = ci->x - ci->xhot - ax, oy = ci->y - ci->yhot - ay;
	for (int j = 0; j < ci->height; j++) {
		int y = oy + j;
		if (y < 0 || y >= c->ih) continue;
		uint8_t *row = (uint8_t *)c->img->data + (size_t)y * c->img->bytes_per_line;
		for (int i = 0; i < ci->width; i++) {
			int x = ox + i;
			if (x < 0 || x >= c->iw) continue;
			unsigned long p = ci->pixels[j * ci->width + i];
			int a = (p >> 24) & 0xff;
			if (!a) continue;
			uint8_t *d = row + x * 4;
			int inv = 255 - a;
			d[2] = (uint8_t)(((p >> 16) & 0xff) + d[2] * inv / 255);
			d[1] = (uint8_t)(((p >> 8) & 0xff) + d[1] * inv / 255);
			d[0] = (uint8_t)((p & 0xff) + d[0] * inv / 255);
		}
	}
	XFree(ci);
}

dk_frame *dk_grab(dk_cap *c, int x, int y, int w, int h, int ow, int oh, int cursor, int *err) {
	*err = 0;
	if (w <= 0 || h <= 0 || ow < 2 || oh < 2) {
		*err = 1;
		return NULL;
	}
	ow &= ~1;
	oh &= ~1;
	if (!ensure_image(c, w, h) || !XShmGetImage(c->dpy, c->root, c->img, x, y, AllPlanes) || c->img->bits_per_pixel != 32) {
		*err = 1;
		return NULL;
	}
	if (cursor && c->has_fixes) draw_cursor(c, x, y);

	// A still screen sends nothing: compare with the last grab.
	size_t len = (size_t)c->img->bytes_per_line * h;
	if (c->prev && c->prev_len == len && c->prev_w == ow && c->prev_h == oh && memcmp(c->prev, c->img->data, len) == 0) return NULL;
	if (c->prev_len != len) {
		free(c->prev);
		c->prev = malloc(len);
		c->prev_len = len;
	}
	memcpy(c->prev, c->img->data, len);
	c->prev_w = ow;
	c->prev_h = oh;

	return dk_scale((const uint8_t *)c->img->data, c->img->bytes_per_line, w, h, ow, oh, 0);
}
