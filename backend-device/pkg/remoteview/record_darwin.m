// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin && !headless

#import "record_darwin.h"

#import <AVFoundation/AVFoundation.h>
#import <VideoToolbox/VideoToolbox.h>

// The recording's limits: wider pictures are scaled down, and the bit
// rate keeps a desktop's text sharp.
static const int kMaxWidth = 3840;
static const int kVideoBps = 8000000;
static const int kAudioRate = 48000;

@interface RVRecorder ()
@property(nonatomic, strong) NSURL *url;
@property(nonatomic) BOOL wantsAudio;
@property(nonatomic) dispatch_queue_t q;
@property(nonatomic, strong) AVAssetWriter *writer;
@property(nonatomic, strong) AVAssetWriterInput *video;
@property(nonatomic, strong) AVAssetWriterInput *audio;
@property(nonatomic, strong) AVAssetWriterInputPixelBufferAdaptor *adaptor;
@property(nonatomic) VTPixelTransferSessionRef transfer;
@property(nonatomic) VTDecompressionSessionRef decoder;
@property(nonatomic) CMFormatDescriptionRef decoderFormat;
@property(nonatomic) CMTime start;
@property(nonatomic) CMTime lastVideo;
@property(nonatomic) BOOL started;
@property(nonatomic) BOOL failed;
@property(nonatomic) BOOL finishing;
// The sound's clock: its first sample's time, and the samples since.
@property(nonatomic) CMTime audioAnchor;
@property(nonatomic) int64_t audioSamples;
@property(nonatomic) CMAudioFormatDescriptionRef pcmFormat;
@end

static CMTime now(void) { return CMClockGetTime(CMClockGetHostTimeClock()); }

static void decoded(void *refCon, void *frameRef, OSStatus status, VTDecodeInfoFlags flags, CVImageBufferRef image, CMTime pts, CMTime dur) {
	if (status != noErr || !image) return;
	RVRecorder *r = (__bridge RVRecorder *)refCon;
	[r appendImage:image];
}

@implementation RVRecorder

- (instancetype)initWithURL:(NSURL *)url audio:(BOOL)audio {
	if ((self = [super init])) {
		_url = url;
		_wantsAudio = audio;
		_q = dispatch_queue_create("org.go-link.desk.recorder", DISPATCH_QUEUE_SERIAL);
		_lastVideo = kCMTimeInvalid;
		_audioAnchor = kCMTimeInvalid;
	}
	return self;
}

- (void)dealloc {
	if (_transfer) {
		VTPixelTransferSessionInvalidate(_transfer);
		CFRelease(_transfer);
	}
	[self dropDecoder];
	if (_pcmFormat) CFRelease(_pcmFormat);
}

- (void)dropDecoder {
	if (_decoder) {
		VTDecompressionSessionInvalidate(_decoder);
		CFRelease(_decoder);
		_decoder = NULL;
	}
	if (_decoderFormat) {
		CFRelease(_decoderFormat);
		_decoderFormat = NULL;
	}
}

