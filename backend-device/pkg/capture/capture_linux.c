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

void dk_retain(dk_frame *f) { __atomic_add_fetch(&f->refs, 1, __ATOMIC_SEQ_CST); }

void dk_release(dk_frame *f) {
	if (__atomic_sub_fetch(&f->refs, 1, __ATOMIC_SEQ_CST) == 0) {
		free(f->data);
		free(f);
	}
}

// BT.601 limited range, the encoders' and the test card's.
static inline uint8_t yv(int r, int g, int b) { return (uint8_t)((66 * r + 129 * g + 25 * b + 128 + 4096) >> 8); }
static inline uint8_t uv(int r, int g, int b) { return (uint8_t)((-38 * r - 74 * g + 112 * b + 128 + 32768) >> 8); }
static inline uint8_t vv(int r, int g, int b) { return (uint8_t)((112 * r - 94 * g - 18 * b + 128 + 32768) >> 8); }

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

// Black bars where the shapes differ; each output pixel averages the
// source pixels under it.
dk_frame *dk_scale(const uint8_t *src, int bpl, int w, int h, int ow, int oh, int rgb) {
	ow &= ~1;
	oh &= ~1;
	int ri = rgb ? 0 : 2, bi = rgb ? 2 : 0;
	dk_frame *f = calloc(1, sizeof *f);
	f->refs = 1;
	f->w = ow;
	f->h = oh;
	size_t ysize = (size_t)ow * oh;
	f->data = malloc(ysize * 3 / 2);
	uint8_t *Y = f->data, *U = Y + ysize, *V = U + ysize / 4;
	memset(Y, 16, ysize);
	memset(U, 128, ysize / 2);

	// The picture fitted in ow x oh, keeping its shape (even edges).
	double s = (double)ow / w < (double)oh / h ? (double)ow / w : (double)oh / h;
	int fw = ((int)(w * s)) & ~1, fh = ((int)(h * s)) & ~1;
	int fx = ((ow - fw) / 2) & ~1, fy = ((oh - fh) / 2) & ~1;
	if (fw < 2 || fh < 2) return f;
	// Each output pixel averages the source pixels under it (a box filter:
	// text stays readable when the picture is smaller than the screen).
	int *x0 = malloc(sizeof(int) * (fw + 1));
	for (int i = 0; i <= fw; i++) x0[i] = (int)((long long)i * w / fw);
	for (int j = 0; j < fh; j += 2) {
		for (int i = 0; i < fw; i += 2) {
			int su = 0, sv = 0;
			for (int dj = 0; dj < 2; dj++) {
				int sy0 = (int)((long long)(j + dj) * h / fh), sy1 = (int)((long long)(j + dj + 1) * h / fh);
				if (sy1 <= sy0) sy1 = sy0 + 1;
				for (int di = 0; di < 2; di++) {
					int sx0 = x0[i + di], sx1 = x0[i + di + 1];
					if (sx1 <= sx0) sx1 = sx0 + 1;
					// At most 4 x 4 samples: enough for any scale down to a quarter.
					int stepx = (sx1 - sx0 + 3) / 4, stepy = (sy1 - sy0 + 3) / 4;
					int r = 0, g = 0, b = 0, n = 0;
					for (int yy = sy0; yy < sy1; yy += stepy) {
						const uint8_t *row = src + (size_t)yy * bpl;
						for (int xx = sx0; xx < sx1; xx += stepx) {
							const uint8_t *p = row + xx * 4;
							b += p[0];
							g += p[1];
							r += p[2];
							n++;
						}
					}
					r /= n;
					g /= n;
					b /= n;
					Y[(size_t)(fy + j + dj) * ow + fx + i + di] = yv(r, g, b);
					su += uv(r, g, b);
					sv += vv(r, g, b);
				}
			}
			size_t ci = (size_t)((fy + j) / 2) * (ow / 2) + (fx + i) / 2;
			U[ci] = (uint8_t)(su / 4);
			V[ci] = (uint8_t)(sv / 4);
		}
	}
	free(x0);
	return f;
}
