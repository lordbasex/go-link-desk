// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

#import <Foundation/Foundation.h>
#import <CoreMedia/CoreMedia.h>
#import <CoreVideo/CoreVideo.h>
#import <VideoToolbox/VideoToolbox.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

extern void goDecoded(uintptr_t handle, uint8_t *bgra, int w, int h, int stride);

typedef struct {
	uintptr_t handle;
	VTDecompressionSessionRef session;
	CMVideoFormatDescriptionRef format;
	uint8_t *sps, *pps;
	int spsLen, ppsLen;
} vdec;

static void on_frame(void *opaque, void *frameRef, OSStatus status, VTDecodeInfoFlags flags, CVImageBufferRef img, CMTime pts, CMTime dur) {
	vdec *d = (vdec *)opaque;
	if (status != noErr || !img)
		return;
	CVPixelBufferLockBaseAddress(img, kCVPixelBufferLock_ReadOnly);
	goDecoded(d->handle, CVPixelBufferGetBaseAddress(img), (int)CVPixelBufferGetWidth(img), (int)CVPixelBufferGetHeight(img), (int)CVPixelBufferGetBytesPerRow(img));
	CVPixelBufferUnlockBaseAddress(img, kCVPixelBufferLock_ReadOnly);
}

void *vdec_new(uintptr_t handle) {
	vdec *d = calloc(1, sizeof *d);
	d->handle = handle;
	return d;
}

static void drop_session(vdec *d) {
	if (d->session) {
		VTDecompressionSessionWaitForAsynchronousFrames(d->session);
		VTDecompressionSessionInvalidate(d->session);
		CFRelease(d->session);
		d->session = NULL;
	}
	if (d->format) {
		CFRelease(d->format);
		d->format = NULL;
	}
}

// open_session (re)creates the session when the parameter sets change.
static int open_session(vdec *d, const uint8_t *sps, int spsLen, const uint8_t *pps, int ppsLen) {
	if (d->session && d->spsLen == spsLen && d->ppsLen == ppsLen && memcmp(d->sps, sps, spsLen) == 0 && memcmp(d->pps, pps, ppsLen) == 0)
		return 0;
	drop_session(d);
	const uint8_t *sets[2] = {sps, pps};
	const size_t sizes[2] = {(size_t)spsLen, (size_t)ppsLen};
	if (CMVideoFormatDescriptionCreateFromH264ParameterSets(kCFAllocatorDefault, 2, sets, sizes, 4, &d->format) != noErr)
		return -1;
	NSDictionary *out = @{(id)kCVPixelBufferPixelFormatTypeKey : @(kCVPixelFormatType_32BGRA)};
	VTDecompressionOutputCallbackRecord cb = {on_frame, d};
	if (VTDecompressionSessionCreate(kCFAllocatorDefault, d->format, NULL, (__bridge CFDictionaryRef)out, &cb, &d->session) != noErr) {
		drop_session(d);
		return -1;
	}
	free(d->sps);
	free(d->pps);
	d->sps = malloc(spsLen);
	d->pps = malloc(ppsLen);
	memcpy(d->sps, sps, spsLen);
	memcpy(d->pps, pps, ppsLen);
	d->spsLen = spsLen;
	d->ppsLen = ppsLen;
	return 0;
}

int vdec_decode(void *ref, const uint8_t *sps, int spsLen, const uint8_t *pps, int ppsLen, const uint8_t *avcc, int len, int64_t ptsMicros) {
	vdec *d = (vdec *)ref;
	if (open_session(d, sps, spsLen, pps, ppsLen) != 0)
		return -1;
	CMBlockBufferRef block = NULL;
	if (CMBlockBufferCreateWithMemoryBlock(kCFAllocatorDefault, NULL, len, kCFAllocatorDefault, NULL, 0, len, 0, &block) != noErr)
		return -1;
	CMBlockBufferReplaceDataBytes(avcc, block, 0, len);
	CMSampleBufferRef sample = NULL;
	CMSampleTimingInfo timing = {kCMTimeInvalid, CMTimeMake(ptsMicros, 1000000), kCMTimeInvalid};
	size_t size = (size_t)len;
	OSStatus st = CMSampleBufferCreateReady(kCFAllocatorDefault, block, d->format, 1, 1, &timing, 1, &size, &sample);
	CFRelease(block);
	if (st != noErr)
		return -1;
	// Synchronous: the picture is handed over before this returns.
	st = VTDecompressionSessionDecodeFrame(d->session, sample, 0, NULL, NULL);
	CFRelease(sample);
	return st == noErr ? 0 : -1;
}

void vdec_free(void *ref) {
	vdec *d = (vdec *)ref;
	drop_session(d);
	free(d->sps);
	free(d->pps);
	free(d);
}