// open makes the file once the first picture tells its size (on q).
- (BOOL)openWidth:(size_t)w height:(size_t)h {
	if (self.writer) return YES;
	if (self.failed || w == 0 || h == 0) return NO;
	if (w > kMaxWidth) {
		h = h * kMaxWidth / w;
		w = kMaxWidth;
	}
	w &= ~(size_t)1;
	h &= ~(size_t)1;
	[[NSFileManager defaultManager] removeItemAtURL:self.url error:nil];
	NSError *err = nil;
	AVAssetWriter *writer = [AVAssetWriter assetWriterWithURL:self.url fileType:AVFileTypeMPEG4 error:&err];
	if (!writer) {
		self.failed = YES;
		return NO;
	}
	NSDictionary *vs = @{
		AVVideoCodecKey : AVVideoCodecTypeH264,
		AVVideoWidthKey : @(w),
		AVVideoHeightKey : @(h),
		AVVideoCompressionPropertiesKey : @{
			AVVideoAverageBitRateKey : @(kVideoBps),
			AVVideoMaxKeyFrameIntervalDurationKey : @2,
			AVVideoProfileLevelKey : AVVideoProfileLevelH264HighAutoLevel,
			AVVideoAllowFrameReorderingKey : @NO,
		},
	};
	AVAssetWriterInput *video = [AVAssetWriterInput assetWriterInputWithMediaType:AVMediaTypeVideo outputSettings:vs];
	video.expectsMediaDataInRealTime = YES;
	NSDictionary *attrs = @{
		(id)kCVPixelBufferPixelFormatTypeKey : @(kCVPixelFormatType_420YpCbCr8BiPlanarVideoRange),
		(id)kCVPixelBufferWidthKey : @(w),
		(id)kCVPixelBufferHeightKey : @(h),
	};
	AVAssetWriterInputPixelBufferAdaptor *adaptor = [AVAssetWriterInputPixelBufferAdaptor assetWriterInputPixelBufferAdaptorWithAssetWriterInput:video sourcePixelBufferAttributes:attrs];
	if (![writer canAddInput:video]) {
		self.failed = YES;
		return NO;
	}
	[writer addInput:video];
	if (self.wantsAudio) {
		NSDictionary *as = @{
			AVFormatIDKey : @(kAudioFormatMPEG4AAC),
			AVSampleRateKey : @(kAudioRate),
			AVNumberOfChannelsKey : @1,
			AVEncoderBitRateKey : @96000,
		};
		AVAssetWriterInput *audio = [AVAssetWriterInput assetWriterInputWithMediaType:AVMediaTypeAudio outputSettings:as];
		audio.expectsMediaDataInRealTime = YES;
		if ([writer canAddInput:audio]) {
			[writer addInput:audio];
			self.audio = audio;
		}
	}
	if (![writer startWriting]) {
		self.failed = YES;
		return NO;
	}
	self.start = now();
	[writer startSessionAtSourceTime:self.start];
	self.writer = writer;
	self.video = video;
	self.adaptor = adaptor;
	self.started = YES;
	VTPixelTransferSessionRef t = NULL;
	if (VTPixelTransferSessionCreate(NULL, &t) == noErr) {
		VTSessionSetProperty(t, kVTPixelTransferPropertyKey_ScalingMode, kVTScalingMode_Letterbox);
		self.transfer = t;
	}
	return YES;
}

- (void)appendImage:(CVPixelBufferRef)pb {
	CVPixelBufferRetain(pb);
	dispatch_async(self.q, ^{
		[self writeImage:pb];
		CVPixelBufferRelease(pb);
	});
}

- (void)writeImage:(CVPixelBufferRef)pb {
	if (self.finishing || ![self openWidth:CVPixelBufferGetWidth(pb) height:CVPixelBufferGetHeight(pb)]) return;
	if (self.writer.status != AVAssetWriterStatusWriting || !self.video.readyForMoreMediaData) return;
	CMTime t = now();
	if (CMTIME_IS_VALID(self.lastVideo) && CMTimeCompare(t, self.lastVideo) <= 0) return;
	CVPixelBufferRef dst = NULL;
	if (!self.adaptor.pixelBufferPool || CVPixelBufferPoolCreatePixelBuffer(NULL, self.adaptor.pixelBufferPool, &dst) != kCVReturnSuccess || !dst) return;
	// The same size and format pass through; anything else is scaled and
	// converted (a picture that changed size keeps the recording's).
	if (!self.transfer || VTPixelTransferSessionTransferImage(self.transfer, pb, dst) != noErr) {
		CVPixelBufferRelease(dst);
		return;
	}
	if ([self.adaptor appendPixelBuffer:dst withPresentationTime:t]) self.lastVideo = t;
	CVPixelBufferRelease(dst);
}

- (void)appendH264:(CMSampleBufferRef)sample {
	CFRetain(sample);
	dispatch_async(self.q, ^{
		[self decode:sample];
		CFRelease(sample);
	});
}

// decode runs the system's decoder over a compressed picture (on q); its
// pictures come back to appendImage.
- (void)decode:(CMSampleBufferRef)sample {
	if (self.finishing) return;
	CMFormatDescriptionRef fmt = CMSampleBufferGetFormatDescription(sample);
	if (!fmt) return;
	if (self.decoder && (!self.decoderFormat || !CMFormatDescriptionEqual(fmt, self.decoderFormat))) [self dropDecoder];
	if (!self.decoder) {
		NSDictionary *out = @{(id)kCVPixelBufferPixelFormatTypeKey : @(kCVPixelFormatType_420YpCbCr8BiPlanarVideoRange)};
		VTDecompressionOutputCallbackRecord cb = {decoded, (__bridge void *)self};
		VTDecompressionSessionRef d = NULL;
		if (VTDecompressionSessionCreate(NULL, fmt, NULL, (__bridge CFDictionaryRef)out, &cb, &d) != noErr || !d) return;
		self.decoder = d;
		self.decoderFormat = (CMFormatDescriptionRef)CFRetain(fmt);
	}
	VTDecompressionSessionDecodeFrame(self.decoder, sample, 0, NULL, NULL);
}

