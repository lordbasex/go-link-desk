// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build (linux || windows) && !headless

// The captured pictures of Linux and Windows: packed I420 frames shared by
// reference count, and the scaler that makes them from the screen's 32-bit
// pixels.

#include "frame.h"

#include <stdlib.h>
#include <string.h>

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
							b += p[bi];
							g += p[1];
							r += p[ri];
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
