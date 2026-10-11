// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

#import <AppKit/AppKit.h>

// ml_run serves the main thread's event loop as an app without a Dock icon
// (accessory): ScreenCaptureKit answers there, and the latency beacon's
// window lives there. It returns after ml_stop.
void ml_run(void) {
	@autoreleasepool {
		[NSApplication sharedApplication];
		[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
		[NSApp run];
	}
}

void ml_stop(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		[NSApp stop:nil];
		// stop takes effect after the next event: send one.
		NSEvent *e = [NSEvent otherEventWithType:NSEventTypeApplicationDefined location:NSZeroPoint modifierFlags:0
			timestamp:0 windowNumber:0 context:nil subtype:0 data1:0 data2:0];
		[NSApp postEvent:e atStart:YES];
	});
}
