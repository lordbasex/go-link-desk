// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux && !headless

#include "pipewire_linux.h"

#include <dlfcn.h>
#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include <pipewire/pipewire.h>
#include <spa/buffer/meta.h>
#include <spa/param/format-utils.h>
#include <spa/param/video/format-utils.h>

// libpipewire's functions, looked up at run time: the binary links none of
// them, and a system without PipeWire runs it all the same (no Wayland
// capture). The headers' inline helpers (the pod builder, the format
// parser) need no library.
#define PW_FUNCS(X)                                                                                               \
	X(pw_init) X(pw_thread_loop_new) X(pw_thread_loop_get_loop) X(pw_thread_loop_start) X(pw_thread_loop_stop) \
	X(pw_thread_loop_destroy) X(pw_thread_loop_lock) X(pw_thread_loop_unlock) X(pw_context_new)               \
	X(pw_context_destroy) X(pw_context_connect_fd) X(pw_core_disconnect) X(pw_properties_new) X(pw_stream_new) \
	X(pw_stream_add_listener) X(pw_stream_connect) X(pw_stream_disconnect) X(pw_stream_destroy)               \
	X(pw_stream_dequeue_buffer) X(pw_stream_queue_buffer) X(pw_stream_update_params)

#define PW_DECL(n) static __typeof__(n) *p_##n;
PW_FUNCS(PW_DECL)

static pthread_mutex_t load_mu = PTHREAD_MUTEX_INITIALIZER;
static int loaded = -1;

