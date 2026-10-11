// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

#import <AppKit/AppKit.h>

static NSWindow *beacon;

// bea_show puts the beacon (a square, white or black) in the top-left
// corner of the display at x, y (the desktop's points, top-left origin),
// above everything, letting clicks through. It needs the app's main loop
// (the window); without one nothing shows.
void bea_show(int on, double x, double y, double side) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (!beacon) {
			beacon = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, side, side) styleMask:NSWindowStyleMaskBorderless backing:NSBackingStoreBuffered defer:NO];
			beacon.level = NSScreenSaverWindowLevel;
			beacon.ignoresMouseEvents = YES;
			beacon.hasShadow = NO;
			beacon.opaque = YES;
			beacon.releasedWhenClosed = NO;
			beacon.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorFullScreenAuxiliary | NSWindowCollectionBehaviorStationary;
		}
		// AppKit counts from the bottom of the main display.
		CGFloat mainH = NSScreen.screens.firstObject.frame.size.height;
		[beacon setFrame:NSMakeRect(x, mainH - y - side, side, side) display:NO];
		beacon.backgroundColor = on ? NSColor.whiteColor : NSColor.blackColor;
		[beacon orderFrontRegardless];
		[beacon display];
	});
}

void bea_hide(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		[beacon orderOut:nil];
	});
}
