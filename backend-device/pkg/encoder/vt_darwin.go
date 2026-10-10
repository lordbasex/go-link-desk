// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

package encoder

/*
#cgo LDFLAGS: -framework VideoToolbox -framework CoreMedia -framework CoreVideo -framework CoreFoundation
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <VideoToolbox/VideoToolbox.h>

extern void goVTFrame(uintptr_t handle, void *data, int len);

typedef struct {
	VTCompressionSessionRef session;
	int w, h, fps;
	int64_t n;
	uintptr_t handle;
} vt_enc;

static void put_start(uint8_t **p) { (*p)[0] = 0; (*p)[1] = 0; (*p)[2] = 0; (*p)[3] = 1; *p += 4; }

// Each encoded frame as one Annex B access unit: an access unit delimiter,
// the SPS and PPS before a keyframe, then the frame's NAL units (their
// 4-byte lengths turned into start codes).
static void vt_out(void *refcon, void *src, OSStatus status, VTEncodeInfoFlags flags, CMSampleBufferRef sb) {
	vt_enc *e = (vt_enc *)refcon;
	if (status != noErr || !sb || !CMSampleBufferDataIsReady(sb))
		return;
	int key = 1;
	CFArrayRef att = CMSampleBufferGetSampleAttachmentsArray(sb, false);
	if (att && CFArrayGetCount(att) > 0) {
		CFDictionaryRef d = (CFDictionaryRef)CFArrayGetValueAtIndex(att, 0);
		key = !CFDictionaryContainsKey(d, kCMSampleAttachmentKey_NotSync);
	}
	CMBlockBufferRef bb = CMSampleBufferGetDataBuffer(sb);
	size_t total = 0;
	char *data = NULL;
	if (!bb || CMBlockBufferGetDataPointer(bb, 0, NULL, &total, &data) != kCMBlockBufferNoErr)
		return;
	const uint8_t *sps = NULL, *pps = NULL;
	size_t sps_n = 0, pps_n = 0;
	if (key) {
		CMFormatDescriptionRef fd = CMSampleBufferGetFormatDescription(sb);
		CMVideoFormatDescriptionGetH264ParameterSetAtIndex(fd, 0, &sps, &sps_n, NULL, NULL);
		CMVideoFormatDescriptionGetH264ParameterSetAtIndex(fd, 1, &pps, &pps_n, NULL, NULL);
	}
	uint8_t *out = (uint8_t *)malloc(6 + 8 + sps_n + pps_n + total + 64);
	if (!out)
		return;
	uint8_t *p = out;
	put_start(&p);
	*p++ = 0x09; // access unit delimiter
	*p++ = 0xf0;
	if (sps && pps) {
		put_start(&p);
		memcpy(p, sps, sps_n);
		p += sps_n;
		put_start(&p);
		memcpy(p, pps, pps_n);
		p += pps_n;
	}
	size_t at = 0;
	while (at + 4 <= total) {
		uint32_t len = ((uint32_t)(uint8_t)data[at] << 24) | ((uint32_t)(uint8_t)data[at + 1] << 16) | ((uint32_t)(uint8_t)data[at + 2] << 8) | (uint8_t)data[at + 3];
		at += 4;
		if (len > total - at)
			break;
		put_start(&p);
		memcpy(p, data + at, len);
		p += len;
		at += len;
	}
	goVTFrame(e->handle, out, (int)(p - out));
	free(out);
}

static void set_int(VTCompressionSessionRef s, CFStringRef key, int64_t v) {
	CFNumberRef n = CFNumberCreate(NULL, kCFNumberSInt64Type, &v);
	VTSessionSetProperty(s, key, n);
	CFRelease(n);
}

static vt_enc *vt_open(int w, int h, int fps, int kbps, int gop, uintptr_t handle, int *err) {
	vt_enc *e = (vt_enc *)calloc(1, sizeof *e);
	if (!e) {
		*err = -1;
		return NULL;
	}
	e->w = w;
	e->h = h;
	e->fps = fps;
	e->handle = handle;
	// the hardware encoder when there is one (every Mac since 2011, Apple silicon's media engine)
	const void *sk[] = { kVTVideoEncoderSpecification_EnableHardwareAcceleratedVideoEncoder };
	const void *sv[] = { kCFBooleanTrue };
	CFDictionaryRef spec = CFDictionaryCreate(NULL, sk, sv, 1, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	int32_t fmt = kCVPixelFormatType_420YpCbCr8BiPlanarVideoRange; // NV12, what the hardware takes
	CFNumberRef fmtn = CFNumberCreate(NULL, kCFNumberSInt32Type, &fmt);
	const void *bk[] = { kCVPixelBufferPixelFormatTypeKey };
	const void *bv[] = { fmtn };
	CFDictionaryRef src = CFDictionaryCreate(NULL, bk, bv, 1, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	OSStatus st = VTCompressionSessionCreate(NULL, w, h, kCMVideoCodecType_H264, spec, src, NULL, vt_out, e, &e->session);
	CFRelease(spec);
	CFRelease(src);
	CFRelease(fmtn);
	if (st != noErr) {
		free(e);
		*err = (int)st;
		return NULL;
	}
	VTSessionSetProperty(e->session, kVTCompressionPropertyKey_RealTime, kCFBooleanTrue);
	VTSessionSetProperty(e->session, kVTCompressionPropertyKey_ProfileLevel, kVTProfileLevel_H264_Baseline_AutoLevel);
	VTSessionSetProperty(e->session, kVTCompressionPropertyKey_AllowFrameReordering, kCFBooleanFalse);
	set_int(e->session, kVTCompressionPropertyKey_AverageBitRate, (int64_t)kbps * 1000);
	set_int(e->session, kVTCompressionPropertyKey_ExpectedFrameRate, fps);
	set_int(e->session, kVTCompressionPropertyKey_MaxKeyFrameInterval, gop);
	{
		// no second's bytes over 1.5 times the average: a steady stream for WebRTC
		int64_t bytes = (int64_t)kbps * 1000 / 8 * 3 / 2, one = 1;
		CFNumberRef b = CFNumberCreate(NULL, kCFNumberSInt64Type, &bytes), o = CFNumberCreate(NULL, kCFNumberSInt64Type, &one);
		const void *lim[] = { b, o };
		CFArrayRef a = CFArrayCreate(NULL, lim, 2, &kCFTypeArrayCallBacks);
		VTSessionSetProperty(e->session, kVTCompressionPropertyKey_DataRateLimits, a);
		CFRelease(a);
		CFRelease(b);
		CFRelease(o);
	}
	VTCompressionSessionPrepareToEncodeFrames(e->session);
	*err = 0;
	return e;
}

// One I420 frame in (copied into an NV12 buffer of the session's pool).
static int vt_encode(vt_enc *e, const uint8_t *i420, int key) {
	CVPixelBufferRef pb = NULL;
	CVPixelBufferPoolRef pool = VTCompressionSessionGetPixelBufferPool(e->session);
	if (!pool || CVPixelBufferPoolCreatePixelBuffer(NULL, pool, &pb) != kCVReturnSuccess)
		return -1;
	CVPixelBufferLockBaseAddress(pb, 0);
	int w = e->w, h = e->h, x, y;
	uint8_t *dy = (uint8_t *)CVPixelBufferGetBaseAddressOfPlane(pb, 0);
	size_t sy = CVPixelBufferGetBytesPerRowOfPlane(pb, 0);
	for (y = 0; y < h; y++)
		memcpy(dy + y * sy, i420 + (size_t)y * w, (size_t)w);
	uint8_t *duv = (uint8_t *)CVPixelBufferGetBaseAddressOfPlane(pb, 1);
	size_t suv = CVPixelBufferGetBytesPerRowOfPlane(pb, 1);
	const uint8_t *u = i420 + (size_t)w * h, *v = u + (size_t)(w / 2) * (h / 2);
	for (y = 0; y < h / 2; y++) {
		uint8_t *row = duv + y * suv;
		const uint8_t *ur = u + (size_t)y * (w / 2), *vr = v + (size_t)y * (w / 2);
		for (x = 0; x < w / 2; x++) {
			row[2 * x] = ur[x];
			row[2 * x + 1] = vr[x];
		}
	}
	CVPixelBufferUnlockBaseAddress(pb, 0);
	CFDictionaryRef props = NULL;
	if (key) {
		const void *k[] = { kVTEncodeFrameOptionKey_ForceKeyFrame };
		const void *val[] = { kCFBooleanTrue };
		props = CFDictionaryCreate(NULL, k, val, 1, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	}
	OSStatus st = VTCompressionSessionEncodeFrame(e->session, pb, CMTimeMake(e->n++, e->fps), kCMTimeInvalid, props, NULL, NULL);
	if (props)
		CFRelease(props);
	CVPixelBufferRelease(pb);
	return st == noErr ? 0 : (int)st;
}

static void vt_close(vt_enc *e) {
	VTCompressionSessionCompleteFrames(e->session, kCMTimeInvalid);
	VTCompressionSessionInvalidate(e->session);
	CFRelease(e->session);
	free(e);
}
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime/cgo"
	"sync"
	"unsafe"
)

// vtEncoder is H.264 from VideoToolbox, macOS's own encoder (the GPU's or
// the media engine's hardware), called directly: nothing to install, and
// the encoder's licensing comes with the system.
type vtEncoder struct {
	mu      sync.Mutex
	e       *C.vt_enc
	handle  cgo.Handle
	frameSz int
}

//export goVTFrame
func goVTFrame(handle C.uintptr_t, data unsafe.Pointer, n C.int) {
	onFrame := cgo.Handle(handle).Value().(func([]byte))
	onFrame(C.GoBytes(data, n))
}

// newVT starts a VideoToolbox session; onFrame gets each access unit (Annex
// B, with an access unit delimiter, and SPS and PPS before each keyframe)
// from VideoToolbox's own thread.
func newVT(cfg Config, gop int, onFrame func([]byte)) (*vtEncoder, error) {
	h := cgo.NewHandle(onFrame)
	var cerr C.int
	e := C.vt_open(C.int(cfg.Width), C.int(cfg.Height), C.int(cfg.FPS), C.int(cfg.BitrateKbps), C.int(gop), C.uintptr_t(h), &cerr)
	if e == nil {
		h.Delete()
		return nil, fmt.Errorf("h264: VideoToolbox session failed (%d)", int(cerr))
	}
	return &vtEncoder{e: e, handle: h, frameSz: cfg.Width*cfg.Height + 2*(cfg.Width/2)*(cfg.Height/2)}, nil
}

func (v *vtEncoder) write(i420 []byte, key bool) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.e == nil {
		return errors.New("h264: closed")
	}
	if len(i420) < v.frameSz {
		return fmt.Errorf("h264: frame is %d bytes, want %d", len(i420), v.frameSz)
	}
	k := C.int(0)
	if key {
		k = 1
	}
	if st := C.vt_encode(v.e, (*C.uint8_t)(unsafe.Pointer(&i420[0])), k); st != 0 {
		return fmt.Errorf("h264: VideoToolbox encode failed (%d)", int(st))
	}
	return nil
}

func (v *vtEncoder) close() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.e == nil {
		return
	}
	C.vt_close(v.e) // waits for the frames in flight: their callbacks still find the handle
	v.e = nil
	v.handle.Delete()
}

// hasVT is whether this build can call VideoToolbox directly.
const hasVT = true
