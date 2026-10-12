// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux && !headless

#pragma once

#include <stdint.h>

#include "capture_linux.h"

// A Wayland desktop's screen: one PipeWire stream the remote desktop
// portal shares. libpipewire is loaded when first needed, so the host
// starts on systems without it.
typedef struct dk_pw dk_pw;

// The pointer as the stream showed it last, in the picture's pixels.
typedef struct {
	int known, visible;
	int x, y;  // the hot spot
	int hot_x, hot_y;
	int w, h;
	uint8_t *rgba;  // straight RGBA w x h when it changed (malloc'd), else NULL
	unsigned serial;
} dk_pw_pointer;

// 1 when libpipewire-0.3 is there.
int dk_pw_load(void);

// Connects to the portal's PipeWire remote fd (taken over) and streams
// node; NULL on failure, with err set.
dk_pw *dk_pw_open(int fd, uint32_t node, char *err, int errlen);
void dk_pw_close(dk_pw *p);

// 1 once the stream ended or failed.
int dk_pw_failed(dk_pw *p);

// The picture's size (0 before the first).
void dk_pw_size(dk_pw *p, int *w, int *h);

// The latest picture scaled to ow x oh (cursor: with the pointer drawn
// in) as a new frame, or NULL when nothing changed since the last take.
dk_frame *dk_pw_take(dk_pw *p, int ow, int oh, int cursor);

// Copies the pointer; its image only when its serial is not have.
void dk_pw_pointer_get(dk_pw *p, dk_pw_pointer *out, unsigned have);
