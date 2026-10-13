// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

#pragma once

#include <stdint.h>

typedef struct mf_enc mf_enc;

// Opens an H.264 encoder for w x h I420 frames at fps, kbps (constant
// bitrate), a keyframe every gop frames, High profile or Baseline: the
// GPU's when gpu is set and there is one that takes it, else Windows'
// own; NULL with *err set when there is none.
mf_enc *mf_open(int w, int h, int fps, int kbps, int gop, int high, int gpu, int *err);

// 1 when the encoder runs on hardware.
int mf_hardware(mf_enc *e);

// The encoder's name (its driver's), "" when it has none.
const char *mf_name(mf_enc *e);

// Hands one frame (pts in 100 ns) to the encoder: 0 taken, 1 not taken
// (it has output to give first), <0 failed (-3: the GPU's encoder asked
// for no frame in a second).
int mf_encode(mf_enc *e, const uint8_t *i420, int64_t pts, int key);

// The next encoded access unit (Annex B) and its pts: 1 got one (valid
// until the next call), 0 none until more input, <0 failed.
int mf_output(mf_enc *e, uint8_t **data, int *len, int64_t *pts);

void mf_set_rate(mf_enc *e, int kbps);
void mf_close(mf_enc *e);
