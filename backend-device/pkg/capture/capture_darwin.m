// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

#import <AppKit/AppKit.h>
#import <CoreGraphics/CoreGraphics.h>
#import <CoreMedia/CoreMedia.h>
#import <CoreVideo/CoreVideo.h>
#import <ScreenCaptureKit/ScreenCaptureKit.h>
#include "capture_darwin.h"

extern void goCaptureFrame(uintptr_t handle, void *pixelBuffer, int width, int height, int64_t ptsMicros);
extern void goCaptureStopped(uintptr_t handle, char *err);

// ---- displays ----

// shareable waits for ScreenCaptureKit's list of what can be captured
// (it asks asynchronously; the callers are Go goroutines, never the main
// thread).
static SCShareableContent *shareable(NSError **err) {
	__block SCShareableContent *content = nil;
	__block NSError *failure = nil;
	dispatch_semaphore_t done = dispatch_semaphore_create(0);
	[SCShareableContent getShareableContentExcludingDesktopWindows:NO onScreenWindowsOnly:YES completionHandler:^(SCShareableContent *c, NSError *e) {
		content = c;
		failure = e;
		dispatch_semaphore_signal(done);
	}];
	if (dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, 5 * NSEC_PER_SEC)) != 0) {
		if (err)
			*err = [NSError errorWithDomain:@"go-link-desk" code:1 userInfo:@{NSLocalizedDescriptionKey : @"timed out listing the displays"}];
		return nil;
	}
	if (err)
		*err = failure;
	return content;
}

static char *copy_error(NSError *e) {
	NSString *s = e ? e.localizedDescription : @"unknown error";
	return strdup(s.UTF8String ? s.UTF8String : "unknown error");
}

int cap_displays(cap_display *out, int max, char **err) {
	@autoreleasepool {
		NSError *e = nil;
		SCShareableContent *content = shareable(&e);
		if (!content) {
			*err = copy_error(e);
			return -1;
		}
		CGDirectDisplayID mainID = CGMainDisplayID();
		int n = 0;
		for (SCDisplay *d in content.displays) {
			if (n >= max)
				break;
			cap_display *o = &out[n++];
			memset(o, 0, sizeof *o);
			o->id = d.displayID;
			o->pointsW = (int)d.width;
			o->pointsH = (int)d.height;
			CGRect b = CGDisplayBounds(d.displayID);
			o->x = (int)b.origin.x;
			o->y = (int)b.origin.y;
			o->primary = d.displayID == mainID;
			CGDisplayModeRef mode = CGDisplayCopyDisplayMode(d.displayID);
			if (mode) {
				o->pixelsW = (int)CGDisplayModeGetPixelWidth(mode);
				o->pixelsH = (int)CGDisplayModeGetPixelHeight(mode);
				CGDisplayModeRelease(mode);
			}
			if (o->pixelsW <= 0) {
				o->pixelsW = o->pointsW;
				o->pixelsH = o->pointsH;
			}
			NSString *name = nil;
			for (NSScreen *s in NSScreen.screens) {
				NSNumber *num = s.deviceDescription[@"NSScreenNumber"];
				if (num && num.unsignedIntValue == d.displayID)
					name = s.localizedName;
			}
			if (!name)
				name = [NSString stringWithFormat:@"Display %u", d.displayID];
			strlcpy(o->name, name.UTF8String ? name.UTF8String : "", sizeof o->name);
		}
		return n;
	}
}

// ---- the stream ----

@interface DeskCapture : NSObject <SCStreamOutput, SCStreamDelegate>
@property(nonatomic) uintptr_t handle;
@property(nonatomic, strong) SCStream *stream;
@property(nonatomic, strong) dispatch_queue_t queue;
@end

