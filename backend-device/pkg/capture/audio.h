// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build (linux || windows) && !headless

#pragma once

// The computer's sound (what its speakers play), as interleaved signed
// 16-bit samples. NULL when the system has none to read: no PulseAudio (or
// PipeWire's) on Linux, no output device on Windows.
void *dk_audio_open(int rate, int channels, int frame_bytes);

// Reads exactly bytes (blocking); -1 when the sound stopped.
int dk_audio_read(void *a, void *buf, int bytes);

void dk_audio_close(void *a);
