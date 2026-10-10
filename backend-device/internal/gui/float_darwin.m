// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless && darwin

#import <Cocoa/Cocoa.h>

static void deskApplyFloat(NSWindow *w, BOOL focus) {
	w.level = NSFloatingWindowLevel;
	w.collectionBehavior |= NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorFullScreenAuxiliary;
	w.hidesOnDeactivate = NO;
	if (focus) {
		[NSApp activateIgnoringOtherApps:YES];
		[w makeKeyAndOrderFront:nil];
	}
}

// deskFloat keeps the window with this title above the other windows and
// on every Space (also over full-screen apps), so a connection request or
// an ongoing session can never hide behind something else. With focus,
// the window also comes to the front with the keyboard.
void deskFloat(const char *title, int focus) {
	NSString *want = [NSString stringWithUTF8String:title];
	void (^apply)(void) = ^{
		for (NSWindow *w in [NSApp windows]) {
			if ([w.title isEqualToString:want]) {
				deskApplyFloat(w, focus != 0);
			}
		}
	};
	// The window may be created a moment after the call: try again then.
	dispatch_async(dispatch_get_main_queue(), apply);
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 200 * NSEC_PER_MSEC), dispatch_get_main_queue(), apply);
}
