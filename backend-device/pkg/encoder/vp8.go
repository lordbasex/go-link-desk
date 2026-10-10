// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package encoder compresses raw I420 frames into VP8 with libvpx (cgo).
// It is tuned for real-time streaming: no look-ahead, constant bitrate,
// fastest speed preset, and keyframes on demand.
package encoder

/*
#cgo pkg-config: vpx
#include <stdlib.h>
#include <string.h>
#include <vpx/vpx_encoder.h>
#include <vpx/vp8cx.h>

typedef struct {
	vpx_codec_ctx_t ctx;
	vpx_image_t img;
	uint8_t *buf;
	size_t cap;
	size_t len;
	int keyframe;
} enc_t;

static int enc_open(enc_t *e, int w, int h, int fps, int kbps, int minq, int maxq, int cpu, int threads) {
	vpx_codec_enc_cfg_t cfg;
	memset(e, 0, sizeof(*e));
	if (vpx_codec_enc_config_default(vpx_codec_vp8_cx(), &cfg, 0) != VPX_CODEC_OK) return -1;
	cfg.g_w = w;
	cfg.g_h = h;
	cfg.g_timebase.num = 1;
	cfg.g_timebase.den = fps;
	cfg.rc_target_bitrate = kbps;
	cfg.rc_end_usage = VPX_CBR;
	cfg.rc_min_quantizer = minq;
	cfg.rc_max_quantizer = maxq;
	cfg.g_lag_in_frames = 0;
	cfg.g_error_resilient = VPX_ERROR_RESILIENT_DEFAULT;
	cfg.g_threads = threads;
	cfg.kf_mode = VPX_KF_AUTO;
	cfg.kf_max_dist = fps * 3;
	if (vpx_codec_enc_init(&e->ctx, vpx_codec_vp8_cx(), &cfg, 0) != VPX_CODEC_OK) return -2;
	vpx_codec_control(&e->ctx, VP8E_SET_CPUUSED, cpu);
	vpx_codec_control(&e->ctx, VP8E_SET_STATIC_THRESHOLD, 1);
	// more threads need token partitions to share the work (HD, T-31)
	if (threads > 2) vpx_codec_control(&e->ctx, VP8E_SET_TOKEN_PARTITIONS, threads >= 8 ? 3 : threads >= 4 ? 2 : 1);
	if (!vpx_img_alloc(&e->img, VPX_IMG_FMT_I420, w, h, 1)) {
		vpx_codec_destroy(&e->ctx);
		return -3;
	}
	return 0;
}

static void copy_plane(uint8_t *dst, int dst_stride, const uint8_t *src, int w, int h) {
	for (int y = 0; y < h; y++) memcpy(dst + y * dst_stride, src + y * w, w);
}

// enc_encode encodes one packed I420 frame. The compressed frame is left
// in e->buf / e->len.
static int enc_encode(enc_t *e, const uint8_t *frame, int w, int h, int64_t pts, int force_kf) {
	int cw = (w + 1) / 2, ch = (h + 1) / 2;
	copy_plane(e->img.planes[VPX_PLANE_Y], e->img.stride[VPX_PLANE_Y], frame, w, h);
	copy_plane(e->img.planes[VPX_PLANE_U], e->img.stride[VPX_PLANE_U], frame + w * h, cw, ch);
	copy_plane(e->img.planes[VPX_PLANE_V], e->img.stride[VPX_PLANE_V], frame + w * h + cw * ch, cw, ch);
	vpx_enc_frame_flags_t flags = force_kf ? VPX_EFLAG_FORCE_KF : 0;
	if (vpx_codec_encode(&e->ctx, &e->img, pts, 1, flags, VPX_DL_REALTIME) != VPX_CODEC_OK) return -1;
	e->len = 0;
	e->keyframe = 0;
	vpx_codec_iter_t iter = NULL;
	const vpx_codec_cx_pkt_t *pkt;
	while ((pkt = vpx_codec_get_cx_data(&e->ctx, &iter)) != NULL) {
		if (pkt->kind != VPX_CODEC_CX_FRAME_PKT) continue;
		size_t need = e->len + pkt->data.frame.sz;
		if (need > e->cap) {
			uint8_t *grown = realloc(e->buf, need);
			if (!grown) return -2;
			e->buf = grown;
			e->cap = need;
		}
		memcpy(e->buf + e->len, pkt->data.frame.buf, pkt->data.frame.sz);
		e->len = need;
		if (pkt->data.frame.flags & VPX_FRAME_IS_KEY) e->keyframe = 1;
	}
	return 0;
}

static void enc_close(enc_t *e) {
	vpx_img_free(&e->img);
	vpx_codec_destroy(&e->ctx);
	free(e->buf);
	e->buf = NULL;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// Config describes the video stream.
type Config struct {
	Width, Height int
	FPS           int
	BitrateKbps   int
	// MinQuantizer and MaxQuantizer bound the quantizer (0-63, lower is
	// better quality); CPUUsed is the libvpx speed preset (-16..16, higher
	// is faster). Zero keeps the streaming defaults (4, 56 and 8); the
	// video quality lab sets them to compare other tunings.
	MinQuantizer, MaxQuantizer int
	CPUUsed                    int
	// Threads is libvpx's thread count; zero keeps the streaming default (2).
	// HD sizes need more (go-link HD, experiment T-31).
	Threads int
	// GOPFrames is H264's keyframe interval (0: two seconds); a viewer
	// switched onto a stream starts at its next keyframe.
	GOPFrames int
}

// Encoder defaults, used when Config leaves the tuning fields at zero.
const (
	DefaultMinQuantizer = 4
	DefaultMaxQuantizer = 56
	DefaultCPUUsed      = 8
)

// VP8 is a libvpx VP8 encoder. It is not safe for concurrent use.
type VP8 struct {
	cfg    Config
	enc    *C.enc_t
	pts    int64
	closed bool
}

// FrameSize returns the byte length of a packed I420 frame.
func FrameSize(w, h int) int {
	cw, ch := (w+1)/2, (h+1)/2
	return w*h + 2*cw*ch
}

// NewVP8 opens an encoder.
func NewVP8(cfg Config) (*VP8, error) {
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.FPS <= 0 || cfg.BitrateKbps <= 0 {
		return nil, fmt.Errorf("encoder: invalid config %+v", cfg)
	}
	minq, maxq, cpu := cfg.MinQuantizer, cfg.MaxQuantizer, cfg.CPUUsed
	threads := cfg.Threads
	if threads <= 0 {
		threads = 2
	}
	if minq == 0 {
		minq = DefaultMinQuantizer
	}
	if maxq == 0 {
		maxq = DefaultMaxQuantizer
	}
	if cpu == 0 {
		cpu = DefaultCPUUsed
	}
	if minq < 0 || maxq > 63 || minq > maxq || cpu < -16 || cpu > 16 {
		return nil, fmt.Errorf("encoder: invalid tuning %+v", cfg)
	}
	e := (*C.enc_t)(C.malloc(C.size_t(unsafe.Sizeof(C.enc_t{}))))
	if rc := C.enc_open(e, C.int(cfg.Width), C.int(cfg.Height), C.int(cfg.FPS), C.int(cfg.BitrateKbps), C.int(minq), C.int(maxq), C.int(cpu), C.int(threads)); rc != 0 {
		C.free(unsafe.Pointer(e))
		return nil, fmt.Errorf("encoder: libvpx init failed (%d)", int(rc))
	}
	return &VP8{cfg: cfg, enc: e}, nil
}

// Encode compresses one packed I420 frame. forceKeyframe makes the frame
// decodable on its own, which a viewer needs to start or recover.
func (v *VP8) Encode(frame []byte, forceKeyframe bool) (data []byte, keyframe bool, err error) {
	if v.closed {
		return nil, false, errors.New("encoder: closed")
	}
	if len(frame) != FrameSize(v.cfg.Width, v.cfg.Height) {
		return nil, false, fmt.Errorf("encoder: frame has %d bytes, want %d", len(frame), FrameSize(v.cfg.Width, v.cfg.Height))
	}
	force := C.int(0)
	if forceKeyframe {
		force = 1
	}
	rc := C.enc_encode(v.enc, (*C.uint8_t)(unsafe.Pointer(&frame[0])), C.int(v.cfg.Width), C.int(v.cfg.Height), C.int64_t(v.pts), force)
	v.pts++
	if rc != 0 {
		return nil, false, fmt.Errorf("encoder: encode failed (%d)", int(rc))
	}
	if v.enc.len == 0 {
		return nil, false, nil // the rate control dropped this frame
	}
	return C.GoBytes(unsafe.Pointer(v.enc.buf), C.int(v.enc.len)), v.enc.keyframe != 0, nil
}

// Close releases libvpx resources.
func (v *VP8) Close() {
	if v.closed {
		return
	}
	v.closed = true
	C.enc_close(v.enc)
	C.free(unsafe.Pointer(v.enc))
}
