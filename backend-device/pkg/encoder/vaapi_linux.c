// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// H.264 with the GPU's encoder through VA-API (libva, MIT): Intel's and
// AMD's drivers. libva is opened with dlopen, so the program needs it
// only where a session uses it. The stream is what a browser decodes
// with the least delay: P frames only, one reference, picture order
// type 2 (output order is decode order), constant bitrate. The driver
// writes the SPS and PPS (no packed headers); the Go side checks they
// come, and keeps them for keyframes that come without.

#include "vaapi_linux.h"

#include <dlfcn.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include <va/va.h>
#include <va/va_drm.h>
#include <va/va_enc_h264.h>

// libva's functions, found at run time. Their types come from the headers.
#define VA_FUNCS(X)                                                              \
	X(vaInitialize) X(vaTerminate) X(vaMaxNumProfiles) X(vaMaxNumEntrypoints)    \
	X(vaQueryConfigProfiles) X(vaQueryConfigEntrypoints) X(vaGetConfigAttributes) \
	X(vaCreateConfig) X(vaDestroyConfig) X(vaCreateSurfaces) X(vaDestroySurfaces) \
	X(vaCreateContext) X(vaDestroyContext) X(vaCreateBuffer) X(vaDestroyBuffer)  \
	X(vaMapBuffer) X(vaUnmapBuffer) X(vaBeginPicture) X(vaRenderPicture)         \
	X(vaEndPicture) X(vaSyncSurface) X(vaDeriveImage) X(vaCreateImage)           \
	X(vaPutImage) X(vaDestroyImage) X(vaQueryVendorString)

#define VA_DECLARE(f) static __typeof__(f) *p_##f;
VA_FUNCS(VA_DECLARE)
static __typeof__(vaGetDisplayDRM) *p_vaGetDisplayDRM;
static __typeof__(vaSetInfoCallback) *p_vaSetInfoCallback;
static __typeof__(vaSetErrorCallback) *p_vaSetErrorCallback;