- (void)appendPCM:(const int16_t *)pcm count:(int)n {
	if (!self.wantsAudio || n <= 0) return;
	NSData *d = [NSData dataWithBytes:pcm length:(NSUInteger)n * 2];
	CMTime at = now();
	dispatch_async(self.q, ^{
		[self writePCM:d at:at];
	});
}

// writePCM keeps the sound on its own clock: each piece follows the last
// by its length, and starts again from the arrival time when they drift
// apart (a gap, the sound turned off).
- (void)writePCM:(NSData *)d at:(CMTime)at {
	if (self.finishing || !self.started || !self.audio || self.writer.status != AVAssetWriterStatusWriting) return;
	if (CMTimeCompare(at, self.start) < 0) return;
	int n = (int)(d.length / 2);
	CMTime pts = CMTIME_IS_VALID(self.audioAnchor) ? CMTimeAdd(self.audioAnchor, CMTimeMake(self.audioSamples, kAudioRate)) : at;
	if (!CMTIME_IS_VALID(self.audioAnchor) || fabs(CMTimeGetSeconds(CMTimeSubtract(at, pts))) > 0.25) {
		self.audioAnchor = at;
		self.audioSamples = 0;
		pts = at;
	}
	self.audioSamples += n;
	if (!self.audio.readyForMoreMediaData) return;
	if (!self.pcmFormat) {
		AudioStreamBasicDescription asbd = {0};
		asbd.mSampleRate = kAudioRate;
		asbd.mFormatID = kAudioFormatLinearPCM;
		asbd.mFormatFlags = kAudioFormatFlagIsSignedInteger | kAudioFormatFlagIsPacked;
		asbd.mBytesPerPacket = 2;
		asbd.mFramesPerPacket = 1;
		asbd.mBytesPerFrame = 2;
		asbd.mChannelsPerFrame = 1;
		asbd.mBitsPerChannel = 16;
		CMAudioFormatDescriptionRef f = NULL;
		if (CMAudioFormatDescriptionCreate(NULL, &asbd, 0, NULL, 0, NULL, NULL, &f) != noErr) return;
		self.pcmFormat = f;
	}
	CMBlockBufferRef block = NULL;
	if (CMBlockBufferCreateWithMemoryBlock(NULL, NULL, d.length, NULL, NULL, 0, d.length, kCMBlockBufferAssureMemoryNowFlag, &block) != noErr) return;
	CMBlockBufferReplaceDataBytes(d.bytes, block, 0, d.length);
	CMSampleBufferRef sample = NULL;
	OSStatus st = CMAudioSampleBufferCreateReadyWithPacketDescriptions(NULL, block, self.pcmFormat, n, pts, NULL, &sample);
	CFRelease(block);
	if (st != noErr || !sample) return;
	[self.audio appendSampleBuffer:sample];
	CFRelease(sample);
}

- (void)finish:(void (^)(BOOL ok, NSString *err))done {
	dispatch_async(self.q, ^{
		self.finishing = YES;
		if (self.decoder) VTDecompressionSessionWaitForAsynchronousFrames(self.decoder);
		[self dropDecoder];
		AVAssetWriter *w = self.writer;
		if (!w || !CMTIME_IS_VALID(self.lastVideo)) {
			[w cancelWriting];
			[[NSFileManager defaultManager] removeItemAtURL:self.url error:nil];
			done(NO, self.failed ? @"the system could not write the file" : @"no picture came while recording");
			return;
		}
		[self.video markAsFinished];
		[self.audio markAsFinished];
		[w endSessionAtSourceTime:self.lastVideo];
		[w finishWritingWithCompletionHandler:^{
			BOOL ok = w.status == AVAssetWriterStatusCompleted;
			done(ok, ok ? @"" : (w.error.localizedDescription ?: @"the file could not be finished"));
		}];
	});
}

@end