int dk_pw_load(void) {
	pthread_mutex_lock(&load_mu);
	if (loaded < 0) {
		loaded = 0;
		void *h = dlopen("libpipewire-0.3.so.0", RTLD_NOW | RTLD_LOCAL);
		if (h) {
			int ok = 1;
#define PW_LOAD(n) \
	if (!(p_##n = (__typeof__(n) *)dlsym(h, #n))) ok = 0;
			PW_FUNCS(PW_LOAD)
			if (ok) {
				p_pw_init(NULL, NULL);
				loaded = 1;
			} else {
				dlclose(h);
			}
		}
	}
	int r = loaded;
	pthread_mutex_unlock(&load_mu);
	return r;
}

struct dk_pw {
	struct pw_thread_loop *loop;
	struct pw_context *ctx;
	struct pw_core *core;
	struct pw_stream *stream;
	struct spa_hook stream_listener;
	struct spa_hook core_listener;

	pthread_mutex_t mu;  // what follows; the stream's thread writes it
	int failed;
	int w, h, rgb;       // the negotiated picture
	uint8_t *pix;        // the latest picture, w * 4 bytes a row
	int dirty;           // it changed since the last take
	dk_pw_pointer ptr;   // ptr.rgba is ours
	int ptr_moved;       // the pointer changed since the last take

	uint8_t *work;  // the picture a take scales (with the pointer)
	size_t work_len;
	int last_ow, last_oh, last_cursor;
};

#define CURSOR_META_SIZE(w, h) (sizeof(struct spa_meta_cursor) + sizeof(struct spa_meta_bitmap) + (w) * (h) * 4)

static void on_state(void *data, enum pw_stream_state old, enum pw_stream_state state, const char *error) {
	dk_pw *p = data;
	(void)old;
	(void)error;
	if (state == PW_STREAM_STATE_ERROR || state == PW_STREAM_STATE_UNCONNECTED) {
		pthread_mutex_lock(&p->mu);
		p->failed = 1;
		pthread_mutex_unlock(&p->mu);
	}
}

static void on_param(void *data, uint32_t id, const struct spa_pod *param) {
	dk_pw *p = data;
	if (!param || id != SPA_PARAM_Format) return;
	uint32_t mt, mst;
	if (spa_format_parse(param, &mt, &mst) < 0 || mt != SPA_MEDIA_TYPE_video || mst != SPA_MEDIA_SUBTYPE_raw) return;
	struct spa_video_info_raw info;
	memset(&info, 0, sizeof info);
	if (spa_format_video_raw_parse(param, &info) < 0) return;
	pthread_mutex_lock(&p->mu);
	if ((int)info.size.width != p->w || (int)info.size.height != p->h) {
		free(p->pix);
		p->pix = NULL;
	}
	p->w = info.size.width;
	p->h = info.size.height;
	p->rgb = info.format == SPA_VIDEO_FORMAT_RGBx || info.format == SPA_VIDEO_FORMAT_RGBA;
	pthread_mutex_unlock(&p->mu);

	// Plain memory buffers (no GPU ones), and the pointer apart.
	uint8_t buf[1024];
	struct spa_pod_builder b = SPA_POD_BUILDER_INIT(buf, sizeof buf);
	const struct spa_pod *params[3];
	params[0] = spa_pod_builder_add_object(&b, SPA_TYPE_OBJECT_ParamMeta, SPA_PARAM_Meta, SPA_PARAM_META_type, SPA_POD_Id(SPA_META_Cursor),
		SPA_PARAM_META_size, SPA_POD_CHOICE_RANGE_Int(CURSOR_META_SIZE(64, 64), CURSOR_META_SIZE(1, 1), CURSOR_META_SIZE(1024, 1024)));
	params[1] = spa_pod_builder_add_object(&b, SPA_TYPE_OBJECT_ParamBuffers, SPA_PARAM_Buffers, SPA_PARAM_BUFFERS_dataType,
		SPA_POD_Int((1 << SPA_DATA_MemPtr) | (1 << SPA_DATA_MemFd)));
	params[2] = spa_pod_builder_add_object(&b, SPA_TYPE_OBJECT_ParamMeta, SPA_PARAM_Meta, SPA_PARAM_META_type, SPA_POD_Id(SPA_META_Header),
		SPA_PARAM_META_size, SPA_POD_Int(sizeof(struct spa_meta_header)));
	p_pw_stream_update_params(p->stream, params, 3);
}

// The pointer's image, straight RGBA, from the metadata's bitmap.
static void read_bitmap(dk_pw *p, struct spa_meta_cursor *mc) {
	if (!mc->bitmap_offset) return;  // unchanged
	struct spa_meta_bitmap *bm = SPA_PTROFF(mc, mc->bitmap_offset, struct spa_meta_bitmap);
	int w = bm->size.width, h = bm->size.height;
	free(p->ptr.rgba);
	p->ptr.rgba = NULL;
	p->ptr.w = p->ptr.h = 0;
	p->ptr.serial++;
	int bgr = bm->format == SPA_VIDEO_FORMAT_BGRA;
	if (w <= 0 || h <= 0 || w > 1024 || h > 1024 || (bm->format != SPA_VIDEO_FORMAT_RGBA && !bgr)) return;
	const uint8_t *src = SPA_PTROFF(bm, bm->offset, uint8_t);
	int stride = bm->stride ? bm->stride : w * 4;
	uint8_t *out = malloc((size_t)w * h * 4);
	for (int y = 0; y < h; y++) {
		const uint8_t *row = src + (size_t)y * stride;
		for (int x = 0; x < w; x++) {
			const uint8_t *s = row + x * 4;
			uint8_t *d = out + ((size_t)y * w + x) * 4;
			d[0] = bgr ? s[2] : s[0];
			d[1] = s[1];
			d[2] = bgr ? s[0] : s[2];
			d[3] = s[3];
		}
	}
	p->ptr.rgba = out;
	p->ptr.w = w;
	p->ptr.h = h;
	p->ptr.hot_x = mc->hotspot.x;
	p->ptr.hot_y = mc->hotspot.y;
}

// The pointer, from a buffer's metadata (under p->mu).
static void read_pointer(dk_pw *p, struct spa_buffer *buf) {
	struct spa_meta_cursor *mc = spa_buffer_find_meta_data(buf, SPA_META_Cursor, sizeof *mc);
	if (!mc) return;
	int visible = spa_meta_cursor_is_valid(mc);
	if (visible) read_bitmap(p, mc);
	if (visible != p->ptr.visible || (visible && (mc->position.x != p->ptr.x || mc->position.y != p->ptr.y)) || mc->bitmap_offset) p->ptr_moved = 1;
	p->ptr.known = 1;
	p->ptr.visible = visible && p->ptr.rgba;
	if (visible) {
		p->ptr.x = mc->position.x;
		p->ptr.y = mc->position.y;
	}
}

// The buffer's picture, when it carries one.
static int has_picture(dk_pw *p, struct spa_buffer *buf) {
	struct spa_data *d = &buf->datas[0];
	return buf->n_datas > 0 && d->data && d->chunk && d->chunk->size > 0 && !(d->chunk->flags & SPA_CHUNK_FLAG_CORRUPTED) && p->w > 0 && p->h > 0;
}

static void on_process(void *data) {
	dk_pw *p = data;
	// Every buffer may move the pointer or change its image (sent only
	// when it changes); of the pictures only the newest matters. Some
	// buffers carry only the pointer.
	struct pw_buffer *pic = NULL, *b;
	pthread_mutex_lock(&p->mu);
	while ((b = p_pw_stream_dequeue_buffer(p->stream))) {
		read_pointer(p, b->buffer);
		if (has_picture(p, b->buffer)) {
			if (pic) p_pw_stream_queue_buffer(p->stream, pic);
			pic = b;
		} else {
			p_pw_stream_queue_buffer(p->stream, b);
		}
	}
	if (pic) {
		struct spa_data *d = &pic->buffer->datas[0];
		int stride = d->chunk->stride ? d->chunk->stride : p->w * 4;
		if (stride >= p->w * 4 && (size_t)d->chunk->offset + (size_t)stride * p->h <= d->maxsize) {
			if (!p->pix) p->pix = malloc((size_t)p->w * p->h * 4);
			const uint8_t *src = SPA_PTROFF(d->data, d->chunk->offset, uint8_t);
			for (int y = 0; y < p->h; y++) memcpy(p->pix + (size_t)y * p->w * 4, src + (size_t)y * stride, (size_t)p->w * 4);
			p->dirty = 1;
		}
	}
	pthread_mutex_unlock(&p->mu);
	if (pic) p_pw_stream_queue_buffer(p->stream, pic);
}

static const struct pw_stream_events stream_events = {
	PW_VERSION_STREAM_EVENTS,
	.state_changed = on_state,
	.param_changed = on_param,
	.process = on_process,
};

static void on_core_error(void *data, uint32_t id, int seq, int res, const char *message) {
	dk_pw *p = data;
	(void)seq;
	(void)res;
	(void)message;
	if (id != PW_ID_CORE) return;
	pthread_mutex_lock(&p->mu);
	p->failed = 1;
	pthread_mutex_unlock(&p->mu);
}

static const struct pw_core_events core_events = {
	PW_VERSION_CORE_EVENTS,
	.error = on_core_error,
};

dk_pw *dk_pw_open(int fd, uint32_t node, char *err, int errlen) {
	if (!dk_pw_load()) {
		snprintf(err, errlen, "PipeWire (libpipewire-0.3) is not installed");
		close(fd);
		return NULL;
	}
	dk_pw *p = calloc(1, sizeof *p);
	pthread_mutex_init(&p->mu, NULL);
	p->loop = p_pw_thread_loop_new("desk-screen", NULL);
	if (!p->loop) goto fail;
	p->ctx = p_pw_context_new(p_pw_thread_loop_get_loop(p->loop), NULL, 0);
	if (!p->ctx || p_pw_thread_loop_start(p->loop) < 0) goto fail;
	p_pw_thread_loop_lock(p->loop);
	p->core = p_pw_context_connect_fd(p->ctx, fd, NULL, 0);
	fd = -1;  // the core has it now
	if (!p->core) {
		p_pw_thread_loop_unlock(p->loop);
		snprintf(err, errlen, "cannot connect to PipeWire");
		goto fail_quiet;
	}
	pw_core_add_listener(p->core, &p->core_listener, &core_events, p);
	p->stream = p_pw_stream_new(p->core, "go-link Desk",
		p_pw_properties_new(PW_KEY_MEDIA_TYPE, "Video", PW_KEY_MEDIA_CATEGORY, "Capture", PW_KEY_MEDIA_ROLE, "Screen", NULL));
	if (!p->stream) {
		p_pw_thread_loop_unlock(p->loop);
		goto fail;
	}
	p_pw_stream_add_listener(p->stream, &p->stream_listener, &stream_events, p);

	// 32-bit pictures in plain memory, any size and rate.
	uint8_t buf[1024];
	struct spa_pod_builder b = SPA_POD_BUILDER_INIT(buf, sizeof buf);
	const struct spa_pod *params[1];
	params[0] = spa_pod_builder_add_object(&b, SPA_TYPE_OBJECT_Format, SPA_PARAM_EnumFormat,
		SPA_FORMAT_mediaType, SPA_POD_Id(SPA_MEDIA_TYPE_video), SPA_FORMAT_mediaSubtype, SPA_POD_Id(SPA_MEDIA_SUBTYPE_raw),
		SPA_FORMAT_VIDEO_format,
		SPA_POD_CHOICE_ENUM_Id(5, SPA_VIDEO_FORMAT_BGRx, SPA_VIDEO_FORMAT_BGRx, SPA_VIDEO_FORMAT_BGRA, SPA_VIDEO_FORMAT_RGBx, SPA_VIDEO_FORMAT_RGBA),
		SPA_FORMAT_VIDEO_size, SPA_POD_CHOICE_RANGE_Rectangle(&SPA_RECTANGLE(1920, 1080), &SPA_RECTANGLE(1, 1), &SPA_RECTANGLE(16384, 16384)),
		SPA_FORMAT_VIDEO_framerate, SPA_POD_CHOICE_RANGE_Fraction(&SPA_FRACTION(60, 1), &SPA_FRACTION(0, 1), &SPA_FRACTION(360, 1)));
	int r = p_pw_stream_connect(p->stream, PW_DIRECTION_INPUT, node, PW_STREAM_FLAG_AUTOCONNECT | PW_STREAM_FLAG_MAP_BUFFERS, params, 1);
	p_pw_thread_loop_unlock(p->loop);
	if (r < 0) goto fail;
	return p;

fail:
	snprintf(err, errlen, "cannot open the screen's PipeWire stream");
fail_quiet:
	if (fd >= 0) close(fd);
	dk_pw_close(p);
	return NULL;
}

void dk_pw_close(dk_pw *p) {
	if (!p) return;
	if (p->loop) {
		p_pw_thread_loop_lock(p->loop);
		if (p->stream) {
			p_pw_stream_disconnect(p->stream);
			p_pw_stream_destroy(p->stream);
		}
		if (p->core) p_pw_core_disconnect(p->core);
		p_pw_thread_loop_unlock(p->loop);
		p_pw_thread_loop_stop(p->loop);
	}
	if (p->ctx) p_pw_context_destroy(p->ctx);
	if (p->loop) p_pw_thread_loop_destroy(p->loop);
	pthread_mutex_destroy(&p->mu);
	free(p->pix);
	free(p->ptr.rgba);
	free(p->work);
	free(p);
}

int dk_pw_failed(dk_pw *p) {
	pthread_mutex_lock(&p->mu);
	int f = p->failed;
	pthread_mutex_unlock(&p->mu);
	return f;
}

void dk_pw_size(dk_pw *p, int *w, int *h) {
	pthread_mutex_lock(&p->mu);
	*w = p->pix ? p->w : 0;
	*h = p->pix ? p->h : 0;
	pthread_mutex_unlock(&p->mu);
}

// Blends the pointer (straight RGBA) into the 32-bit picture.
static void draw_pointer(uint8_t *pix, int w, int h, int rgb, const dk_pw_pointer *c) {
	int ox = c->x - c->hot_x, oy = c->y - c->hot_y;
	int ri = rgb ? 0 : 2, bi = rgb ? 2 : 0;
	for (int j = 0; j < c->h; j++) {
		int y = oy + j;
		if (y < 0 || y >= h) continue;
		for (int i = 0; i < c->w; i++) {
			int x = ox + i;
			if (x < 0 || x >= w) continue;
			const uint8_t *s = c->rgba + ((size_t)j * c->w + i) * 4;
			int a = s[3];
			if (!a) continue;
			uint8_t *d = pix + ((size_t)y * w + x) * 4;
			d[ri] = (uint8_t)((s[0] * a + d[ri] * (255 - a)) / 255);
			d[1] = (uint8_t)((s[1] * a + d[1] * (255 - a)) / 255);
			d[bi] = (uint8_t)((s[2] * a + d[bi] * (255 - a)) / 255);
		}
	}
}

dk_frame *dk_pw_take(dk_pw *p, int ow, int oh, int cursor) {
	pthread_mutex_lock(&p->mu);
	if (!p->pix || ow < 2 || oh < 2) {
		pthread_mutex_unlock(&p->mu);
		return NULL;
	}
	int changed = p->dirty || (cursor && p->ptr_moved) || ow != p->last_ow || oh != p->last_oh || cursor != p->last_cursor;
	if (!changed) {
		pthread_mutex_unlock(&p->mu);
		return NULL;
	}
	int w = p->w, h = p->h, rgb = p->rgb;
	size_t len = (size_t)w * h * 4;
	if (p->work_len != len) {
		free(p->work);
		p->work = malloc(len);
		p->work_len = len;
	}
	memcpy(p->work, p->pix, len);
	dk_pw_pointer ptr = p->ptr;
	int draw = cursor && ptr.visible && ptr.rgba;
	if (draw) draw_pointer(p->work, w, h, rgb, &ptr);  // under the lock: ptr.rgba stays
	p->dirty = 0;
	p->ptr_moved = 0;
	p->last_ow = ow;
	p->last_oh = oh;
	p->last_cursor = cursor;
	pthread_mutex_unlock(&p->mu);
	// Only this goroutine's take touches work.
	return dk_scale(p->work, w * 4, w, h, ow, oh, rgb);
}

void dk_pw_pointer_get(dk_pw *p, dk_pw_pointer *out, unsigned have) {
	pthread_mutex_lock(&p->mu);
	*out = p->ptr;
	out->rgba = NULL;
	if (p->ptr.rgba && p->ptr.serial != have) {
		size_t n = (size_t)p->ptr.w * p->ptr.h * 4;
		out->rgba = malloc(n);
		memcpy(out->rgba, p->ptr.rgba, n);
	}
	pthread_mutex_unlock(&p->mu);
}