// load opens libva once: 0 when it is there.
static int load(void) {
	static int state; // 0 not tried, 1 loaded, -1 missing
	if (state) return state > 0 ? 0 : -1;
	state = -1;
	void *va = dlopen("libva.so.2", RTLD_NOW | RTLD_LOCAL);
	void *drm = dlopen("libva-drm.so.2", RTLD_NOW | RTLD_LOCAL);
	if (!va || !drm) return -1;
#define VA_LOAD(f) \
	if (!(p_##f = (__typeof__(f) *)dlsym(va, #f))) return -1;
	VA_FUNCS(VA_LOAD)
	if (!(p_vaGetDisplayDRM = (__typeof__(vaGetDisplayDRM) *)dlsym(drm, "vaGetDisplayDRM"))) return -1;
	// Optional: without them libva writes its notes to stderr.
	p_vaSetInfoCallback = (__typeof__(vaSetInfoCallback) *)dlsym(va, "vaSetInfoCallback");
	p_vaSetErrorCallback = (__typeof__(vaSetErrorCallback) *)dlsym(va, "vaSetErrorCallback");
	state = 1;
	return 0;
}

#define REFS 2 // the picture being made and the one it refers to

struct va_enc {
	int fd;
	VADisplay dpy;
	VAConfigID config;
	VAContextID ctx;
	VASurfaceID input;
	VASurfaceID recon[REFS];
	VABufferID coded;
	int w, h, wmb, hmb; // the picture, and its size in macroblocks
	int fps, kbps, gop, high, level;
	int fps_sent;  // the frame rate the rate control was given
	int rate_new;  // the bitrate changed: tell the rate control
	int since_idr; // frames since the last IDR
	int idr_id;
	int cur;       // recon[cur] is made now, recon[!cur] is the reference
	uint8_t *out;
	size_t out_cap;
	char driver[128];
	// Where the frame goes when the surface cannot be mapped (vaPutImage).
	VAImage image;
	int have_image;
};

// find_entry tells whether a driver encodes H.264 at this profile, and
// with which entry point: 0, or -1.
static int find_entry(VADisplay dpy, VAProfile profile, VAEntrypoint *entry) {
	int n = p_vaMaxNumProfiles(dpy);
	if (n <= 0) return -1;
	VAProfile *profiles = calloc(n, sizeof *profiles);
	int got = 0, found = 0;
	if (p_vaQueryConfigProfiles(dpy, profiles, &got) == VA_STATUS_SUCCESS)
		for (int i = 0; i < got; i++) found |= profiles[i] == profile;
	free(profiles);
	if (!found) return -1;
	n = p_vaMaxNumEntrypoints(dpy);
	if (n <= 0) return -1;
	VAEntrypoint *entries = calloc(n, sizeof *entries);
	int slice = 0, lp = 0;
	if (p_vaQueryConfigEntrypoints(dpy, profile, entries, &got) == VA_STATUS_SUCCESS)
		for (int i = 0; i < got; i++) {
			slice |= entries[i] == VAEntrypointEncSlice;
			lp |= entries[i] == VAEntrypointEncSliceLP;
		}
	free(entries);
	// The full encoder where there is one; the low-power one (newer Intel
	// chips have only that) otherwise.
	if (slice) *entry = VAEntrypointEncSlice;
	else if (lp) *entry = VAEntrypointEncSliceLP;
	else return -1;
	return 0;
}

static void quiet(void *ctx, const char *msg) { (void)ctx, (void)msg; }

// level is the H.264 level for a picture size (macroblocks) at 60 fps or
// less, as far as 4K.
static int level_for(int mbs, int fps) {
	long rate = (long)mbs * fps;
	if (mbs <= 1620 && rate <= 40500) return 31;
	if (mbs <= 8192 && rate <= 245760) return 42;
	if (mbs <= 22080 && rate <= 589824) return 51;
	return 52;
}

static int make_buffer(va_enc *e, VABufferType type, unsigned size, void *data, VABufferID *id) {
	return p_vaCreateBuffer(e->dpy, e->ctx, type, size, 1, data, id) == VA_STATUS_SUCCESS ? 0 : -1;
}

// misc makes a misc parameter buffer of a type with its payload.
static int misc(va_enc *e, VAEncMiscParameterType type, const void *payload, size_t size, VABufferID *id) {
	size_t total = sizeof(VAEncMiscParameterBuffer) + size;
	VAEncMiscParameterBuffer *m = calloc(1, total);
	m->type = type;
	memcpy(m->data, payload, size);
	int rc = make_buffer(e, VAEncMiscParameterBufferType, total, m, id);
	free(m);
	return rc;
}

void va_close(va_enc *e) {
	if (!e) return;
	if (e->dpy) {
		if (e->have_image) p_vaDestroyImage(e->dpy, e->image.image_id);
		if (e->coded != VA_INVALID_ID) p_vaDestroyBuffer(e->dpy, e->coded);
		if (e->ctx != VA_INVALID_ID) p_vaDestroyContext(e->dpy, e->ctx);
		if (e->input != VA_INVALID_SURFACE) p_vaDestroySurfaces(e->dpy, &e->input, 1);
		if (e->recon[0] != VA_INVALID_SURFACE) p_vaDestroySurfaces(e->dpy, e->recon, REFS);
		if (e->config != VA_INVALID_ID) p_vaDestroyConfig(e->dpy, e->config);
		p_vaTerminate(e->dpy);
	}
	if (e->fd >= 0) close(e->fd);
	free(e->out);
	free(e);
}

va_enc *va_open(int w, int h, int fps, int kbps, int gop, int high, int *err) {
	*err = 0;
	if (load() != 0) {
		*err = -1; // no libva
		return NULL;
	}
	va_enc *e = calloc(1, sizeof *e);
	e->fd = -1;
	e->config = e->ctx = e->coded = VA_INVALID_ID;
	e->input = e->recon[0] = e->recon[1] = VA_INVALID_SURFACE;
	e->w = w;
	e->h = h;
	e->wmb = (w + 15) / 16;
	e->hmb = (h + 15) / 16;
	e->fps = e->fps_sent = fps > 0 ? fps : 30;
	e->kbps = kbps;
	e->gop = gop > 0 ? gop : e->fps * 10;
	e->high = high;
	e->level = level_for(e->wmb * e->hmb, e->fps);

	VAProfile profile = high ? VAProfileH264High : VAProfileH264ConstrainedBaseline;
	VAEntrypoint entry = 0;
	// The first render node whose driver encodes this profile.
	for (int i = 128; i < 136 && !e->dpy; i++) {
		char path[32];
		snprintf(path, sizeof path, "/dev/dri/renderD%d", i);
		int fd = open(path, O_RDWR | O_CLOEXEC);
		if (fd < 0) continue;
		VADisplay dpy = p_vaGetDisplayDRM(fd);
		int major, minor;
		if (dpy && p_vaSetInfoCallback) p_vaSetInfoCallback(dpy, quiet, NULL);
		if (dpy && p_vaSetErrorCallback) p_vaSetErrorCallback(dpy, quiet, NULL);
		if (dpy && p_vaInitialize(dpy, &major, &minor) == VA_STATUS_SUCCESS) {
			if (find_entry(dpy, profile, &entry) == 0) {
				e->dpy = dpy;
				e->fd = fd;
				break;
			}
			p_vaTerminate(dpy);
		}
		close(fd);
	}
	if (!e->dpy) {
		*err = -2; // no GPU that encodes H.264
		va_close(e);
		return NULL;
	}
	const char *vendor = p_vaQueryVendorString(e->dpy);
	snprintf(e->driver, sizeof e->driver, "%s", vendor ? vendor : "unknown");

	VAConfigAttrib attrs[2] = {{.type = VAConfigAttribRTFormat}, {.type = VAConfigAttribRateControl}};
	if (p_vaGetConfigAttributes(e->dpy, profile, entry, attrs, 2) != VA_STATUS_SUCCESS ||
	    !(attrs[0].value & VA_RT_FORMAT_YUV420)) {
		*err = -3;
		va_close(e);
		return NULL;
	}
	attrs[0].value = VA_RT_FORMAT_YUV420;
	if (attrs[1].value != VA_ATTRIB_NOT_SUPPORTED && (attrs[1].value & VA_RC_CBR)) attrs[1].value = VA_RC_CBR;
	else if (attrs[1].value != VA_ATTRIB_NOT_SUPPORTED && (attrs[1].value & VA_RC_VBR)) attrs[1].value = VA_RC_VBR;
	else {
		*err = -4; // no bitrate control
		va_close(e);
		return NULL;
	}
	if (p_vaCreateConfig(e->dpy, profile, entry, attrs, 2, &e->config) != VA_STATUS_SUCCESS) {
		*err = -5;
		va_close(e);
		return NULL;
	}
	// Surfaces in whole macroblocks; the SPS crops the picture back.
	int sw = e->wmb * 16, sh = e->hmb * 16;
	if (p_vaCreateSurfaces(e->dpy, VA_RT_FORMAT_YUV420, sw, sh, &e->input, 1, NULL, 0) != VA_STATUS_SUCCESS ||
	    p_vaCreateSurfaces(e->dpy, VA_RT_FORMAT_YUV420, sw, sh, e->recon, REFS, NULL, 0) != VA_STATUS_SUCCESS) {
		e->input = e->recon[0] = VA_INVALID_SURFACE;
		*err = -6;
		va_close(e);
		return NULL;
	}
	VASurfaceID all[REFS + 1] = {e->input, e->recon[0], e->recon[1]};
	if (p_vaCreateContext(e->dpy, e->config, sw, sh, VA_PROGRESSIVE, all, REFS + 1, &e->ctx) != VA_STATUS_SUCCESS) {
		*err = -7;
		va_close(e);
		return NULL;
	}
	// Room for a whole raw frame: a keyframe never comes near it.
	if (make_buffer(e, VAEncCodedBufferType, sw * sh * 3 / 2 + 4096, NULL, &e->coded) != 0) {
		*err = -8;
		va_close(e);
		return NULL;
	}
	e->since_idr = -1; // the first frame is an IDR
	return e;
}

const char *va_driver(va_enc *e) { return e->driver; }

void va_set_rate(va_enc *e, int kbps) {
	if (kbps > 0 && kbps != e->kbps) {
		e->kbps = kbps;
		e->rate_new = 1;
	}
}

// fill writes a packed I420 frame into NV12 at dst (the surface's planes),
// repeating the last row and column into the macroblocks' padding.
static void fill(const VAImage *img, uint8_t *base, const uint8_t *i420, int w, int h) {
	int cw = w / 2, ch = h / 2;
	int pw = img->width, ph = img->height;
	const uint8_t *Y = i420, *U = i420 + (size_t)w * h, *V = U + (size_t)cw * ch;
	uint8_t *dy = base + img->offsets[0], *duv = base + img->offsets[1];
	for (int y = 0; y < ph; y++) {
		const uint8_t *src = Y + (size_t)(y < h ? y : h - 1) * w;
		uint8_t *row = dy + (size_t)y * img->pitches[0];
		memcpy(row, src, w);
		if (pw > w) memset(row + w, src[w - 1], pw - w);
	}
	for (int y = 0; y < ph / 2; y++) {
		int sy = y < ch ? y : ch - 1;
		const uint8_t *u = U + (size_t)sy * cw, *v = V + (size_t)sy * cw;
		uint8_t *row = duv + (size_t)y * img->pitches[1];
		for (int x = 0; x < pw / 2; x++) {
			int sx = x < cw ? x : cw - 1;
			row[2 * x] = u[sx];
			row[2 * x + 1] = v[sx];
		}
	}
}

// upload puts the frame into the input surface: mapped directly where the
// driver allows it (vaDeriveImage), else through an image (vaPutImage).
static int upload(va_enc *e, const uint8_t *i420) {
	VAImage img;
	if (!e->have_image && p_vaDeriveImage(e->dpy, e->input, &img) == VA_STATUS_SUCCESS) {
		void *p = NULL;
		int ok = img.format.fourcc == VA_FOURCC_NV12 && p_vaMapBuffer(e->dpy, img.buf, &p) == VA_STATUS_SUCCESS;
		if (ok) {
			fill(&img, p, i420, e->w, e->h);
			p_vaUnmapBuffer(e->dpy, img.buf);
		}
		p_vaDestroyImage(e->dpy, img.image_id);
		if (ok) return 0;
	}
	if (!e->have_image) {
		VAImageFormat fmt = {.fourcc = VA_FOURCC_NV12, .byte_order = VA_LSB_FIRST, .bits_per_pixel = 12};
		if (p_vaCreateImage(e->dpy, &fmt, e->wmb * 16, e->hmb * 16, &e->image) != VA_STATUS_SUCCESS) return -1;
		e->have_image = 1;
	}
	void *p = NULL;
	if (p_vaMapBuffer(e->dpy, e->image.buf, &p) != VA_STATUS_SUCCESS) return -1;
	fill(&e->image, p, i420, e->w, e->h);
	p_vaUnmapBuffer(e->dpy, e->image.buf);
	if (p_vaPutImage(e->dpy, e->input, e->image.image_id, 0, 0, e->image.width, e->image.height, 0, 0,
	                 e->image.width, e->image.height) != VA_STATUS_SUCCESS)
		return -1;
	return 0;
}

static VAPictureH264 none(void) {
	VAPictureH264 p;
	memset(&p, 0, sizeof p);
	p.picture_id = VA_INVALID_SURFACE;
	p.flags = VA_PICTURE_H264_INVALID;
	return p;
}

int va_encode(va_enc *e, const uint8_t *i420, int key, int fps_now, uint8_t **out, int *n) {
	*out = NULL;
	*n = 0;
	if (upload(e, i420) != 0) return -1;
	int idr = key || e->since_idr < 0 || e->since_idr + 1 >= e->gop;
	if (idr) e->since_idr = 0;
	else e->since_idr++;
	int frame_num = e->since_idr % 256; // log2_max_frame_num = 8
	// The rate control spreads the bitrate over the frame rate: a screen
	// sends a frame when it changes, which is less often. Told again when
	// that changes by a quarter.
	int fps = fps_now > 0 ? fps_now : e->fps;
	if (fps > e->fps) fps = e->fps;
	if (fps < 1) fps = 1;
	int rate_changed = e->rate_new || fps * 4 < e->fps_sent * 3 || fps * 4 > e->fps_sent * 5;

	VABufferID bufs[8];
	int nb = 0, rc = -1;
	if (idr) {
		VAEncSequenceParameterBufferH264 seq;
		memset(&seq, 0, sizeof seq);
		seq.seq_parameter_set_id = 0;
		seq.level_idc = e->level;
		seq.intra_period = e->gop;
		seq.intra_idr_period = e->gop;
		seq.ip_period = 1; // no B frames
		seq.bits_per_second = e->kbps * 1000;
		seq.max_num_ref_frames = 1;
		seq.picture_width_in_mbs = e->wmb;
		seq.picture_height_in_mbs = e->hmb;
		seq.seq_fields.bits.chroma_format_idc = 1;
		seq.seq_fields.bits.frame_mbs_only_flag = 1;
		seq.seq_fields.bits.direct_8x8_inference_flag = 1;
		seq.seq_fields.bits.log2_max_frame_num_minus4 = 4;
		seq.seq_fields.bits.pic_order_cnt_type = 2;
		if (e->wmb * 16 != e->w || e->hmb * 16 != e->h) {
			seq.frame_cropping_flag = 1;
			seq.frame_crop_right_offset = (e->wmb * 16 - e->w) / 2;
			seq.frame_crop_bottom_offset = (e->hmb * 16 - e->h) / 2;
		}
		seq.vui_parameters_present_flag = 1;
		seq.vui_fields.bits.timing_info_present_flag = 1;
		seq.vui_fields.bits.bitstream_restriction_flag = 1;
		seq.vui_fields.bits.log2_max_mv_length_horizontal = 15;
		seq.vui_fields.bits.log2_max_mv_length_vertical = 15;
		seq.num_units_in_tick = 1;
		seq.time_scale = 2 * e->fps;
		if (make_buffer(e, VAEncSequenceParameterBufferType, sizeof seq, &seq, &bufs[nb]) != 0) goto done;
		nb++;
		rate_changed = 1; // with every IDR
	}
	if (rate_changed) {
		VAEncMiscParameterRateControl rcp;
		memset(&rcp, 0, sizeof rcp);
		rcp.bits_per_second = e->kbps * 1000;
		rcp.target_percentage = 100;
		rcp.window_size = 1000; // ms, as VP8's buffer
		rcp.initial_qp = 26;
		rcp.rc_flags.bits.reset = !idr; // a new bitrate in the middle of a GOP
		if (misc(e, VAEncMiscParameterTypeRateControl, &rcp, sizeof rcp, &bufs[nb]) != 0) goto done;
		nb++;
		VAEncMiscParameterHRD hrd;
		memset(&hrd, 0, sizeof hrd);
		hrd.buffer_size = e->kbps * 1000; // a second
		hrd.initial_buffer_fullness = hrd.buffer_size / 2;
		if (misc(e, VAEncMiscParameterTypeHRD, &hrd, sizeof hrd, &bufs[nb]) != 0) goto done;
		nb++;
		VAEncMiscParameterFrameRate fr;
		memset(&fr, 0, sizeof fr);
		fr.framerate = fps; // numerator only: the denominator (high 16 bits) 0 is 1
		if (misc(e, VAEncMiscParameterTypeFrameRate, &fr, sizeof fr, &bufs[nb]) != 0) goto done;
		nb++;
		e->rate_new = 0;
		e->fps_sent = fps;
	}

	VAPictureH264 ref = none();
	if (!idr) {
		ref.picture_id = e->recon[!e->cur];
		ref.frame_idx = (frame_num + 255) % 256;
		ref.flags = VA_PICTURE_H264_SHORT_TERM_REFERENCE;
		ref.TopFieldOrderCnt = ref.BottomFieldOrderCnt = 2 * (e->since_idr - 1);
	}
	VAEncPictureParameterBufferH264 pic;
	memset(&pic, 0, sizeof pic);
	pic.CurrPic.picture_id = e->recon[e->cur];
	pic.CurrPic.frame_idx = frame_num;
	pic.CurrPic.TopFieldOrderCnt = pic.CurrPic.BottomFieldOrderCnt = 2 * e->since_idr;
	for (int i = 0; i < 16; i++) pic.ReferenceFrames[i] = none();
	if (!idr) pic.ReferenceFrames[0] = ref;
	pic.coded_buf = e->coded;
	pic.frame_num = frame_num;
	pic.pic_init_qp = 26;
	pic.pic_fields.bits.idr_pic_flag = idr;
	pic.pic_fields.bits.reference_pic_flag = 1;
	pic.pic_fields.bits.entropy_coding_mode_flag = e->high; // CABAC in High
	pic.pic_fields.bits.transform_8x8_mode_flag = e->high;
	pic.pic_fields.bits.deblocking_filter_control_present_flag = 1;
	if (make_buffer(e, VAEncPictureParameterBufferType, sizeof pic, &pic, &bufs[nb]) != 0) goto done;
	nb++;

	VAEncSliceParameterBufferH264 *slice = calloc(1, sizeof *slice);
	slice->macroblock_address = 0;
	slice->num_macroblocks = e->wmb * e->hmb;
	slice->macroblock_info = VA_INVALID_ID;
	slice->slice_type = idr ? 2 : 0; // I or P
	slice->pic_parameter_set_id = 0;
	slice->idr_pic_id = e->idr_id;
	for (int i = 0; i < 32; i++) slice->RefPicList0[i] = slice->RefPicList1[i] = none();
	if (!idr) slice->RefPicList0[0] = ref;
	int sliced = make_buffer(e, VAEncSliceParameterBufferType, sizeof *slice, slice, &bufs[nb]);
	free(slice);
	if (sliced != 0) goto done;
	nb++;

	if (p_vaBeginPicture(e->dpy, e->ctx, e->input) != VA_STATUS_SUCCESS) goto done;
	if (p_vaRenderPicture(e->dpy, e->ctx, bufs, nb) != VA_STATUS_SUCCESS) {
		p_vaEndPicture(e->dpy, e->ctx);
		goto done;
	}
	if (p_vaEndPicture(e->dpy, e->ctx) != VA_STATUS_SUCCESS) goto done;
	if (p_vaSyncSurface(e->dpy, e->input) != VA_STATUS_SUCCESS) goto done;

	// The access unit: the coded buffer's segments, one after the other.
	VACodedBufferSegment *seg = NULL;
	if (p_vaMapBuffer(e->dpy, e->coded, (void **)&seg) != VA_STATUS_SUCCESS) goto done;
	size_t len = 0;
	for (VACodedBufferSegment *s = seg; s; s = s->next) len += s->size;
	if (len > e->out_cap) {
		uint8_t *grown = realloc(e->out, len);
		if (!grown) {
			p_vaUnmapBuffer(e->dpy, e->coded);
			goto done;
		}
		e->out = grown;
		e->out_cap = len;
	}
	len = 0;
	for (VACodedBufferSegment *s = seg; s; s = s->next) {
		memcpy(e->out + len, s->buf, s->size);
		len += s->size;
	}
	p_vaUnmapBuffer(e->dpy, e->coded);
	*out = e->out;
	*n = (int)len;
	if (idr) e->idr_id = (e->idr_id + 1) & 0xffff;
	e->cur = !e->cur; // this picture is the next one's reference
	rc = 0;
done:
	for (int i = 0; i < nb; i++) p_vaDestroyBuffer(e->dpy, bufs[i]);
	return rc;
}
