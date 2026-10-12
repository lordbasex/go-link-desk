// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux && !headless

#include "audio_linux.h"

#include <dlfcn.h>
#include <pthread.h>
#include <stdint.h>
#include <stdlib.h>

// The computer's sound: what its speakers play, read from the monitor of
// the default output through PulseAudio's simple API (PipeWire answers it
// too, as pipewire-pulse). libpulse-simple is looked up at run time, so a
// system without it runs the host all the same, silent. The few types of
// it used here are declared below: their layout is PulseAudio's stable
// ABI, and no development headers are needed to build.

typedef struct pa_simple pa_simple;

typedef struct {
	int format; // pa_sample_format_t
	uint32_t rate;
	uint8_t channels;
} dk_pa_sample_spec;

typedef struct {
	uint32_t maxlength, tlength, prebuf, minreq, fragsize;
} dk_pa_buffer_attr;

enum { PA_SAMPLE_S16LE = 3, PA_STREAM_RECORD = 2 };

static pa_simple *(*p_new)(const char *server, const char *name, int dir, const char *dev, const char *stream_name,
	const dk_pa_sample_spec *ss, const void *map, const dk_pa_buffer_attr *attr, int *error);
static int (*p_read)(pa_simple *s, void *data, size_t bytes, int *error);
static void (*p_free)(pa_simple *s);

static pthread_mutex_t load_mu = PTHREAD_MUTEX_INITIALIZER;
static int loaded = -1;

static int load(void) {
	pthread_mutex_lock(&load_mu);
	if (loaded < 0) {
		loaded = 0;
		void *h = dlopen("libpulse-simple.so.0", RTLD_NOW | RTLD_LOCAL);
		if (h) {
			p_new = dlsym(h, "pa_simple_new");
			p_read = dlsym(h, "pa_simple_read");
			p_free = dlsym(h, "pa_simple_free");
			loaded = p_new && p_read && p_free;
		}
	}
	int r = loaded;
	pthread_mutex_unlock(&load_mu);
	return r;
}

void *dk_audio_open(int rate, int channels, int frame_bytes) {
	if (!load()) return NULL;
	dk_pa_sample_spec ss = {PA_SAMPLE_S16LE, (uint32_t)rate, (uint8_t)channels};
	// Small fragments: the sound keeps up with the picture.
	dk_pa_buffer_attr attr = {(uint32_t)-1, (uint32_t)-1, (uint32_t)-1, (uint32_t)-1, (uint32_t)frame_bytes};
	int err = 0;
	return p_new(NULL, "go-link Desk", PA_STREAM_RECORD, "@DEFAULT_MONITOR@", "Computer sound", &ss, NULL, &attr, &err);
}

int dk_audio_read(void *a, void *buf, int bytes) {
	int err = 0;
	return p_read((pa_simple *)a, buf, (size_t)bytes, &err) < 0 ? -1 : 0;
}

void dk_audio_close(void *a) {
	if (a) p_free((pa_simple *)a);
}
