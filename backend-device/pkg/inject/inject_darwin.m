// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

#import <AppKit/AppKit.h>
#import <ApplicationServices/ApplicationServices.h>
#import <Carbon/Carbon.h>

static CGEventSourceRef source(void) {
	static CGEventSourceRef src;
	if (!src)
		src = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
	return src;
}

static void post(CGEventRef e, uint64_t flags) {
	if (!e)
		return;
	CGEventSetFlags(e, (CGEventFlags)flags);
	CGEventPost(kCGHIDEventTap, e);
	CFRelease(e);
}

// inj_mouse posts a mouse event of type at x, y (the desktop's points) for
// button (0 left, 1 right, 2+ other), with a click count.
void inj_mouse(int type, double x, double y, int button, int clicks, uint64_t flags) {
	CGMouseButton b = button == 0 ? kCGMouseButtonLeft : button == 1 ? kCGMouseButtonRight : (CGMouseButton)button;
	CGEventRef e = CGEventCreateMouseEvent(source(), (CGEventType)type, CGPointMake(x, y), b);
	if (e && clicks > 0)
		CGEventSetIntegerValueField(e, kCGMouseEventClickState, clicks);
	post(e, flags);
}

// inj_mouse_rel moves the pointer by dx, dy (pointer lock): the position
// moves, and the deltas carry the motion for games that read only those.
void inj_mouse_rel(int type, double x, double y, int dx, int dy, int button, uint64_t flags) {
	CGEventRef e = CGEventCreateMouseEvent(source(), (CGEventType)type, CGPointMake(x, y), (CGMouseButton)button);
	if (e) {
		CGEventSetIntegerValueField(e, kCGMouseEventDeltaX, dx);
		CGEventSetIntegerValueField(e, kCGMouseEventDeltaY, dy);
	}
	post(e, flags);
}

// inj_scroll posts a scroll: pixels (smooth, like a trackpad) or lines.
void inj_scroll(int dx, int dy, int lines, uint64_t flags) {
	CGEventRef e = CGEventCreateScrollWheelEvent2(source(), lines ? kCGScrollEventUnitLine : kCGScrollEventUnitPixel, 2, dy, dx, 0);
	post(e, flags);
}

void inj_key(int vk, int down, int repeat, uint64_t flags) {
	CGEventRef e = CGEventCreateKeyboardEvent(source(), (CGKeyCode)vk, down ? true : false);
	if (e && repeat)
		CGEventSetIntegerValueField(e, kCGKeyboardEventAutorepeat, 1);
	post(e, flags);
}

// inj_text types UTF-16 text, up to 20 units an event (what the system
// takes), on a key down and up with no key of its own.
void inj_text(const UniChar *s, int n) {
	for (int at = 0; at < n; at += 20) {
		int len = n - at < 20 ? n - at : 20;
		CGEventRef down = CGEventCreateKeyboardEvent(source(), 0, true);
		CGEventRef up = CGEventCreateKeyboardEvent(source(), 0, false);
		if (down) {
			CGEventKeyboardSetUnicodeString(down, len, s + at);
			post(down, 0);
		}
		if (up) {
			CGEventKeyboardSetUnicodeString(up, len, s + at);
			post(up, 0);
		}
	}
}

void inj_position(double *x, double *y) {
	CGEventRef e = CGEventCreate(NULL);
	CGPoint p = CGEventGetLocation(e);
	CFRelease(e);
	*x = p.x;
	*y = p.y;
}

double inj_double_click_interval(void) { return NSEvent.doubleClickInterval; }

int inj_trusted(void) { return AXIsProcessTrusted() ? 1 : 0; }
