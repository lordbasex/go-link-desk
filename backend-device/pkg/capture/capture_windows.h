// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build windows && !headless

#pragma once

#include <stdint.h>

#include "frame.h"

typedef struct dk_cap dk_cap;

// One monitor of the desktop, in the virtual screen's pixels.
typedef struct {
	char name[64];
	int x, y, w, h;
	int primary;
} dk_monitor;

// Makes this process see every monitor in real pixels (per-monitor DPI
// awareness), as the manifest of the release's .exe does; a build without
// it (development) would see scaled monitors.
void dk_win_dpi(void);

// The monitors, up to max (names: the monitor's own name, else
// "Display N"); their count.
int dk_monitors(dk_monitor *out, int max, int names);

dk_cap *dk_open(void);
void dk_close(dk_cap *c);

// Grabs the monitor area x, y, w, h and returns it scaled to ow x oh
// (keeping its shape, black bars where it differs) as a new frame (one
// reference), or NULL when nothing changed since the last grab, or while
// Windows shows a desktop no program may read (the lock screen, a UAC
// prompt). *err is set when the screen cannot be read at all. cursor:
// draw the pointer into it. *how is 1 while the Desktop Duplication API
// reads it (the GPU), 2 while GDI does.
dk_frame *dk_grab(dk_cap *c, int x, int y, int w, int h, int ow, int oh, int cursor, int *err, int *how);