@implementation DeskCapture
- (void)stream:(SCStream *)stream didOutputSampleBuffer:(CMSampleBufferRef)sb ofType:(SCStreamOutputType)type {
	if (type != SCStreamOutputTypeScreen || !CMSampleBufferIsValid(sb))
		return;
	// Only complete frames carry a new picture: ScreenCaptureKit also sends
	// "idle" ones while nothing changes, which are not encoded at all.
	CFArrayRef att = CMSampleBufferGetSampleAttachmentsArray(sb, false);
	if (!att || CFArrayGetCount(att) == 0)
		return;
	NSDictionary *info = (__bridge NSDictionary *)CFArrayGetValueAtIndex(att, 0);
	NSNumber *status = info[SCStreamFrameInfoStatus];
	if (!status || status.integerValue != SCFrameStatusComplete)
		return;
	CVPixelBufferRef pb = CMSampleBufferGetImageBuffer(sb);
	if (!pb)
		return;
	CMTime pts = CMSampleBufferGetPresentationTimeStamp(sb);
	int64_t us = CMTIME_IS_VALID(pts) ? (int64_t)(CMTimeGetSeconds(pts) * 1e6) : 0;
	goCaptureFrame(self.handle, (void *)pb, (int)CVPixelBufferGetWidth(pb), (int)CVPixelBufferGetHeight(pb), us);
}

- (void)stream:(SCStream *)stream didStopWithError:(NSError *)error {
	goCaptureStopped(self.handle, copy_error(error));
}
@end

static SCStreamConfiguration *make_config(int w, int h, int fps, int cursor) {
	SCStreamConfiguration *c = [[SCStreamConfiguration alloc] init];
	c.width = w;
	c.height = h;
	c.minimumFrameInterval = CMTimeMake(1, fps > 0 ? fps : 60);
	// NV12, what the hardware encoder takes: no conversion anywhere.
	c.pixelFormat = kCVPixelFormatType_420YpCbCr8BiPlanarVideoRange;
	c.colorMatrix = kCGDisplayStreamYCbCrMatrix_ITU_R_709_2;
	c.showsCursor = cursor ? YES : NO;
	c.scalesToFit = YES;
	c.queueDepth = 5;
	return c;
}

static SCDisplay *find_display(SCShareableContent *content, uint32_t id) {
	SCDisplay *first = nil;
	for (SCDisplay *d in content.displays) {
		if (!first)
			first = d;
		if (d.displayID == id)
			return d;
	}
	return id == 0 ? first : nil;
}

void *cap_start(uint32_t display, int w, int h, int fps, int cursor, uintptr_t handle, char **err) {
	@autoreleasepool {
		NSError *e = nil;
		SCShareableContent *content = shareable(&e);
		if (!content) {
			*err = copy_error(e);
			return NULL;
		}
		SCDisplay *d = find_display(content, display);
		if (!d) {
			*err = strdup("display not found");
			return NULL;
		}
		// The whole display, our own windows included: what the viewer sees
		// is exactly what the person at the computer sees.
		SCContentFilter *filter = [[SCContentFilter alloc] initWithDisplay:d excludingWindows:@[]];
		DeskCapture *cap = [[DeskCapture alloc] init];
		cap.handle = handle;
		cap.queue = dispatch_queue_create("org.go-link.desk.capture", DISPATCH_QUEUE_SERIAL);
		cap.stream = [[SCStream alloc] initWithFilter:filter configuration:make_config(w, h, fps, cursor) delegate:cap];
		if (![cap.stream addStreamOutput:cap type:SCStreamOutputTypeScreen sampleHandlerQueue:cap.queue error:&e]) {
			*err = copy_error(e);
			return NULL;
		}
		__block NSError *startErr = nil;
		dispatch_semaphore_t done = dispatch_semaphore_create(0);
		[cap.stream startCaptureWithCompletionHandler:^(NSError *se) {
			startErr = se;
			dispatch_semaphore_signal(done);
		}];
		if (dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, 10 * NSEC_PER_SEC)) != 0) {
			*err = strdup("timed out starting the capture");
			return NULL;
		}
		if (startErr) {
			*err = copy_error(startErr);
			return NULL;
		}
		return (__bridge_retained void *)cap;
	}
}

