// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build (linux || windows) && !headless

// The captured pictures of Linux and Windows: packed I420 frames shared by
// reference count, and the scaler that makes them from the screen's 32-bit
// pixels (libyuv: its SIMD rows do a 1080p frame in about a millisecond,
// where plain C took tens).

#include "frame.h"

#include <stdlib.h>
#include <string.h>

#include <libyuv.h>

void dk_retain(dk_frame *f) { __atomic_add_fetch(&f->refs, 1, __ATOMIC_SEQ_CST); }

void dk_release(dk_frame *f) {
	if (__atomic_sub_fetch(&f->refs, 1, __ATOMIC_SEQ_CST) == 0) {
		free(f->data);
		free(f);
	}
}

// The scaled picture before its conversion, kept by each capture thread
// (a frame's worth of memory taken and given back each time costs more
// than the scaling).
static _Thread_local uint8_t *scaled;
static _Thread_local size_t scaled_cap;

// Black bars where the shapes differ. A smaller picture averages the
// pixels under each of its own (a box filter: text stays readable); the
// colors are BT.601 limited range, the encoders' and the test card's.
dk_frame *dk_scale(const uint8_t *src, int bpl, int w, int h, int ow, int oh, int rgb) {
	ow &= ~1;
	oh &= ~1;
	dk_frame *f = calloc(1, sizeof *f);
	f->refs = 1;
	f->w = ow;
	f->h = oh;
	size_t ysize = (size_t)ow * oh;
	f->data = malloc(ysize * 3 / 2);
	uint8_t *Y = f->data, *U = Y + ysize, *V = U + ysize / 4;

	// The picture fitted in ow x oh, keeping its shape (even edges).
	double s = (double)ow / w < (double)oh / h ? (double)ow / w : (double)oh / h;
	int fw = ((int)(w * s)) & ~1, fh = ((int)(h * s)) & ~1;
	int fx = ((ow - fw) / 2) & ~1, fy = ((oh - fh) / 2) & ~1;
	if (fw != ow || fh != oh) {
		memset(Y, 16, ysize);
		memset(U, 128, ysize / 2);
	}
	if (fw < 2 || fh < 2) return f;
	if (fw != w || fh != h) {
		size_t need = (size_t)fw * fh * 4;
		if (need > scaled_cap) {
			free(scaled);
			scaled = malloc(need);
			scaled_cap = scaled ? need : 0;
			if (!scaled) return f;
		}
		// The four bytes of a pixel are scaled alike whatever their order.
		ARGBScale(src, bpl, w, h, scaled, fw * 4, fw, fh, kFilterBox);
		src = scaled;
		bpl = fw * 4;
	}
	uint8_t *y = Y + (size_t)fy * ow + fx;
	uint8_t *u = U + (size_t)(fy / 2) * (ow / 2) + fx / 2;
	uint8_t *v = V + (size_t)(fy / 2) * (ow / 2) + fx / 2;
	// libyuv names formats by a 32-bit word: its ARGB is B, G, R, A in
	// memory (BGRx), its ABGR is R, G, B, A (RGBx).
	if (rgb)
		ABGRToI420(src, bpl, y, ow, u, ow / 2, v, ow / 2, fw, fh);
	else
		ARGBToI420(src, bpl, y, ow, u, ow / 2, v, ow / 2, fw, fh);
	return f;
}
