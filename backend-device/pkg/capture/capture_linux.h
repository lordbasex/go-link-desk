// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux && !headless

#include <stdint.h>

// A captured picture: packed I420 at w x h, shared by reference count.
typedef struct {
	int refs;
	int w, h;
	uint8_t *data;
} dk_frame;

typedef struct dk_cap dk_cap;

// One monitor of the X screen (XRandR), in the root window's pixels.
typedef struct {
	char name[64];
	int x, y, w, h;
	int primary;
} dk_monitor;

// The session's X server: 1 for an X11 desktop, 0 when there is none to
// capture (no DISPLAY, or a Wayland session).
int dk_x11_ready(void);

// The monitors, up to max; their count (0: none, -1: no X server).
int dk_monitors(dk_monitor *out, int max);

dk_cap *dk_open(void);
void dk_close(dk_cap *c);

// Grabs the area x, y, w, h of the root window and returns it scaled to
// ow x oh (keeping its shape, black bars where it differs) as a new frame
// (one reference), or NULL when it did not change since the last grab (or
// on failure: *err set). cursor: draw the pointer into it.
dk_frame *dk_grab(dk_cap *c, int x, int y, int w, int h, int ow, int oh, int cursor, int *err);

void dk_retain(dk_frame *f);
void dk_release(dk_frame *f);