// cap_update changes the size, frame rate, cursor and display of a running
// capture, without stopping it.
int cap_update(void *ref, uint32_t display, int w, int h, int fps, int cursor, char **err) {
	@autoreleasepool {
		DeskCapture *cap = (__bridge DeskCapture *)ref;
		__block NSError *ue = nil;
		dispatch_semaphore_t done = dispatch_semaphore_create(0);
		[cap.stream updateConfiguration:make_config(w, h, fps, cursor) completionHandler:^(NSError *e) {
			ue = e;
			dispatch_semaphore_signal(done);
		}];
		dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, 5 * NSEC_PER_SEC));
		if (ue) {
			*err = copy_error(ue);
			return -1;
		}
		if (display == 0)
			return 0;
		NSError *e = nil;
		SCShareableContent *content = shareable(&e);
		SCDisplay *d = content ? find_display(content, display) : nil;
		if (!d) {
			*err = e ? copy_error(e) : strdup("display not found");
			return -1;
		}
		dispatch_semaphore_t done2 = dispatch_semaphore_create(0);
		[cap.stream updateContentFilter:[[SCContentFilter alloc] initWithDisplay:d excludingWindows:@[]] completionHandler:^(NSError *fe) {
			ue = fe;
			dispatch_semaphore_signal(done2);
		}];
		dispatch_semaphore_wait(done2, dispatch_time(DISPATCH_TIME_NOW, 5 * NSEC_PER_SEC));
		if (ue) {
			*err = copy_error(ue);
			return -1;
		}
		return 0;
	}
}

void cap_stop(void *ref) {
	@autoreleasepool {
		DeskCapture *cap = (__bridge_transfer DeskCapture *)ref;
		dispatch_semaphore_t done = dispatch_semaphore_create(0);
		[cap.stream stopCaptureWithCompletionHandler:^(NSError *e) {
			dispatch_semaphore_signal(done);
		}];
		dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, 5 * NSEC_PER_SEC));
		// Frames already queued finish before the queue is let go.
		dispatch_sync(cap.queue, ^{});
	}
}

// ---- pixel buffers ----

void cap_retain(void *pb) { CVPixelBufferRetain((CVPixelBufferRef)pb); }
void cap_release(void *pb) { CVPixelBufferRelease((CVPixelBufferRef)pb); }

// cap_to_i420 unpacks an NV12 pixel buffer into packed I420 (the VP8
// encoder's input): the luma rows as they are, the chroma deinterleaved.
int cap_to_i420(void *pbv, uint8_t *dst, int w, int h) {
	CVPixelBufferRef pb = (CVPixelBufferRef)pbv;
	if ((int)CVPixelBufferGetWidth(pb) != w || (int)CVPixelBufferGetHeight(pb) != h || CVPixelBufferGetPlaneCount(pb) != 2)
		return -1;
	CVPixelBufferLockBaseAddress(pb, kCVPixelBufferLock_ReadOnly);
	const uint8_t *y = CVPixelBufferGetBaseAddressOfPlane(pb, 0);
	size_t sy = CVPixelBufferGetBytesPerRowOfPlane(pb, 0);
	for (int r = 0; r < h; r++)
		memcpy(dst + (size_t)r * w, y + r * sy, (size_t)w);
	const uint8_t *uv = CVPixelBufferGetBaseAddressOfPlane(pb, 1);
	size_t suv = CVPixelBufferGetBytesPerRowOfPlane(pb, 1);
	int cw = w / 2, ch = h / 2;
	uint8_t *u = dst + (size_t)w * h, *v = u + (size_t)cw * ch;
	for (int r = 0; r < ch; r++) {
		const uint8_t *row = uv + r * suv;
		for (int c = 0; c < cw; c++) {
			u[r * cw + c] = row[2 * c];
			v[r * cw + c] = row[2 * c + 1];
		}
	}
	CVPixelBufferUnlockBaseAddress(pb, kCVPixelBufferLock_ReadOnly);
	return 0;
}

// ---- permissions ----

int cap_has_access(void) { return CGPreflightScreenCaptureAccess() ? 1 : 0; }
int cap_request_access(void) { return CGRequestScreenCaptureAccess() ? 1 : 0; }
