// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package decoder

/*
#cgo pkg-config: vpx
#include <stdlib.h>
#include <string.h>
#include <vpx/vpx_decoder.h>
#include <vpx/vp8dx.h>

typedef struct {
	vpx_codec_ctx_t ctx;
} dec_t;

static int dec_open(dec_t *d, int threads) {
	memset(d, 0, sizeof(*d));
	vpx_codec_dec_cfg_t cfg = {0};
	cfg.threads = threads;
	return vpx_codec_dec_init(&d->ctx, vpx_codec_vp8_dx(), &cfg, 0) == VPX_CODEC_OK ? 0 : -1;
}

// dec_frame decodes one frame and returns its picture (NULL: none, or an
// error in *err).
static vpx_image_t *dec_frame(dec_t *d, const uint8_t *data, int n, int *err) {
	*err = 0;
	if (vpx_codec_decode(&d->ctx, data, n, NULL, 0) != VPX_CODEC_OK) {
		*err = 1;
		return NULL;
	}
	vpx_codec_iter_t it = NULL;
	vpx_image_t *last = NULL, *img;
	while ((img = vpx_codec_get_frame(&d->ctx, &it)) != NULL) last = img;
	return last;
}

static int dec_corrupt(dec_t *d) {
	int corrupt = 0;
	vpx_codec_control(&d->ctx, VP8D_GET_FRAME_CORRUPTED, &corrupt);
	return corrupt;
}

static void dec_close(dec_t *d) { vpx_codec_destroy(&d->ctx); }
*/
import "C"

import (
	"errors"
	"sync"
	"unsafe"
)

// VP8 decodes a VP8 stream (libvpx), what a computer without an H.264
// encoder sends.
type VP8 struct {
	mu sync.Mutex
	d  *C.dec_t
}

// NewVP8 opens a decoder with the given number of threads.
func NewVP8(threads int) (*VP8, error) {
	d := (*C.dec_t)(C.calloc(1, C.sizeof_dec_t))
	if C.dec_open(d, C.int(max(1, threads))) != 0 {
		C.free(unsafe.Pointer(d))
		return nil, errors.New("decoder: libvpx has no VP8 decoder")
	}
	return &VP8{d: d}, nil
}

// Decode decodes one frame. It returns no picture and no error for a
// frame libvpx shows nothing for, and ErrCorrupt (with the picture) when
// the frame built on a lost one.
func (v *VP8) Decode(frame []byte) (*I420, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.d == nil {
		return nil, errors.New("decoder: closed")
	}
	if len(frame) == 0 {
		return nil, nil
	}
	var cerr C.int
	img := C.dec_frame(v.d, (*C.uint8_t)(unsafe.Pointer(&frame[0])), C.int(len(frame)), &cerr)
	if cerr != 0 {
		return nil, errors.New("decoder: vp8 frame not decoded")
	}
	if img == nil {
		return nil, nil
	}
	if img.fmt != C.VPX_IMG_FMT_I420 {
		return nil, errors.New("decoder: vp8 picture is not I420")
	}
	w, h := int(img.d_w), int(img.d_h)
	sy, suv := int(img.stride[0]), int(img.stride[1])
	ch := (h + 1) / 2
	p := &I420{
		Width: w, Height: h, StrideY: sy, StrideUV: suv,
		Y: unsafe.Slice((*byte)(unsafe.Pointer(img.planes[0])), sy*h),
		U: unsafe.Slice((*byte)(unsafe.Pointer(img.planes[1])), suv*ch),
		V: unsafe.Slice((*byte)(unsafe.Pointer(img.planes[2])), suv*ch),
	}
	if C.dec_corrupt(v.d) != 0 {
		return p, ErrCorrupt
	}
	return p, nil
}

// Close frees the decoder.
func (v *VP8) Close() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.d == nil {
		return
	}
	C.dec_close(v.d)
	C.free(unsafe.Pointer(v.d))
	v.d = nil
}
