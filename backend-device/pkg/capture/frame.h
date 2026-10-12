// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build (linux || windows) && !headless

#pragma once

#include <stdint.h>

// A captured picture: packed I420 at w x h, shared by reference count.
typedef struct {
	int refs;
	int w, h;
	uint8_t *data;
} dk_frame;

void dk_retain(dk_frame *f);
void dk_release(dk_frame *f);

// The picture src (w x h, 32 bits a pixel, bpl bytes a row: BGRx, or RGBx
// when rgb) fitted in ow x oh, keeping its shape, as a new I420 frame (one
// reference).
dk_frame *dk_scale(const uint8_t *src, int bpl, int w, int h, int ow, int oh, int rgb);
