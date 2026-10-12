// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

#pragma once

#include <stdint.h>

// H.264 from the GPU through VA-API (Intel, AMD): libva is loaded when the
// first encoder opens, so the program runs where it is not installed.
typedef struct va_enc va_enc;

// va_open starts an encoder (high: Constrained High, else Constrained
// Baseline), or returns NULL with *err set.
va_enc *va_open(int w, int h, int fps, int kbps, int gop, int high, int *err);

// va_encode encodes one packed I420 frame (key: an IDR now). The access
// unit (Annex B) is left in *out / *n until the next call. fps_now is the
// frame rate the frames come at (0: unknown), for the rate control.
int va_encode(va_enc *e, const uint8_t *i420, int key, int fps_now, uint8_t **out, int *n);

void va_set_rate(va_enc *e, int kbps);

// va_driver is the driver's name (for the log).
const char *va_driver(va_enc *e);

void va_close(va_enc *e);
