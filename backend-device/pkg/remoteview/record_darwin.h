// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

#import <CoreMedia/CoreMedia.h>
#import <CoreVideo/CoreVideo.h>
#import <Foundation/Foundation.h>

// RVRecorder writes a session's picture, and the computer's sound when
// asked, to an MPEG-4 file (H.264 and AAC, encoded by the system): the
// pictures the window shows, decoded again for it when they come as H.264.
// Every method may be called from any thread.
@interface RVRecorder : NSObject
- (instancetype)initWithURL:(NSURL *)url audio:(BOOL)audio;
// A decoded picture (VP8 sessions).
- (void)appendImage:(CVPixelBufferRef)pb;
// A compressed H.264 picture as the window enqueues it.
- (void)appendH264:(CMSampleBufferRef)sample;
// The computer's sound: 16-bit mono at 48 kHz.
- (void)appendPCM:(const int16_t *)pcm count:(int)n;
// Closes the file; done is told on a queue of its own whether it holds
// anything (an empty file is removed).
- (void)finish:(void (^)(BOOL ok, NSString *err))done;
@end
