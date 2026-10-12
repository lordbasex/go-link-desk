// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package encoder compresses raw I420 frames into VP8 with libvpx (cgo).
// It is tuned for real-time screen streaming: no look-ahead, constant
// bitrate with WebRTC's short buffers, libvpx's screen content mode, the
// frames' own times, and keyframes on demand.
package encoder

/*
#cgo pkg-config: vpx
#include <stdlib.h>
#include <string.h>
#include <vpx/vpx_encoder.h>
#include <vpx/vp8cx.h>

typedef struct {
	vpx_codec_ctx_t ctx;
	vpx_codec_enc_cfg_t cfg;
	vpx_image_t img;
	uint8_t *buf;
	size_t cap;
	size_t len;
	int keyframe;
} enc_t;

// Frames carry their times in a 90 kHz clock (RTP's): libvpx gives each
// one the bits of the time since the previous, and picks its speed by
// that time. A screen changes far less often than the frame rate.
#define TIMEBASE 90000

static int enc_open(enc_t *e, int w, int h, int fps, int kbps, int minq, int maxq, int cpu, int threads, int screen) {
	memset(e, 0, sizeof(*e));
	vpx_codec_enc_cfg_t *cfg = &e->cfg;
	if (vpx_codec_enc_config_default(vpx_codec_vp8_cx(), cfg, 0) != VPX_CODEC_OK) return -1;
	cfg->g_w = w;
	cfg->g_h = h;
	cfg->g_timebase.num = 1;
	cfg->g_timebase.den = TIMEBASE;
	cfg->rc_target_bitrate = kbps;
	cfg->rc_end_usage = VPX_CBR;
	cfg->rc_min_quantizer = minq;
	cfg->rc_max_quantizer = maxq;
	// WebRTC's buffer (ms): libvpx's own (6 s) lets a busy screen send
	// bursts that arrive late.
	cfg->rc_buf_sz = 1000;
	cfg->rc_buf_initial_sz = 500;
	cfg->rc_buf_optimal_sz = 600;
	cfg->rc_undershoot_pct = 100;
	cfg->rc_overshoot_pct = 15;
	cfg->g_lag_in_frames = 0;
	cfg->g_error_resilient = VPX_ERROR_RESILIENT_DEFAULT;
	cfg->g_threads = threads;
	cfg->kf_mode = VPX_KF_AUTO;
	cfg->kf_max_dist = fps * 3;
	if (vpx_codec_enc_init(&e->ctx, vpx_codec_vp8_cx(), cfg, 0) != VPX_CODEC_OK) return -2;
	vpx_codec_control(&e->ctx, VP8E_SET_CPUUSED, cpu);
	vpx_codec_control(&e->ctx, VP8E_SET_STATIC_THRESHOLD, 1);
	// Screen content: sharper text and still areas left alone. Not mode
	// 2, which drops frames: a screen sends one only when it changes, so
	// a dropped one would leave the viewer with an old picture.
	if (screen) vpx_codec_control(&e->ctx, VP8E_SET_SCREEN_CONTENT_MODE, 1);
	// more threads need token partitions to share the work (HD, T-31)
	if (threads > 2) vpx_codec_control(&e->ctx, VP8E_SET_TOKEN_PARTITIONS, threads >= 8 ? 3 : threads >= 4 ? 2 : 1);
	if (!vpx_img_alloc(&e->img, VPX_IMG_FMT_I420, w, h, 1)) {
		vpx_codec_destroy(&e->ctx);
		return -3;
	}
	return 0;
}

// enc_set_bitrate changes the bitrate as the encoder runs (no keyframe).
static int enc_set_bitrate(enc_t *e, int kbps) {
	e->cfg.rc_target_bitrate = kbps;
	return vpx_codec_enc_config_set(&e->ctx, &e->cfg) == VPX_CODEC_OK ? 0 : -1;
}

static void copy_plane(uint8_t *dst, int dst_stride, const uint8_t *src, int w, int h) {
	for (int y = 0; y < h; y++) memcpy(dst + y * dst_stride, src + y * w, w);
}

// enc_encode encodes one packed I420 frame. The compressed frame is left
// in e->buf / e->len.
static int enc_encode(enc_t *e, const uint8_t *frame, int w, int h, int64_t pts, unsigned long dur, int force_kf) {
	int cw = (w + 1) / 2, ch = (h + 1) / 2;
	copy_plane(e->img.planes[VPX_PLANE_Y], e->img.stride[VPX_PLANE_Y], frame, w, h);
	copy_plane(e->img.planes[VPX_PLANE_U], e->img.stride[VPX_PLANE_U], frame + w * h, cw, ch);
	copy_plane(e->img.planes[VPX_PLANE_V], e->img.stride[VPX_PLANE_V], frame + w * h + cw * ch, cw, ch);
	vpx_enc_frame_flags_t flags = force_kf ? VPX_EFLAG_FORCE_KF : 0;
	if (vpx_codec_encode(&e->ctx, &e->img, pts, dur, flags, VPX_DL_REALTIME) != VPX_CODEC_OK) return -1;
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
	"runtime"
	"time"
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
	// Screen turns on libvpx's screen content mode (a desktop, not a
	// camera or a game).
	Screen bool
	// GOPFrames is H264's keyframe interval (0: two seconds); a viewer
	// switched onto a stream starts at its next keyframe.
	GOPFrames int
	// H.264 tuning, zero values are the streaming defaults: Constrained
	// High profile, the low-latency rate control, no quantizer cap.
	// Baseline and NoLowLatency go back to the plainer settings; MaxQP
	// caps the quantizer (1-51, lower keeps text sharper but costs bits).
	Baseline     bool
	NoLowLatency bool
	MaxQP        int
}

// timebase is the frames' clock (Hz), the C side's TIMEBASE.
const timebase = 90000

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
	pts    int64 // the last frame's, in TIMEBASE ticks
	shift  int64 // added to the source's times (they started again)
	have   bool
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
	// No more than the computer's cores: more only take turns.
	threads = max(1, min(threads, runtime.NumCPU()))
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
	screen := C.int(0)
	if cfg.Screen {
		screen = 1
	}
	if rc := C.enc_open(e, C.int(cfg.Width), C.int(cfg.Height), C.int(cfg.FPS), C.int(cfg.BitrateKbps), C.int(minq), C.int(maxq), C.int(cpu), C.int(threads), screen); rc != 0 {
		C.free(unsafe.Pointer(e))
		return nil, fmt.Errorf("encoder: libvpx init failed (%d)", int(rc))
	}
	return &VP8{cfg: cfg, enc: e}, nil
}

// Encode compresses one packed I420 frame, a frame period after the
// previous one. forceKeyframe makes the frame decodable on its own, which
// a viewer needs to start or recover.
func (v *VP8) Encode(frame []byte, forceKeyframe bool) (data []byte, keyframe bool, err error) {
	return v.EncodeAt(frame, -1, forceKeyframe)
}

// EncodeAt is Encode for a frame at a time on its source's clock (a
// screen's frames come when it changes); a negative time is a frame
// period after the previous frame.
func (v *VP8) EncodeAt(frame []byte, at time.Duration, forceKeyframe bool) (data []byte, keyframe bool, err error) {
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
	period := int64(timebase / v.cfg.FPS)
	pts := v.pts + period
	if at >= 0 {
		pts = int64(at)*timebase/int64(time.Second) + v.shift
		// libvpx needs times that go forward: a source that starts its
		// clock again goes on from here, a frame period later.
		if v.have && pts <= v.pts {
			v.shift += v.pts + period - pts
			pts = v.pts + period
		}
	}
	// The frame lasts until the next one, unknown yet: a frame period.
	rc := C.enc_encode(v.enc, (*C.uint8_t)(unsafe.Pointer(&frame[0])), C.int(v.cfg.Width), C.int(v.cfg.Height), C.int64_t(pts), C.ulong(period), force)
	v.pts, v.have = pts, true
	if rc != 0 {
		return nil, false, fmt.Errorf("encoder: encode failed (%d)", int(rc))
	}
	if v.enc.len == 0 {
		return nil, false, nil // the rate control dropped this frame
	}
	return C.GoBytes(unsafe.Pointer(v.enc.buf), C.int(v.enc.len)), v.enc.keyframe != 0, nil
}

// SetBitrate changes the bitrate without starting the stream again.
func (v *VP8) SetBitrate(kbps int) error {
	if v.closed {
		return errors.New("encoder: closed")
	}
	if kbps <= 0 {
		return fmt.Errorf("encoder: invalid bitrate %d", kbps)
	}
	if C.enc_set_bitrate(v.enc, C.int(kbps)) != 0 {
		return errors.New("encoder: libvpx refused the bitrate")
	}
	v.cfg.BitrateKbps = kbps
	return nil
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
