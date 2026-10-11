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
		// When another app keeps the focus, still over it.
		[w orderFrontRegardless];
	}
}

// deskFloatTry looks for the window titled want, tries left times more
// (every 100 ms) until it is on screen: a window shown while the main
// thread is busy (a session ending) appears only some time later, and one
// floated before it was on screen stays behind the other apps.
static void deskFloatTry(NSString *want, BOOL focus, int left) {
	BOOL shown = NO;
	for (NSWindow *w in [NSApp windows]) {
		if ([w.title isEqualToString:want]) {
			deskApplyFloat(w, focus && w.isVisible);
			shown = shown || w.isVisible;
		}
	}
	if (shown || left <= 0) {
		return;
	}
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 100 * NSEC_PER_MSEC), dispatch_get_main_queue(), ^{
		deskFloatTry(want, focus, left - 1);
	});
}

// deskFloat keeps the window with this title above the other windows and
// on every Space (also over full-screen apps), so a connection request or
// an ongoing session can never hide behind something else. With focus,
// the window also comes to the front with the keyboard.
void deskFloat(const char *title, int focus) {
	NSString *want = [NSString stringWithUTF8String:title];
	dispatch_async(dispatch_get_main_queue(), ^{
		// Up to 5 s for the window to show.
		deskFloatTry(want, focus != 0, 50);
	});
}
