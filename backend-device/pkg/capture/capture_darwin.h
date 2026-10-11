// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

#include <stdint.h>

typedef struct {
	uint32_t id;
	char name[128];
	int pointsW, pointsH; // its size in points (the desktop's coordinates)
	int pixelsW, pixelsH; // in pixels (Retina: twice the points)
	int x, y;             // its top-left corner on the desktop, in points
	int primary;
} cap_display;

int cap_displays(cap_display *out, int max, char **err);
void *cap_start(uint32_t display, int w, int h, int fps, int cursor, int audio, uintptr_t handle, char **err);
int cap_update(void *ref, uint32_t display, int w, int h, int fps, int cursor, int audio, char **err);
void cap_stop(void *ref);
void cap_retain(void *pb);
void cap_release(void *pb);
int cap_to_i420(void *pb, uint8_t *dst, int w, int h);
int cap_has_access(void);
int cap_request_access(void);
