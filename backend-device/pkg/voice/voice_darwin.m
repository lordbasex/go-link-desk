// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

#import <AVFoundation/AVFoundation.h>
#include <stdint.h>
#include <stdlib.h>

extern void goVoiceMic(uintptr_t handle, int16_t *samples, int n);

enum { kRate = 48000 };

@interface DeskVoice : NSObject
@property(nonatomic) uintptr_t handle;
@property(nonatomic, strong) AVAudioEngine *engine;
@property(nonatomic, strong) AVAudioPlayerNode *player;
@property(nonatomic, strong) AVAudioFormat *mono; // 48 kHz mono float: what is played
@property(nonatomic, strong) AVAudioConverter *toMono;
@property(atomic) BOOL muted;
@property(atomic) int queued; // frames scheduled and not yet played
@end

@implementation DeskVoice
@end

static char *copy_error(NSError *e, const char *fallback) {
	NSString *s = e ? e.localizedDescription : [NSString stringWithUTF8String:fallback];
	return strdup(s.UTF8String ? s.UTF8String : fallback);
}

void *voice_start(int mic, uintptr_t handle, char **err) {
	@autoreleasepool {
		DeskVoice *v = [[DeskVoice alloc] init];
		v.handle = handle;
		v.engine = [[AVAudioEngine alloc] init];
		v.mono = [[AVAudioFormat alloc] initWithCommonFormat:AVAudioPCMFormatFloat32 sampleRate:kRate channels:1 interleaved:NO];
		NSError *e = nil;
		if (mic) {
			AVAudioInputNode *input = v.engine.inputNode;
			// The system's echo cancellation: what the speaker plays is taken
			// out of what the microphone hears.
			if (![input setVoiceProcessingEnabled:YES error:&e]) {
				*err = copy_error(e, "voice processing is not available");
				return NULL;
			}
			// Leave the computer's other sound (the screen being shared) at
			// its volume while the call lasts.
			if (@available(macOS 14.0, *)) {
				AVAudioVoiceProcessingOtherAudioDuckingConfiguration duck = {.enableAdvancedDucking = NO, .duckingLevel = AVAudioVoiceProcessingOtherAudioDuckingLevelMin};
				input.voiceProcessingOtherAudioDuckingConfiguration = duck;
			}
			AVAudioFormat *inFmt = [input outputFormatForBus:0];
			AVAudioFormat *outFmt = [[AVAudioFormat alloc] initWithCommonFormat:AVAudioPCMFormatInt16 sampleRate:kRate channels:1 interleaved:YES];
			v.toMono = [[AVAudioConverter alloc] initFromFormat:inFmt toFormat:outFmt];
			if (!v.toMono) {
				*err = strdup("the microphone's format cannot be converted");
				return NULL;
			}
			__weak DeskVoice *weak = v;
			[input installTapOnBus:0 bufferSize:960 format:inFmt block:^(AVAudioPCMBuffer *buf, AVAudioTime *when) {
				DeskVoice *s = weak;
				if (!s || s.muted)
					return;
				AVAudioFrameCount cap = (AVAudioFrameCount)((double)buf.frameLength * kRate / inFmt.sampleRate) + 32;
				AVAudioPCMBuffer *out = [[AVAudioPCMBuffer alloc] initWithPCMFormat:outFmt frameCapacity:cap];
				__block BOOL given = NO;
				NSError *ce = nil;
				[s.toMono convertToBuffer:out error:&ce withInputFromBlock:^AVAudioBuffer *(AVAudioPacketCount n, AVAudioConverterInputStatus *status) {
					if (given) {
						*status = AVAudioConverterInputStatus_NoDataNow;
						return nil;
					}
					given = YES;
					*status = AVAudioConverterInputStatus_HaveData;
					return buf;
				}];
				if (out.frameLength > 0)
					goVoiceMic(s.handle, out.int16ChannelData[0], (int)out.frameLength);
			}];
		}
		v.player = [[AVAudioPlayerNode alloc] init];
		[v.engine attachNode:v.player];
		[v.engine connect:v.player to:v.engine.mainMixerNode format:v.mono];
		[v.engine prepare];
		if (![v.engine startAndReturnError:&e]) {
			*err = copy_error(e, "the sound did not start");
			return NULL;
		}
		[v.player play];
		return (__bridge_retained void *)v;
	}
}

void voice_play(void *ref, const int16_t *samples, int n) {
	@autoreleasepool {
		DeskVoice *v = (__bridge DeskVoice *)ref;
		// Too much queued (the network caught up in a burst): drop it rather
		// than fall behind the picture.
		if (v.queued > kRate * 3 / 10)
			return;
		AVAudioPCMBuffer *buf = [[AVAudioPCMBuffer alloc] initWithPCMFormat:v.mono frameCapacity:(AVAudioFrameCount)n];
		buf.frameLength = (AVAudioFrameCount)n;
		float *dst = buf.floatChannelData[0];
		for (int i = 0; i < n; i++)
			dst[i] = samples[i] / 32768.0f;
		@synchronized(v) {
			v.queued += n;
		}
		__weak DeskVoice *weak = v;
		[v.player scheduleBuffer:buf completionHandler:^{
			DeskVoice *s = weak;
			if (s) {
				@synchronized(s) {
					s.queued -= n;
				}
			}
		}];
	}
}

void voice_mute(void *ref, int muted) {
	DeskVoice *v = (__bridge DeskVoice *)ref;
	v.muted = muted != 0;
}

void voice_stop(void *ref) {
	@autoreleasepool {
		DeskVoice *v = (__bridge_transfer DeskVoice *)ref;
		[v.player stop];
		if (v.toMono)
			[v.engine.inputNode removeTapOnBus:0];
		[v.engine stop];
	}
}

int voice_mic_access(void) { return [AVCaptureDevice authorizationStatusForMediaType:AVMediaTypeAudio] == AVAuthorizationStatusAuthorized ? 1 : 0; }

int voice_mic_request(void) {
	if (voice_mic_access())
		return 1;
	__block BOOL ok = NO;
	dispatch_semaphore_t done = dispatch_semaphore_create(0);
	[AVCaptureDevice requestAccessForMediaType:AVMediaTypeAudio completionHandler:^(BOOL granted) {
		ok = granted;
		dispatch_semaphore_signal(done);
	}];
	dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, 120 * NSEC_PER_SEC));
	return ok ? 1 : 0;
}
