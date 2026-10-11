// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

#import <AVFoundation/AVFoundation.h>
#import <CoreMedia/CoreMedia.h>
#include <stdint.h>
#include <stdlib.h>

extern void goCameraFrame(uintptr_t handle, void *pixelBuffer, int width, int height, int64_t ptsMicros);

@interface DeskCamera : NSObject <AVCaptureVideoDataOutputSampleBufferDelegate>
@property(nonatomic) uintptr_t handle;
@property(nonatomic, strong) AVCaptureSession *session;
@property(nonatomic, strong) dispatch_queue_t queue;
@end

@implementation DeskCamera
- (void)captureOutput:(AVCaptureOutput *)output didOutputSampleBuffer:(CMSampleBufferRef)sb fromConnection:(AVCaptureConnection *)connection {
	CVPixelBufferRef pb = CMSampleBufferGetImageBuffer(sb);
	if (!pb)
		return;
	CMTime pts = CMSampleBufferGetPresentationTimeStamp(sb);
	int64_t us = CMTIME_IS_VALID(pts) ? (int64_t)(CMTimeGetSeconds(pts) * 1e6) : 0;
	goCameraFrame(self.handle, (void *)pb, (int)CVPixelBufferGetWidth(pb), (int)CVPixelBufferGetHeight(pb), us);
}
@end

static char *copy_error(NSError *e) {
	NSString *s = e ? e.localizedDescription : @"unknown error";
	return strdup(s.UTF8String ? s.UTF8String : "unknown error");
}

// The preset nearest to the size asked for, never larger than 1280x720.
static AVCaptureSessionPreset preset_for(int w, int h) {
	if (w >= 1280 || h >= 720)
		return AVCaptureSessionPreset1280x720;
	if (w >= 960 || h >= 540)
		return AVCaptureSessionPreset960x540;
	return AVCaptureSessionPreset640x480;
}

void *cam_start(int w, int h, int fps, uintptr_t handle, char **err) {
	@autoreleasepool {
		AVCaptureDevice *dev = [AVCaptureDevice defaultDeviceWithMediaType:AVMediaTypeVideo];
		if (!dev)
			return NULL; // no camera
		NSError *e = nil;
		AVCaptureDeviceInput *in = [AVCaptureDeviceInput deviceInputWithDevice:dev error:&e];
		if (!in) {
			*err = copy_error(e);
			return NULL;
		}
		DeskCamera *cam = [[DeskCamera alloc] init];
		cam.handle = handle;
		cam.queue = dispatch_queue_create("org.go-link.desk.camera", DISPATCH_QUEUE_SERIAL);
		cam.session = [[AVCaptureSession alloc] init];
		[cam.session beginConfiguration];
		AVCaptureSessionPreset p = preset_for(w, h);
		if ([cam.session canSetSessionPreset:p])
			cam.session.sessionPreset = p;
		if (![cam.session canAddInput:in]) {
			*err = strdup("the camera cannot be opened");
			return NULL;
		}
		[cam.session addInput:in];
		AVCaptureVideoDataOutput *out = [[AVCaptureVideoDataOutput alloc] init];
		// NV12, what the hardware encoder takes.
		out.videoSettings = @{(id)kCVPixelBufferPixelFormatTypeKey : @(kCVPixelFormatType_420YpCbCr8BiPlanarVideoRange)};
		out.alwaysDiscardsLateVideoFrames = YES;
		[out setSampleBufferDelegate:cam queue:cam.queue];
		if (![cam.session canAddOutput:out]) {
			*err = strdup("the camera's frames cannot be read");
			return NULL;
		}
		[cam.session addOutput:out];
		if (fps > 0 && [dev lockForConfiguration:nil]) {
			for (AVFrameRateRange *r in dev.activeFormat.videoSupportedFrameRateRanges) {
				if (r.maxFrameRate >= fps && r.minFrameRate <= fps) {
					dev.activeVideoMinFrameDuration = CMTimeMake(1, fps);
					dev.activeVideoMaxFrameDuration = CMTimeMake(1, fps);
					break;
				}
			}
			[dev unlockForConfiguration];
		}
		[cam.session commitConfiguration];
		[cam.session startRunning];
		if (!cam.session.isRunning) {
			*err = strdup("the camera did not start (in use by another app, or not allowed)");
			return NULL;
		}
		return (__bridge_retained void *)cam;
	}
}

void cam_stop(void *ref) {
	@autoreleasepool {
		DeskCamera *cam = (__bridge_transfer DeskCamera *)ref;
		[cam.session stopRunning];
		dispatch_sync(cam.queue, ^{});
	}
}

int cam_access(void) { return [AVCaptureDevice authorizationStatusForMediaType:AVMediaTypeVideo] == AVAuthorizationStatusAuthorized ? 1 : 0; }

int cam_request(void) {
	if (cam_access())
		return 1;
	__block BOOL ok = NO;
	dispatch_semaphore_t done = dispatch_semaphore_create(0);
	[AVCaptureDevice requestAccessForMediaType:AVMediaTypeVideo completionHandler:^(BOOL granted) {
		ok = granted;
		dispatch_semaphore_signal(done);
	}];
	dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, 120 * NSEC_PER_SEC));
	return ok ? 1 : 0;
}
