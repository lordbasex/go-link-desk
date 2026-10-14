// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin && !headless

#import <AVFoundation/AVFoundation.h>
#import <Cocoa/Cocoa.h>
#import <CoreMedia/CoreMedia.h>
#import <CoreVideo/CoreVideo.h>
#import <QuartzCore/QuartzCore.h>
#include <stdint.h>
#import <objc/runtime.h>

#import "record_darwin.h"

// Go callbacks (remoteview_darwin.go).
extern void goRVMove(uintptr_t h, double x, double y, int inside);
extern void goRVButton(uintptr_t h, int button, int down, double x, double y, int inside);
extern void goRVWheel(uintptr_t h, double dx, double dy, int lines);
extern void goRVKey(uintptr_t h, int code, int down, int repeat, int mods);
extern void goRVResize(uintptr_t h, int w, int hgt);
extern void goRVAction(uintptr_t h, char *name, char *arg);
extern void goRVThumb(uintptr_t h, void *data, int n);

static void action(uintptr_t h, NSString *name, NSString *arg) {
	goRVAction(h, (char *)name.UTF8String, (char *)(arg ?: @"").UTF8String);
}

// arr is v if it is an array (JSON null is not).
static NSArray *arr(id v) {
	return [v isKindOfClass:NSArray.class] ? v : @[];
}

static void onMain(void (^block)(void)) {
	if (NSThread.isMainThread) block();
	else dispatch_async(dispatch_get_main_queue(), block);
}

// The brand's colors (frontend/packages/shared/src/tokens.css).
static NSColor *rgb(int hex) {
	return [NSColor colorWithSRGBRed:((hex >> 16) & 0xff) / 255.0 green:((hex >> 8) & 0xff) / 255.0 blue:(hex & 0xff) / 255.0 alpha:1];
}
// gLight picks the light theme's colors (rv_theme).
static BOOL gLight;
#define THEMED(dark, light) (gLight ? rgb(light) : rgb(dark))
#define C_BG THEMED(0x0e1016, 0xf3f4f7)
#define C_VIDEO rgb(0x05060a)
#define C_SURFACE THEMED(0x161a23, 0xffffff)
#define C_SURFACE2 THEMED(0x1b2030, 0xeceef3)
#define C_SELECTED THEMED(0x1e2330, 0xe2e6ee)
#define C_BORDER THEMED(0x262c3a, 0xd8dce5)
#define C_TEXT THEMED(0xe9ecf2, 0x151821)
#define C_MUTED THEMED(0xa3abbd, 0x4b5366)
#define C_FAINT THEMED(0x8c95a8, 0x6b7386)
#define C_ACCENT THEMED(0xf2a33a, 0xd97e12)
#define C_ON_ACCENT rgb(0x1a1206)
#define C_OK THEMED(0x7ee2a8, 0x1d8f55)
#define C_DANGER THEMED(0xf4a9b6, 0xc23a4c)
#define C_TINT_BG THEMED(0x2a1d0c, 0xfdf2e1)
#define C_TINT_BORDER THEMED(0x6b4a1c, 0xf1c88a)
#define C_TINT_TEXT THEMED(0xd9a35a, 0xb06a12)
#define C_FIELD_BORDER THEMED(0x3a4256, 0xc9ced9)
#define C_PLACEHOLDER THEMED(0x7f889c, 0x8a91a1)
#define C_DASHED THEMED(0x4a5368, 0xc0c6d2)
#define C_DANGER_FILL THEMED(0x5a2a33, 0xf7d7dc)
#define APPEARANCE [NSAppearance appearanceNamed:gLight ? NSAppearanceNameAqua : NSAppearanceNameDarkAqua]

static NSTextField *label(NSString *s, CGFloat size, NSFontWeight weight, NSColor *color) {
	NSTextField *l = [NSTextField labelWithString:s ?: @""];
	l.font = [NSFont systemFontOfSize:size weight:weight];
	l.textColor = color;
	l.lineBreakMode = NSLineBreakByTruncatingTail;
	return l;
}

static NSTextField *wrapping(NSString *s, CGFloat size, NSColor *color, CGFloat width) {
	NSTextField *l = [NSTextField wrappingLabelWithString:s ?: @""];
	l.font = [NSFont systemFontOfSize:size];
	l.textColor = color;
	l.preferredMaxLayoutWidth = width;
	return l;
}

// A flat button in the brand's style.
@interface RVButton : NSButton
@property(nonatomic) BOOL primary;
@property(nonatomic, strong) NSColor *fill;
@end

@implementation RVButton
+ (instancetype)withTitle:(NSString *)title symbol:(NSString *)symbol target:(id)t action:(SEL)a primary:(BOOL)primary {
	RVButton *b = [[RVButton alloc] init];
	b.bordered = NO;
	b.wantsLayer = YES;
	b.layer.cornerRadius = 7;
	b.primary = primary;
	b.target = t;
	b.action = a;
	b.title = title ?: @"";
	if (symbol) {
		b.image = [NSImage imageWithSystemSymbolName:symbol accessibilityDescription:title];
		b.imagePosition = title.length ? NSImageLeading : NSImageOnly;
		b.imageHugsTitle = YES;
	}
	b.font = [NSFont systemFontOfSize:13 weight:primary ? NSFontWeightSemibold : NSFontWeightRegular];
	[b restyle];
	return b;
}
- (void)restyle {
	NSColor *fg = self.primary ? C_ON_ACCENT : C_TEXT;
	if (!self.enabled) fg = C_FAINT;
	self.layer.backgroundColor = (self.fill ?: (self.primary ? (self.enabled ? C_ACCENT : C_SURFACE2) : C_SURFACE2)).CGColor;
	self.contentTintColor = fg;
	NSMutableParagraphStyle *p = [[NSMutableParagraphStyle alloc] init];
	p.alignment = NSTextAlignmentCenter;
	self.attributedTitle = [[NSAttributedString alloc] initWithString:self.title attributes:@{NSForegroundColorAttributeName : fg, NSFontAttributeName : self.font, NSParagraphStyleAttributeName : p}];
}
- (void)setEnabled:(BOOL)enabled {
	[super setEnabled:enabled];
	[self restyle];
}
- (NSSize)intrinsicContentSize {
	NSSize s = [super intrinsicContentSize];
	return NSMakeSize(s.width + (self.title.length ? 22 : 12), MAX(30, s.height + 10));
}
@end

// Modifier bits of the input protocol.
static int modsOf(NSEventModifierFlags f) {
	int m = 0;
	if (f & NSEventModifierFlagShift) m |= 1;
	if (f & NSEventModifierFlagControl) m |= 2;
	if (f & NSEventModifierFlagOption) m |= 4;
	if (f & NSEventModifierFlagCommand) m |= 8;
	if (f & NSEventModifierFlagCapsLock) m |= 16;
	return m;
}

// The device-dependent bit of each modifier key (IOLLEvent.h), to tell a
// press from a release in flagsChanged.
static NSUInteger modifierBit(unsigned short code) {
	switch (code) {
	case 56: return 0x02;   // left shift
	case 60: return 0x04;   // right shift
	case 59: return 0x01;   // left control
	case 62: return 0x2000; // right control
	case 58: return 0x20;   // left option
	case 61: return 0x40;   // right option
	case 55: return 0x08;   // left command
	case 54: return 0x10;   // right command
	}
	return 0;
}

#pragma mark - The picture

// RVView shows the picture and takes the mouse and keyboard.
@interface RVView : NSView <NSDraggingDestination>
@property(nonatomic) uintptr_t handle;
@property(nonatomic, strong) AVSampleBufferDisplayLayer *video;
@property(nonatomic, strong) CALayer *pointer;
@property(nonatomic) CGSize picture; // the picture's pixels (0: none yet)
@property(nonatomic) BOOL control;
@property(nonatomic, strong) NSCursor *remoteCursor;
@property(nonatomic) double cursorW, cursorH; // the pointer's size, as a fraction of the picture
@property(nonatomic) double hotX, hotY;       // its hot spot, as a fraction of its size
@property(nonatomic, strong) NSImage *cursorImage;
@property(nonatomic) double cx, cy;
@property(nonatomic) BOOL cvisible;
@property(nonatomic, strong) NSTrackingArea *tracking;
@property(nonatomic) NSSize lastPixels;
@end

@implementation RVView

- (instancetype)initWithFrame:(NSRect)frame handle:(uintptr_t)h {
	self = [super initWithFrame:frame];
	if (!self) return nil;
	_handle = h;
	self.wantsLayer = YES;
	self.layer.backgroundColor = C_VIDEO.CGColor;
	_video = [AVSampleBufferDisplayLayer layer];
	_video.videoGravity = AVLayerVideoGravityResizeAspect;
	_video.frame = self.bounds;
	[self.layer addSublayer:_video];
	_pointer = [CALayer layer];
	_pointer.hidden = YES;
	_pointer.contentsGravity = kCAGravityResize;
	_pointer.zPosition = 10;
	[self.layer addSublayer:_pointer];
	[self registerForDraggedTypes:@[ NSPasteboardTypeFileURL ]];
	return self;
}

- (BOOL)acceptsFirstResponder {
	return YES;
}
- (BOOL)acceptsFirstMouse:(NSEvent *)event {
	return YES;
}
- (BOOL)isFlipped {
	return YES;
}

// pictureRect is where the picture is drawn: fitted, centered.
- (NSRect)pictureRect {
	NSRect b = self.bounds;
	if (self.picture.width <= 0 || self.picture.height <= 0) return b;
	double s = MIN(b.size.width / self.picture.width, b.size.height / self.picture.height);
	double w = self.picture.width * s, h = self.picture.height * s;
	return NSMakeRect(b.origin.x + (b.size.width - w) / 2, b.origin.y + (b.size.height - h) / 2, w, h);
}

- (void)layout {
	[super layout];
	[CATransaction begin];
	[CATransaction setDisableActions:YES];
	self.video.frame = self.bounds;
	[self placePointer];
	[CATransaction commit];
	NSRect px = [self convertRectToBacking:self.bounds];
	NSSize size = NSMakeSize(round(px.size.width), round(px.size.height));
	if (!NSEqualSizes(size, self.lastPixels) && size.width > 0 && size.height > 0) {
		self.lastPixels = size;
		goRVResize(self.handle, (int)size.width, (int)size.height);
	}
}

- (void)updateTrackingAreas {
	[super updateTrackingAreas];
	if (self.tracking) [self removeTrackingArea:self.tracking];
	self.tracking = [[NSTrackingArea alloc] initWithRect:NSZeroRect
	                                             options:NSTrackingMouseMoved | NSTrackingActiveInKeyWindow | NSTrackingInVisibleRect | NSTrackingCursorUpdate
	                                               owner:self
	                                            userInfo:nil];
	[self addTrackingArea:self.tracking];
}

// The remote pointer: the local one takes its image while controlling;
// otherwise it is drawn where the computer says.
- (void)resetCursorRects {
	if (self.control && self.remoteCursor) [self addCursorRect:[self pictureRect] cursor:self.remoteCursor];
}

- (void)cursorUpdate:(NSEvent *)event {
	if (self.control && self.remoteCursor && NSPointInRect([self convertPoint:event.locationInWindow fromView:nil], [self pictureRect])) {
		[self.remoteCursor set];
	} else {
		[NSCursor.arrowCursor set];
	}
}

- (void)placePointer {
	NSRect r = [self pictureRect];
	BOOL show = !self.control && self.cvisible && self.cursorImage && self.cursorW > 0;
	self.pointer.hidden = !show;
	if (!show) return;
	double w = self.cursorW * r.size.width, h = self.cursorH * r.size.height;
	self.pointer.frame = NSMakeRect(r.origin.x + self.cx * r.size.width - self.hotX * w, r.origin.y + self.cy * r.size.height - self.hotY * h, w, h);
}

- (void)remakeCursor {
	NSRect r = [self pictureRect];
	if (!self.cursorImage || self.cursorW <= 0 || r.size.width <= 0) {
		self.remoteCursor = nil;
	} else {
		NSSize size = NSMakeSize(MAX(1, self.cursorW * r.size.width), MAX(1, self.cursorH * r.size.height));
		NSImage *img = [self.cursorImage copy];
		img.size = size;
		self.remoteCursor = [[NSCursor alloc] initWithImage:img hotSpot:NSMakePoint(self.hotX * size.width, self.hotY * size.height)];
	}
	[self.window invalidateCursorRectsForView:self];
}

- (void)setFrameSize:(NSSize)size {
	[super setFrameSize:size];
	[self remakeCursor];
}

// Where an event is, over the picture (0..1).
- (BOOL)at:(NSEvent *)event x:(double *)x y:(double *)y {
	NSPoint p = [self convertPoint:event.locationInWindow fromView:nil];
	NSRect r = [self pictureRect];
	*x = r.size.width > 0 ? (p.x - r.origin.x) / r.size.width : 0;
	*y = r.size.height > 0 ? (p.y - r.origin.y) / r.size.height : 0;
	BOOL inside = *x >= 0 && *x <= 1 && *y >= 0 && *y <= 1 && self.picture.width > 0;
	*x = MIN(1, MAX(0, *x));
	*y = MIN(1, MAX(0, *y));
	return inside;
}

- (void)moved:(NSEvent *)event {
	double x, y;
	BOOL in = [self at:event x:&x y:&y];
	goRVMove(self.handle, x, y, in);
}

- (void)mouseMoved:(NSEvent *)e { [self moved:e]; }
- (void)mouseDragged:(NSEvent *)e { [self moved:e]; }
- (void)rightMouseDragged:(NSEvent *)e { [self moved:e]; }
- (void)otherMouseDragged:(NSEvent *)e { [self moved:e]; }

- (void)button:(NSEvent *)event number:(int)button down:(BOOL)down {
	if (down) [self.window makeFirstResponder:self];
	double x, y;
	BOOL in = [self at:event x:&x y:&y];
	goRVButton(self.handle, button, down, x, y, in);
}

static int buttonOf(NSEvent *e) {
	switch (e.buttonNumber) {
	case 2: return 1; // middle
	case 3: return 3; // back
	case 4: return 4; // forward
	}
	return -1;
}

- (void)mouseDown:(NSEvent *)e { [self button:e number:0 down:YES]; }
- (void)mouseUp:(NSEvent *)e { [self button:e number:0 down:NO]; }
- (void)rightMouseDown:(NSEvent *)e { [self button:e number:2 down:YES]; }
- (void)rightMouseUp:(NSEvent *)e { [self button:e number:2 down:NO]; }
- (void)otherMouseDown:(NSEvent *)e {
	int b = buttonOf(e);
	if (b >= 0) [self button:e number:b down:YES];
}
- (void)otherMouseUp:(NSEvent *)e {
	int b = buttonOf(e);
	if (b >= 0) [self button:e number:b down:NO];
}

- (void)scrollWheel:(NSEvent *)e {
	goRVWheel(self.handle, -e.scrollingDeltaX, -e.scrollingDeltaY, e.hasPreciseScrollingDeltas ? 0 : 1);
}

- (void)keyDown:(NSEvent *)e {
	goRVKey(self.handle, e.keyCode, 1, e.isARepeat, modsOf(e.modifierFlags));
}

- (void)keyUp:(NSEvent *)e {
	goRVKey(self.handle, e.keyCode, 0, 0, modsOf(e.modifierFlags));
}

- (void)flagsChanged:(NSEvent *)e {
	unsigned short code = e.keyCode;
	if (code == 57) { // caps lock toggles: a press and a release
		goRVKey(self.handle, code, 1, 0, modsOf(e.modifierFlags));
		goRVKey(self.handle, code, 0, 0, modsOf(e.modifierFlags));
		return;
	}
	NSUInteger bit = modifierBit(code);
	if (bit) goRVKey(self.handle, code, (e.modifierFlags & bit) ? 1 : 0, 0, modsOf(e.modifierFlags));
}

// Shortcuts go to the computer while controlling it, not to this app.
- (BOOL)performKeyEquivalent:(NSEvent *)e {
	if (self.control && self.window.isKeyWindow && self.window.firstResponder == self) {
		if (e.type == NSEventTypeKeyDown) [self keyDown:e];
		return YES;
	}
	return [super performKeyEquivalent:e];
}

// Files dropped on the picture are sent to the computer.
- (NSDragOperation)draggingEntered:(id<NSDraggingInfo>)sender {
	return NSDragOperationCopy;
}

- (BOOL)performDragOperation:(id<NSDraggingInfo>)sender {
	NSArray<NSURL *> *urls = [sender.draggingPasteboard readObjectsForClasses:@[ NSURL.class ] options:@{NSPasteboardURLReadingFileURLsOnlyKey : @YES}];
	NSMutableArray *paths = [NSMutableArray array];
	for (NSURL *u in urls) [paths addObject:u.path];
	if (paths.count == 0) return NO;
	action(self.handle, @"drop", [paths componentsJoinedByString:@"\n"]);
	return YES;
}

@end

#pragma mark - A session

@class RVMain;
static RVMain *gMain;

// RVSession is one session's page: its toolbar over its picture. It lives
// in a tab of the main window, or in a window of its own.
@interface RVSession : NSObject <NSWindowDelegate>
@property(nonatomic) uintptr_t handle;
@property(nonatomic, strong) NSString *title;
@property(nonatomic, strong) NSView *page;
@property(nonatomic, strong) RVView *view;
@property(nonatomic, strong) NSStackView *bar;
@property(nonatomic, strong) NSTextField *barTitle;
@property(nonatomic, strong) NSMutableDictionary<NSString *, RVButton *> *buttons;
@property(nonatomic, strong) NSView *overlay;
@property(nonatomic, strong) NSTextField *message;
@property(nonatomic, strong) NSSecureTextField *password;
@property(nonatomic, strong) RVButton *passwordButton;
@property(nonatomic, strong) NSProgressIndicator *spinner;
@property(nonatomic, strong) NSTextField *hud;
@property(nonatomic, strong) NSWindow *window; // its own window, or nil in a tab
@property(nonatomic) dispatch_queue_t queue;
@property(nonatomic) CMVideoFormatDescriptionRef format; // on queue
// Decoded pictures (VP8): their buffers, and the format of the last one.
@property(nonatomic) CVPixelBufferPoolRef pool; // under @synchronized(self)
@property(nonatomic) CGSize poolSize;
@property(nonatomic) CMVideoFormatDescriptionRef imageFormat; // on queue
@property(nonatomic) BOOL closed;
@property(nonatomic) BOOL shown; // its tab is the one shown (or it has its own window)
@property(atomic, strong) RVRecorder *recorder; // while the session is recorded
@property(nonatomic, copy) NSString *recordPath;
@end

@interface RVMain : NSObject <NSWindowDelegate>
@property(nonatomic) uintptr_t handle;
@property(nonatomic, strong) NSWindow *window;
@property(nonatomic, strong) NSDictionary<NSString *, NSString *> *labels;
@property(nonatomic, strong) NSStackView *tabs;
@property(nonatomic, strong) NSView *content;
@property(nonatomic, strong) NSView *home;
@property(nonatomic, strong) NSMutableArray<RVSession *> *sessions;
@property(nonatomic, weak) RVSession *selected; // nil: Home
// Home's parts.
@property(nonatomic, strong) NSTextField *code;
@property(nonatomic, strong) NSString *link;
- (void)flashCopied;
@property(nonatomic, strong) NSTextField *status;
@property(nonatomic, strong) NSView *statusDot;
@property(nonatomic, strong) RVButton *codeCopyBtn, *renewBtn, *permBtn;
@property(nonatomic, strong) NSArray *perms;
@property(nonatomic, strong) NSTextField *version;
@property(nonatomic, strong) NSTextField *address;
@property(nonatomic, strong) NSPopUpButton *door;
@property(nonatomic, strong) RVButton *connect;
@property(nonatomic, strong) RVButton *demo;
@property(nonatomic, strong) NSView *grid; // the tiles' document view
@property(nonatomic, strong) NSScrollView *scroll;
@property(nonatomic, strong) NSArray *devices, *recents;
@property(nonatomic, strong) NSDictionary *lastState;
@property(nonatomic, strong) NSString *filter; // all, favorites, recents
@property(nonatomic) BOOL chose;                // the person picked a filter
@property(nonatomic) int viewMode; // 0 cards, 1 list, 2 detailed list
@property(nonatomic) CGFloat lastWidth;
@property(nonatomic, strong) NSButton *detailBtn;
@property(nonatomic, strong) NSTextField *linkLabel;
@property(nonatomic, strong) NSTextField *copied;
@property(nonatomic, strong) NSMutableDictionary<NSString *, NSButton *> *filterButtons;
@property(nonatomic, strong) NSButton *clearBtn, *gridBtn, *listBtn;
- (void)addSession:(RVSession *)s;
- (void)removeSession:(RVSession *)s;
- (void)select:(RVSession *)s;
- (void)renderTabs;
- (void)buildContent;
@end

@implementation RVSession

- (void)dealloc {
	if (_format) CFRelease(_format);
	if (_imageFormat) CFRelease(_imageFormat);
	if (_pool) CVPixelBufferPoolRelease(_pool);
}

- (NSString *)L:(NSString *)key {
	return gMain.labels[key] ?: key;
}

- (void)build:(NSArray<NSDictionary *> *)tools {
	self.page = [[NSView alloc] init];
	self.page.wantsLayer = YES;
	self.page.layer.backgroundColor = C_VIDEO.CGColor;
	self.view = [[RVView alloc] initWithFrame:NSMakeRect(0, 0, 800, 600) handle:self.handle];
	self.view.translatesAutoresizingMaskIntoConstraints = NO;

	// The toolbar, over the picture.
	self.barTitle = label(self.title, 13, NSFontWeightSemibold, C_TEXT);
	[self.barTitle setContentCompressionResistancePriority:NSLayoutPriorityDefaultLow forOrientation:NSLayoutConstraintOrientationHorizontal];
	NSMutableArray *items = [NSMutableArray arrayWithObject:self.barTitle];
	self.buttons = [NSMutableDictionary dictionary];
	NSView *spacer = [[NSView alloc] init];
	[spacer setContentHuggingPriority:1 forOrientation:NSLayoutConstraintOrientationHorizontal];
	[items addObject:spacer];
	for (NSDictionary *t in tools) {
		BOOL end = [t[@"id"] isEqual:@"disconnect"];
		RVButton *b = [RVButton withTitle:end ? t[@"label"] : @"" symbol:t[@"symbol"] target:self action:@selector(tool:) primary:NO];
		b.identifier = t[@"id"];
		b.toolTip = t[@"label"];
		if (end) b.fill = C_DANGER_FILL;
		[b restyle];
		self.buttons[t[@"id"]] = b;
		[items addObject:b];
	}
	self.bar = [NSStackView stackViewWithViews:items];
	self.bar.spacing = 6;
	self.bar.edgeInsets = NSEdgeInsetsMake(6, 14, 6, 10);
	self.bar.wantsLayer = YES;
	self.bar.layer.backgroundColor = C_SURFACE.CGColor;
	self.bar.translatesAutoresizingMaskIntoConstraints = NO;
	NSBox *line = [[NSBox alloc] init];
	line.boxType = NSBoxCustom;
	line.borderWidth = 0;
	line.fillColor = C_BORDER;
	line.translatesAutoresizingMaskIntoConstraints = NO;
	[self.page addSubview:self.view];
	[self.page addSubview:self.bar];
	[self.page addSubview:line];
	[NSLayoutConstraint activateConstraints:@[
		[self.bar.topAnchor constraintEqualToAnchor:self.page.topAnchor],
		[self.bar.leadingAnchor constraintEqualToAnchor:self.page.leadingAnchor],
		[self.bar.trailingAnchor constraintEqualToAnchor:self.page.trailingAnchor],
		[self.bar.heightAnchor constraintEqualToConstant:44],
		[line.topAnchor constraintEqualToAnchor:self.bar.bottomAnchor],
		[line.leadingAnchor constraintEqualToAnchor:self.page.leadingAnchor],
		[line.trailingAnchor constraintEqualToAnchor:self.page.trailingAnchor],
		[line.heightAnchor constraintEqualToConstant:1],
		[self.view.topAnchor constraintEqualToAnchor:line.bottomAnchor],
		[self.view.leadingAnchor constraintEqualToAnchor:self.page.leadingAnchor],
		[self.view.trailingAnchor constraintEqualToAnchor:self.page.trailingAnchor],
		[self.view.bottomAnchor constraintEqualToAnchor:self.page.bottomAnchor],
	]];
	[self buildOverlay];
}

- (void)buildOverlay {
	NSView *o = [[NSView alloc] init];
	o.wantsLayer = YES;
	o.layer.backgroundColor = [C_SURFACE colorWithAlphaComponent:0.96].CGColor;
	o.layer.cornerRadius = 14;
	o.layer.borderWidth = 1;
	o.layer.borderColor = C_BORDER.CGColor;
	o.translatesAutoresizingMaskIntoConstraints = NO;

	self.spinner = [[NSProgressIndicator alloc] init];
	self.spinner.style = NSProgressIndicatorStyleSpinning;
	self.spinner.controlSize = NSControlSizeRegular;
	self.message = wrapping(@"", 15, C_TEXT, 360);
	self.message.alignment = NSTextAlignmentCenter;
	self.password = [[NSSecureTextField alloc] init];
	self.password.target = self;
	self.password.action = @selector(sendPassword:);
	self.password.font = [NSFont systemFontOfSize:14];
	[self.password.widthAnchor constraintEqualToConstant:260].active = YES;
	self.passwordButton = [RVButton withTitle:@"OK" symbol:@"lock.open" target:self action:@selector(sendPassword:) primary:YES];

	NSStackView *stack = [NSStackView stackViewWithViews:@[ self.spinner, self.message, self.password, self.passwordButton ]];
	stack.orientation = NSUserInterfaceLayoutOrientationVertical;
	stack.spacing = 14;
	stack.edgeInsets = NSEdgeInsetsMake(26, 30, 26, 30);
	stack.translatesAutoresizingMaskIntoConstraints = NO;
	[o addSubview:stack];
	[NSLayoutConstraint activateConstraints:@[
		[stack.leadingAnchor constraintEqualToAnchor:o.leadingAnchor],
		[stack.trailingAnchor constraintEqualToAnchor:o.trailingAnchor],
		[stack.topAnchor constraintEqualToAnchor:o.topAnchor],
		[stack.bottomAnchor constraintEqualToAnchor:o.bottomAnchor],
	]];
	[self.page addSubview:o];
	[NSLayoutConstraint activateConstraints:@[
		[o.centerXAnchor constraintEqualToAnchor:self.view.centerXAnchor],
		[o.centerYAnchor constraintEqualToAnchor:self.view.centerYAnchor],
		[o.widthAnchor constraintLessThanOrEqualToConstant:440],
		[o.widthAnchor constraintGreaterThanOrEqualToConstant:300],
	]];
	self.overlay = o;

	self.hud = [NSTextField labelWithString:@""];
	self.hud.font = [NSFont monospacedSystemFontOfSize:11 weight:NSFontWeightRegular];
	self.hud.textColor = C_TEXT;
	self.hud.drawsBackground = YES;
	self.hud.backgroundColor = [NSColor colorWithWhite:0 alpha:0.65];
	self.hud.hidden = YES;
	self.hud.translatesAutoresizingMaskIntoConstraints = NO;
	[self.page addSubview:self.hud];
	[NSLayoutConstraint activateConstraints:@[
		[self.hud.leadingAnchor constraintEqualToAnchor:self.view.leadingAnchor constant:10],
		[self.hud.topAnchor constraintEqualToAnchor:self.view.topAnchor constant:10],
	]];
}

// restyle repaints the page in the theme's colors.
- (void)restyle {
	self.bar.layer.backgroundColor = C_SURFACE.CGColor;
	self.barTitle.textColor = C_TEXT;
	self.overlay.layer.backgroundColor = [C_SURFACE colorWithAlphaComponent:0.96].CGColor;
	self.overlay.layer.borderColor = C_BORDER.CGColor;
	self.message.textColor = C_TEXT;
	for (NSString *k in self.buttons) {
		RVButton *b = self.buttons[k];
		if ([k isEqual:@"disconnect"]) b.fill = C_DANGER_FILL;
		[b restyle];
	}
	self.window.appearance = APPEARANCE;
	for (NSView *v in self.page.subviews) {
		if ([v isKindOfClass:NSBox.class]) ((NSBox *)v).fillColor = C_BORDER;
	}
}

- (void)tool:(NSButton *)sender {
	action(self.handle, sender.identifier, @"");
}

- (void)menuChose:(NSMenuItem *)item {
	action(self.handle, @"menu", item.representedObject);
}

- (void)popMenu:(NSString *)tool items:(NSArray *)items {
	NSButton *b = self.buttons[tool];
	if (!b) return;
	NSMenu *menu = [[NSMenu alloc] init];
	menu.autoenablesItems = NO;
	for (NSDictionary *it in items) {
		if ([it[@"separator"] boolValue]) {
			[menu addItem:NSMenuItem.separatorItem];
			continue;
		}
		NSMenuItem *mi = [[NSMenuItem alloc] initWithTitle:it[@"title"] ?: @"" action:@selector(menuChose:) keyEquivalent:@""];
		mi.target = self;
		mi.representedObject = it[@"id"] ?: @"";
		mi.state = [it[@"checked"] boolValue] ? NSControlStateValueOn : NSControlStateValueOff;
		mi.enabled = ![it[@"disabled"] boolValue] && it[@"id"] != nil;
		[menu addItem:mi];
	}
	[menu popUpMenuPositioningItem:nil atLocation:NSMakePoint(0, b.bounds.size.height + 4) inView:b];
}

- (void)sendPassword:(id)sender {
	NSString *p = self.password.stringValue;
	if (p.length == 0) return;
	self.password.stringValue = @"";
	action(self.handle, @"password", p);
}

- (NSWindow *)host {
	return self.window ?: gMain.window;
}

// Shown or not: the computer may send less to a hidden one.
- (void)setShown:(BOOL)shown {
	if (_shown == shown) return;
	_shown = shown;
	action(self.handle, shown ? @"visible" : @"hidden", @"");
	if (!shown) {
		action(self.handle, @"blur", @"");
		return;
	}
	// A hidden picture may have dropped frames it needed: start again
	// from a whole one.
	dispatch_async(self.queue, ^{
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
		[self.view.video flush];
#pragma clang diagnostic pop
		action(self.handle, @"keyframe", @"");
	});
}

// Its own window's events.
- (void)windowWillClose:(NSNotification *)n {
	if (self.closed) return;
	self.closed = YES;
	action(self.handle, @"closed", @"");
}
- (void)windowDidResignKey:(NSNotification *)n {
	action(self.handle, @"blur", @"");
}
- (void)windowDidChangeOcclusionState:(NSNotification *)n {
	self.shown = (self.window.occlusionState & NSWindowOcclusionStateVisible) != 0;
}

// enqueue draws one access unit (Annex B), on the session's queue.
- (void)enqueue:(NSData *)au {
	const uint8_t *p = au.bytes;
	size_t n = au.length;
	NSMutableArray<NSData *> *nals = [NSMutableArray array];
	size_t i = 0, start = SIZE_MAX;
	while (i + 3 <= n) {
		size_t sc = 0;
		if (p[i] == 0 && p[i + 1] == 0 && p[i + 2] == 1) sc = 3;
		else if (i + 4 <= n && p[i] == 0 && p[i + 1] == 0 && p[i + 2] == 0 && p[i + 3] == 1) sc = 4;
		if (sc) {
			if (start != SIZE_MAX) [nals addObject:[NSData dataWithBytes:p + start length:i - start]];
			i += sc;
			start = i;
			continue;
		}
		i++;
	}
	if (start != SIZE_MAX && start < n) [nals addObject:[NSData dataWithBytes:p + start length:n - start]];

	const uint8_t *sps = NULL, *pps = NULL;
	size_t spsLen = 0, ppsLen = 0;
	NSMutableData *avcc = [NSMutableData data];
	for (NSData *nal in nals) {
		if (nal.length == 0) continue;
		uint8_t type = ((const uint8_t *)nal.bytes)[0] & 0x1f;
		if (type == 7) {
			sps = nal.bytes;
			spsLen = nal.length;
			continue;
		}
		if (type == 8) {
			pps = nal.bytes;
			ppsLen = nal.length;
			continue;
		}
		if (type == 9 || type == 6) continue; // access unit delimiter, SEI
		uint32_t len = CFSwapInt32HostToBig((uint32_t)nal.length);
		[avcc appendBytes:&len length:4];
		[avcc appendData:nal];
	}
	if (sps && pps) {
		const uint8_t *sets[2] = {sps, pps};
		size_t sizes[2] = {spsLen, ppsLen};
		CMVideoFormatDescriptionRef fmt = NULL;
		if (CMVideoFormatDescriptionCreateFromH264ParameterSets(NULL, 2, sets, sizes, 4, &fmt) == noErr) {
			if (!self.format || !CMFormatDescriptionEqual(fmt, self.format)) {
				if (self.format) CFRelease(self.format);
				self.format = fmt;
				CMVideoDimensions d = CMVideoFormatDescriptionGetDimensions(fmt);
				dispatch_async(dispatch_get_main_queue(), ^{
					self.view.picture = CGSizeMake(d.width, d.height);
					[self.view setNeedsLayout:YES];
					[self.view remakeCursor];
				});
			} else {
				CFRelease(fmt);
			}
		}
	}
	if (!self.format || avcc.length == 0) {
		if (!self.format) action(self.handle, @"keyframe", @"");
		return;
	}
	CMBlockBufferRef block = NULL;
	void *mem = malloc(avcc.length);
	memcpy(mem, avcc.bytes, avcc.length);
	if (CMBlockBufferCreateWithMemoryBlock(NULL, mem, avcc.length, kCFAllocatorMalloc, NULL, 0, avcc.length, 0, &block) != noErr) {
		free(mem);
		return;
	}
	CMSampleBufferRef sample = NULL;
	size_t sampleSize = avcc.length;
	OSStatus st = CMSampleBufferCreateReady(NULL, block, self.format, 1, 0, NULL, 1, &sampleSize, &sample);
	CFRelease(block);
	if (st != noErr || !sample) return;
	CFArrayRef attachments = CMSampleBufferGetSampleAttachmentsArray(sample, YES);
	if (attachments && CFArrayGetCount(attachments) > 0) {
		CFMutableDictionaryRef a = (CFMutableDictionaryRef)CFArrayGetValueAtIndex(attachments, 0);
		CFDictionarySetValue(a, kCMSampleAttachmentKey_DisplayImmediately, kCFBooleanTrue);
	}
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
	AVSampleBufferDisplayLayer *layer = self.view.video;
	if (layer.status == AVQueuedSampleBufferRenderingStatusFailed) {
		[layer flush];
		action(self.handle, @"keyframe", @"");
	}
	[layer enqueueSampleBuffer:sample];
#pragma clang diagnostic pop
	RVRecorder *rec = self.recorder;
	if (rec) [rec appendH264:sample];
	CFRelease(sample);
}

// enqueueImage draws one decoded picture, on the session's queue.
- (void)enqueueImage:(CVPixelBufferRef)pb {
	if (!self.imageFormat || !CMVideoFormatDescriptionMatchesImageBuffer(self.imageFormat, pb)) {
		CMVideoFormatDescriptionRef fmt = NULL;
		if (CMVideoFormatDescriptionCreateForImageBuffer(NULL, pb, &fmt) != noErr) return;
		if (self.imageFormat) CFRelease(self.imageFormat);
		self.imageFormat = fmt;
		size_t w = CVPixelBufferGetWidth(pb), h = CVPixelBufferGetHeight(pb);
		dispatch_async(dispatch_get_main_queue(), ^{
			self.view.picture = CGSizeMake(w, h);
			[self.view setNeedsLayout:YES];
			[self.view remakeCursor];
		});
	}
	CMSampleTimingInfo timing = {kCMTimeInvalid, CMClockGetTime(CMClockGetHostTimeClock()), kCMTimeInvalid};
	CMSampleBufferRef sample = NULL;
	if (CMSampleBufferCreateReadyWithImageBuffer(NULL, pb, self.imageFormat, &timing, &sample) != noErr || !sample) return;
	CFArrayRef attachments = CMSampleBufferGetSampleAttachmentsArray(sample, YES);
	if (attachments && CFArrayGetCount(attachments) > 0) {
		CFMutableDictionaryRef a = (CFMutableDictionaryRef)CFArrayGetValueAtIndex(attachments, 0);
		CFDictionarySetValue(a, kCMSampleAttachmentKey_DisplayImmediately, kCFBooleanTrue);
	}
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
	AVSampleBufferDisplayLayer *layer = self.view.video;
	if (layer.status == AVQueuedSampleBufferRenderingStatusFailed) [layer flush];
	[layer enqueueSampleBuffer:sample];
#pragma clang diagnostic pop
	RVRecorder *rec = self.recorder;
	if (rec) [rec appendImage:pb];
	CFRelease(sample);
}

// pixelBuffer copies an I420 picture into a new NV12 buffer (BT.601,
// video range: what the hosts' converters write), from the pool.
- (CVPixelBufferRef)pixelBufferWidth:(int)w height:(int)h y:(const uint8_t *)y u:(const uint8_t *)u v:(const uint8_t *)v strideY:(int)sy strideUV:(int)suv {
	CVPixelBufferRef pb = NULL;
	@synchronized(self) {
		if (!self.pool || self.poolSize.width != w || self.poolSize.height != h) {
			if (self.pool) CVPixelBufferPoolRelease(self.pool);
			self.pool = NULL;
			NSDictionary *attrs = @{
				(id)kCVPixelBufferPixelFormatTypeKey : @(kCVPixelFormatType_420YpCbCr8BiPlanarVideoRange),
				(id)kCVPixelBufferWidthKey : @(w),
				(id)kCVPixelBufferHeightKey : @(h),
				(id)kCVPixelBufferIOSurfacePropertiesKey : @{},
			};
			CVPixelBufferPoolRef pool = NULL;
			if (CVPixelBufferPoolCreate(NULL, NULL, (__bridge CFDictionaryRef)attrs, &pool) != kCVReturnSuccess) return NULL;
			self.pool = pool;
			self.poolSize = CGSizeMake(w, h);
		}
		if (CVPixelBufferPoolCreatePixelBuffer(NULL, self.pool, &pb) != kCVReturnSuccess) return NULL;
	}
	CVPixelBufferLockBaseAddress(pb, 0);
	uint8_t *dy = CVPixelBufferGetBaseAddressOfPlane(pb, 0);
	size_t dsy = CVPixelBufferGetBytesPerRowOfPlane(pb, 0);
	for (int r = 0; r < h; r++) memcpy(dy + r * dsy, y + (size_t)r * sy, w);
	uint8_t *duv = CVPixelBufferGetBaseAddressOfPlane(pb, 1);
	size_t dsuv = CVPixelBufferGetBytesPerRowOfPlane(pb, 1);
	int cw = (w + 1) / 2, ch = (h + 1) / 2;
	for (int r = 0; r < ch; r++) {
		uint8_t *d = duv + r * dsuv;
		const uint8_t *su = u + (size_t)r * suv, *sv = v + (size_t)r * suv;
		for (int c = 0; c < cw; c++) {
			d[2 * c] = su[c];
			d[2 * c + 1] = sv[c];
		}
	}
	CVPixelBufferUnlockBaseAddress(pb, 0);
	CVBufferSetAttachment(pb, kCVImageBufferYCbCrMatrixKey, kCVImageBufferYCbCrMatrix_ITU_R_601_4, kCVAttachmentMode_ShouldPropagate);
	CVBufferSetAttachment(pb, kCVImageBufferColorPrimariesKey, kCVImageBufferColorPrimaries_SMPTE_C, kCVAttachmentMode_ShouldPropagate);
	CVBufferSetAttachment(pb, kCVImageBufferTransferFunctionKey, kCVImageBufferTransferFunction_ITU_R_709_2, kCVAttachmentMode_ShouldPropagate);
	return pb;
}

@end

#pragma mark - Tabs

// RVTab is one tab of the main window's strip: Home, or a session.
@interface RVTab : NSView
@property(nonatomic, weak) RVSession *session; // nil: Home
@property(nonatomic) BOOL selected;
@property(nonatomic, strong) NSTextField *title;
@property(nonatomic, strong) NSButton *close;
@end

@implementation RVTab
- (instancetype)initWithTitle:(NSString *)title symbol:(NSString *)symbol session:(RVSession *)s {
	self = [super init];
	_session = s;
	self.wantsLayer = YES;
	self.layer.cornerRadius = 8;
	NSImageView *icon = [NSImageView imageViewWithImage:[NSImage imageWithSystemSymbolName:symbol accessibilityDescription:nil]];
	icon.contentTintColor = s ? C_MUTED : C_ACCENT;
	_title = label(title, 12.5, NSFontWeightMedium, C_TEXT);
	NSMutableArray *views = [NSMutableArray arrayWithObjects:icon, _title, nil];
	if (s) {
		_close = [NSButton buttonWithImage:[NSImage imageWithSystemSymbolName:@"xmark" accessibilityDescription:@"close"] target:self action:@selector(closeTab:)];
		_close.bordered = NO;
		_close.contentTintColor = C_FAINT;
		[views addObject:_close];
	}
	NSStackView *row = [NSStackView stackViewWithViews:views];
	row.spacing = 7;
	row.edgeInsets = NSEdgeInsetsMake(0, 12, 0, s ? 8 : 14);
	row.translatesAutoresizingMaskIntoConstraints = NO;
	[self addSubview:row];
	[NSLayoutConstraint activateConstraints:@[
		[row.leadingAnchor constraintEqualToAnchor:self.leadingAnchor],
		[row.trailingAnchor constraintEqualToAnchor:self.trailingAnchor],
		[row.topAnchor constraintEqualToAnchor:self.topAnchor],
		[row.bottomAnchor constraintEqualToAnchor:self.bottomAnchor],
		[self.heightAnchor constraintEqualToConstant:30],
		[self.widthAnchor constraintLessThanOrEqualToConstant:240],
	]];
	return self;
}
- (void)setSelected:(BOOL)selected {
	_selected = selected;
	self.layer.backgroundColor = selected ? C_SELECTED.CGColor : NSColor.clearColor.CGColor;
	self.title.textColor = selected ? C_TEXT : C_MUTED;
}
- (void)mouseDown:(NSEvent *)e {
	[gMain select:self.session];
}
- (void)closeTab:(id)sender {
	RVSession *s = self.session;
	if (s && !s.closed) {
		s.closed = YES;
		action(s.handle, @"closed", @"");
	}
}
@end

#pragma mark - Home's tiles

// RVTile is a computer in My computers or a recent session: a card (its
// picture, its name over it), a row of the list, or a row of the detailed
// list.
@interface RVTile : NSView
@property(nonatomic, strong) NSString *ident;
@property(nonatomic, strong) NSString *name;
@property(nonatomic) BOOL linked;
@property(nonatomic) BOOL favorite;
@property(nonatomic) int mode; // 0 card, 1 list, 2 detailed list
@property(nonatomic, strong) NSTrackingArea *tracking;
@property(nonatomic, strong) NSView *ring;
@end

static const CGFloat kTileW = 200, kTileH = 150, kRowH = 56, kDetailH = 38, kGap = 14;

static NSButton *iconButton(NSString *symbol, id target, SEL a, NSColor *tint) {
	NSButton *b = [NSButton buttonWithImage:[NSImage imageWithSystemSymbolName:symbol accessibilityDescription:nil] target:target action:a];
	b.bordered = NO;
	b.contentTintColor = tint;
	return b;
}

static NSImage *thumbOf(NSDictionary *t) {
	NSString *b64 = [t[@"thumb"] isKindOfClass:NSString.class] ? t[@"thumb"] : nil;
	NSData *jpeg = b64.length ? [[NSData alloc] initWithBase64EncodedString:b64 options:0] : nil;
	return jpeg ? [[NSImage alloc] initWithData:jpeg] : nil;
}

@implementation RVTile
- (instancetype)initWith:(NSDictionary *)t linked:(BOOL)linked mode:(int)mode width:(CGFloat)width {
	CGFloat h = mode == 0 ? kTileH : mode == 1 ? kRowH : kDetailH;
	CGFloat w = mode == 0 ? kTileW : width;
	self = [super initWithFrame:NSMakeRect(0, 0, w, h)];
	_ident = t[@"id"];
	_name = t[@"name"] ?: t[@"title"];
	_linked = linked;
	_mode = mode;
	_favorite = [t[@"favorite"] boolValue];
	self.wantsLayer = YES;
	self.layer.cornerRadius = mode == 0 ? 10 : 8;
	self.layer.masksToBounds = YES;
	self.layer.backgroundColor = (mode == 2 ? NSColor.clearColor : C_SURFACE).CGColor;
	NSImage *img = thumbOf(t);
	NSString *sym = linked ? @"desktopcomputer" : @"display";
	NSColor *iconTint = mode == 0 ? NSColor.whiteColor : C_FAINT;

	if (mode == 0) {
		NSView *shot = [[NSView alloc] initWithFrame:self.bounds];
		shot.wantsLayer = YES;
		shot.layer.backgroundColor = C_SURFACE2.CGColor;
		if (img) {
			shot.layer.contents = img;
			shot.layer.contentsGravity = kCAGravityResizeAspectFill;
		} else {
			NSImageView *icon = [NSImageView imageViewWithImage:[NSImage imageWithSystemSymbolName:sym accessibilityDescription:nil]];
			icon.symbolConfiguration = [NSImageSymbolConfiguration configurationWithPointSize:30 weight:NSFontWeightUltraLight];
			icon.contentTintColor = C_FAINT;
			icon.frame = NSMakeRect(0, 0, kTileW, kTileH - 34);
			[shot addSubview:icon];
		}
		[self addSubview:shot];
		NSView *band = [[NSView alloc] initWithFrame:NSMakeRect(0, kTileH - 36, kTileW, 36)];
		band.wantsLayer = YES;
		band.layer.backgroundColor = [NSColor colorWithWhite:0 alpha:0.72].CGColor;
		[self addSubview:band];
		NSImageView *mon = [NSImageView imageViewWithImage:[NSImage imageWithSystemSymbolName:sym accessibilityDescription:nil]];
		mon.contentTintColor = C_TEXT;
		mon.frame = NSMakeRect(9, kTileH - 27, 16, 16);
		[self addSubview:mon];
		NSTextField *title = label(t[@"title"], 12.5, NSFontWeightSemibold, NSColor.whiteColor);
		if (!linked) title.font = [NSFont monospacedDigitSystemFontOfSize:12.5 weight:NSFontWeightSemibold];
		title.frame = NSMakeRect(30, kTileH - 28, kTileW - 60, 18);
		title.toolTip = t[@"subtitle"];
		[self addSubview:title];
	} else if (mode == 1) {
		NSView *shot = [[NSView alloc] initWithFrame:NSMakeRect(8, 7, 70, 42)];
		shot.wantsLayer = YES;
		shot.layer.cornerRadius = 5;
		shot.layer.masksToBounds = YES;
		shot.layer.backgroundColor = C_VIDEO.CGColor;
		if (img) {
			shot.layer.contents = img;
			shot.layer.contentsGravity = kCAGravityResizeAspectFill;
		}
		[self addSubview:shot];
		NSTextField *title = label(t[@"title"], 13, NSFontWeightSemibold, C_TEXT);
		if (!linked) title.font = [NSFont monospacedDigitSystemFontOfSize:13 weight:NSFontWeightSemibold];
		title.frame = NSMakeRect(90, 9, w - 160, 18);
		[self addSubview:title];
		NSTextField *sub = label(t[@"subtitle"], 11.5, NSFontWeightRegular, C_FAINT);
		sub.frame = NSMakeRect(90, 29, w - 160, 16);
		[self addSubview:sub];
	} else {
		// Columns: name, code, system, last connection.
		NSImageView *icon = [NSImageView imageViewWithImage:[NSImage imageWithSystemSymbolName:sym accessibilityDescription:nil]];
		icon.contentTintColor = iconTint;
		icon.frame = NSMakeRect(10, 11, 16, 16);
		[self addSubview:icon];
		CGFloat cols[4] = {34, w * 0.36, w * 0.58, w * 0.74};
		NSArray *texts = @[ t[@"name"] ?: @"", t[@"code"] ?: @"", t[@"system"] ?: @"", t[@"when"] ?: @"" ];
		for (int i = 0; i < 4; i++) {
			NSTextField *c = label(texts[i], 12.5, i == 0 ? NSFontWeightMedium : NSFontWeightRegular, i == 0 ? C_TEXT : C_MUTED);
			if (i == 1) c.font = [NSFont monospacedDigitSystemFontOfSize:12.5 weight:NSFontWeightRegular];
			CGFloat end = i < 3 ? cols[i + 1] - 10 : w - 70;
			c.frame = NSMakeRect(cols[i], 10, MAX(20, end - cols[i]), 18);
			[self addSubview:c];
		}
		NSBox *line = [[NSBox alloc] initWithFrame:NSMakeRect(0, kDetailH - 1, w, 1)];
		line.boxType = NSBoxCustom;
		line.borderWidth = 0;
		line.fillColor = C_BORDER;
		[self addSubview:line];
	}
	CGFloat midY = mode == 1 ? 17 : 8;
	if (linked) {
		NSButton *star = iconButton(_favorite ? @"star.fill" : @"star", self, @selector(star:), _favorite ? C_ACCENT : iconTint);
		star.frame = mode == 0 ? NSMakeRect(kTileW - 28, 6, 22, 22) : NSMakeRect(w - 62, midY, 22, 22);
		[self addSubview:star];
	}
	NSButton *more = iconButton(@"ellipsis", self, @selector(more:), iconTint);
	more.frame = mode == 0 ? NSMakeRect(kTileW - 30, kTileH - 29, 24, 22) : NSMakeRect(w - 34, midY, 24, 22);
	[self addSubview:more];
	self.ring = [[NSView alloc] initWithFrame:self.bounds];
	self.ring.wantsLayer = YES;
	self.ring.layer.cornerRadius = self.layer.cornerRadius;
	self.ring.layer.borderWidth = mode == 0 ? 2 : 1;
	self.ring.layer.borderColor = C_ACCENT.CGColor;
	self.ring.hidden = YES;
	[self addSubview:self.ring positioned:NSWindowBelow relativeTo:more];
	return self;
}
- (BOOL)isFlipped {
	return YES;
}
- (NSView *)hitTest:(NSPoint)p {
	NSView *v = [super hitTest:p];
	return [v isKindOfClass:NSButton.class] ? v : (v ? self : nil);
}
- (void)updateTrackingAreas {
	[super updateTrackingAreas];
	if (self.tracking) [self removeTrackingArea:self.tracking];
	self.tracking = [[NSTrackingArea alloc] initWithRect:NSZeroRect options:NSTrackingMouseEnteredAndExited | NSTrackingActiveInKeyWindow | NSTrackingInVisibleRect owner:self userInfo:nil];
	[self addTrackingArea:self.tracking];
}
- (void)mouseEntered:(NSEvent *)e {
	if (self.mode == 0) self.ring.hidden = NO;
	else self.layer.backgroundColor = C_SURFACE2.CGColor;
}
- (void)mouseExited:(NSEvent *)e {
	self.ring.hidden = YES;
	self.layer.backgroundColor = (self.mode == 2 ? NSColor.clearColor : C_SURFACE).CGColor;
}
- (void)resetCursorRects {
	[self addCursorRect:self.bounds cursor:NSCursor.pointingHandCursor];
}
- (void)mouseUp:(NSEvent *)e {
	if (NSPointInRect([self convertPoint:e.locationInWindow fromView:nil], self.bounds)) {
		action(gMain.handle, self.linked ? @"device" : @"recent", self.ident);
	}
}
- (void)rightMouseDown:(NSEvent *)e {
	[NSMenu popUpContextMenu:[self menu] withEvent:e forView:self];
}
- (void)star:(id)sender {
	action(gMain.handle, @"favorite", self.ident);
}
- (NSMenu *)menu {
	NSMenu *m = [[NSMenu alloc] init];
	NSMenuItem *c = [m addItemWithTitle:gMain.labels[@"connect"] ?: @"Connect" action:@selector(connectNow:) keyEquivalent:@""];
	c.target = self;
	if (self.linked) {
		NSMenuItem *f = [m addItemWithTitle:self.favorite ? (gMain.labels[@"unfavorite"] ?: @"Remove from favorites") : (gMain.labels[@"favorite"] ?: @"Add to favorites") action:@selector(star:) keyEquivalent:@""];
		f.target = self;
		NSMenuItem *r = [m addItemWithTitle:gMain.labels[@"rename"] ?: @"Rename…" action:@selector(rename:) keyEquivalent:@""];
		r.target = self;
		[m addItem:NSMenuItem.separatorItem];
		NSMenuItem *x = [m addItemWithTitle:gMain.labels[@"forget"] ?: @"Forget" action:@selector(forget:) keyEquivalent:@""];
		x.target = self;
	} else {
		NSMenuItem *cp = [m addItemWithTitle:gMain.labels[@"copyCode"] ?: @"Copy the code" action:@selector(copyIdent:) keyEquivalent:@""];
		cp.target = self;
		[m addItem:NSMenuItem.separatorItem];
		NSMenuItem *x = [m addItemWithTitle:gMain.labels[@"removeRecent"] ?: @"Remove" action:@selector(removeRecent:) keyEquivalent:@""];
		x.target = self;
	}
	return m;
}
- (void)more:(NSButton *)sender {
	[[self menu] popUpMenuPositioningItem:nil atLocation:NSMakePoint(0, sender.bounds.size.height + 2) inView:sender];
}
- (void)connectNow:(id)s {
	action(gMain.handle, self.linked ? @"device" : @"recent", self.ident);
}
- (void)copyIdent:(id)s {
	[NSPasteboard.generalPasteboard clearContents];
	[NSPasteboard.generalPasteboard setString:self.ident forType:NSPasteboardTypeString];
}
- (void)removeRecent:(id)s {
	action(gMain.handle, @"remove-recent", self.ident);
}
- (void)rename:(id)s {
	NSAlert *a = [[NSAlert alloc] init];
	a.messageText = gMain.labels[@"renameTitle"] ?: @"Rename";
	NSTextField *field = [[NSTextField alloc] initWithFrame:NSMakeRect(0, 0, 260, 24)];
	field.stringValue = self.name ?: @"";
	a.accessoryView = field;
	[a addButtonWithTitle:gMain.labels[@"save"] ?: @"Save"];
	[a addButtonWithTitle:gMain.labels[@"cancel"] ?: @"Cancel"];
	a.window.initialFirstResponder = field;
	NSString *ident = self.ident;
	[a beginSheetModalForWindow:gMain.window
	          completionHandler:^(NSModalResponse r) {
		          if (r == NSAlertFirstButtonReturn) action(gMain.handle, @"rename", [NSString stringWithFormat:@"%@\n%@", ident, field.stringValue]);
	          }];
}
- (void)forget:(id)s {
	action(gMain.handle, @"forget", self.ident);
}
@end

// RVGrid lays the items out: cards in rows, list rows in two columns, or
// the detailed list's rows one under the other.
@interface RVGrid : NSView
@property(nonatomic, strong) NSArray<NSView *> *items;
@property(nonatomic) int mode;
@end

@implementation RVGrid
- (BOOL)isFlipped {
	return YES;
}
+ (CGFloat)rowWidth:(CGFloat)width mode:(int)mode {
	CGFloat inner = width - 56;
	if (mode == 1 && inner >= 2 * 300 + kGap) return (inner - kGap) / 2;
	return inner;
}
- (void)relayout {
	CGFloat width = self.enclosingScrollView.contentSize.width;
	CGFloat pad = 28, x = pad, y = 6, rowH = 0;
	for (NSView *v in self.items) {
		NSSize size = v.frame.size;
		if (![v isKindOfClass:RVTile.class] && size.height < 20) size.height = MAX(60, v.fittingSize.height);
		if (![v isKindOfClass:RVTile.class] || self.mode == 2) {
			if (rowH > 0) {
				y += rowH + (self.mode == 2 ? 0 : kGap);
				rowH = 0;
			}
			x = pad;
			v.frame = NSMakeRect(pad, y, width - 2 * pad, size.height);
			y += size.height + (self.mode == 2 ? 0 : kGap);
			continue;
		}
		if (x + size.width > width - pad + 1 && x > pad) {
			x = pad;
			y += rowH + (self.mode == 1 ? 8 : kGap);
		}
		v.frame = NSMakeRect(x, y, size.width, size.height);
		x += size.width + kGap;
		rowH = size.height;
	}
	y += rowH;
	[self setFrameSize:NSMakeSize(width, MAX(y + pad, self.enclosingScrollView.contentSize.height))];
}
- (void)setItems:(NSArray<NSView *> *)items {
	for (NSView *v in _items) [v removeFromSuperview];
	_items = items;
	for (NSView *v in items) [self addSubview:v];
	[self relayout];
}
@end

// RVCopyField is a text that copies its value when clicked.
@interface RVCopyField : NSTextField
@property(nonatomic, strong) NSString *value;
- (void)copyValue;
@end

@implementation RVCopyField
- (void)resetCursorRects {
	if (self.value.length) [self addCursorRect:self.bounds cursor:NSCursor.pointingHandCursor];
}
- (void)mouseDown:(NSEvent *)e {
	[self copyValue];
}
- (void)copyValue {
	if (self.value.length == 0) return;
	[NSPasteboard.generalPasteboard clearContents];
	[NSPasteboard.generalPasteboard setString:self.value forType:NSPasteboardTypeString];
	[gMain flashCopied];
}
@end

#pragma mark - The main window

@implementation RVMain

- (NSString *)L:(NSString *)key {
	return self.labels[key] ?: key;
}

- (void)build {
	NSRect frame = NSMakeRect(0, 0, 1180, 760);
	NSScreen *screen = NSScreen.mainScreen;
	if (screen) {
		frame.size.width = MIN(frame.size.width, screen.visibleFrame.size.width * 0.92);
		frame.size.height = MIN(frame.size.height, screen.visibleFrame.size.height * 0.92);
	}
	NSWindow *w = [[NSWindow alloc] initWithContentRect:frame
	                                          styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable | NSWindowStyleMaskFullSizeContentView
	                                            backing:NSBackingStoreBuffered
	                                              defer:NO];
	w.releasedWhenClosed = NO;
	w.title = @"go-link Desk";
	w.titleVisibility = NSWindowTitleHidden;
	w.titlebarAppearsTransparent = YES;
	w.appearance = APPEARANCE;
	w.backgroundColor = C_BG;
	w.minSize = NSMakeSize(820, 540);
	w.delegate = self;
	w.tabbingMode = NSWindowTabbingModeDisallowed;
	w.collectionBehavior |= NSWindowCollectionBehaviorFullScreenPrimary;
	self.window = w;
	self.sessions = [NSMutableArray array];
	self.filter = @"all";
	self.viewMode = (int)MIN(2, MAX(0, [NSUserDefaults.standardUserDefaults integerForKey:@"org.go-link.desk.view"]));
	[self buildContent];
	[w center];
	[w setFrameAutosaveName:@"org.go-link.desk.main"];
}

// buildContent builds the window's insides in the theme's colors; built
// again when the theme changes, keeping the sessions' pages.
- (void)buildContent {
	NSWindow *w = self.window;
	w.appearance = APPEARANCE;
	w.backgroundColor = C_BG;
	NSView *root = [[NSView alloc] init];
	root.wantsLayer = YES;
	root.layer.backgroundColor = C_BG.CGColor;
	w.contentView = root;

	// The top bar, in the title bar: the address, then the tabs.
	NSView *top = [[NSView alloc] init];
	top.wantsLayer = YES;
	top.layer.backgroundColor = C_SURFACE.CGColor;
	top.translatesAutoresizingMaskIntoConstraints = NO;

	self.address = [[NSTextField alloc] init];
	self.address.placeholderAttributedString = [[NSAttributedString alloc] initWithString:[self L:@"address"] attributes:@{NSForegroundColorAttributeName : C_PLACEHOLDER, NSFontAttributeName : [NSFont systemFontOfSize:13]}];
	self.address.font = [NSFont monospacedDigitSystemFontOfSize:14 weight:NSFontWeightMedium];
	self.address.textColor = C_TEXT;
	self.address.bezeled = NO;
	self.address.drawsBackground = NO;
	self.address.focusRingType = NSFocusRingTypeNone;
	self.address.target = self;
	self.address.action = @selector(connect:);
	self.address.translatesAutoresizingMaskIntoConstraints = NO;
	NSView *field = [[NSView alloc] init];
	field.wantsLayer = YES;
	field.layer.cornerRadius = 7;
	field.layer.backgroundColor = C_BG.CGColor;
	field.layer.borderWidth = 1;
	field.layer.borderColor = C_FIELD_BORDER.CGColor;
	field.translatesAutoresizingMaskIntoConstraints = NO;
	NSImageView *glyph = [NSImageView imageViewWithImage:[NSImage imageWithSystemSymbolName:@"display" accessibilityDescription:nil]];
	glyph.contentTintColor = C_FAINT;
	glyph.translatesAutoresizingMaskIntoConstraints = NO;
	self.door = [[NSPopUpButton alloc] init];
	self.door.bordered = NO;
	self.door.controlSize = NSControlSizeSmall;
	self.door.font = [NSFont systemFontOfSize:11];
	[self.door addItemsWithTitles:@[ [self L:@"doorSession"], [self L:@"doorLink"] ]];
	self.door.toolTip = [self L:@"doorTip"];
	self.door.translatesAutoresizingMaskIntoConstraints = NO;
	self.connect = [RVButton withTitle:[self L:@"connect"] symbol:@"arrow.right" target:self action:@selector(connect:) primary:YES];
	self.connect.imagePosition = NSImageTrailing;
	self.connect.translatesAutoresizingMaskIntoConstraints = NO;
	// No other computer at hand: go-link Desk's demo, always on.
	self.demo = [RVButton withTitle:[self L:@"demo"] symbol:@"play.circle" target:self action:@selector(demo:) primary:NO];
	self.demo.translatesAutoresizingMaskIntoConstraints = NO;
	[field addSubview:glyph];
	[field addSubview:self.address];
	[field addSubview:self.door];
	RVButton *settings = [RVButton withTitle:@"" symbol:@"gearshape" target:self action:@selector(settings:) primary:NO];
	settings.toolTip = [self L:@"settings"];
	settings.fill = NSColor.clearColor;
	[settings restyle];
	settings.translatesAutoresizingMaskIntoConstraints = NO;
	[top addSubview:field];
	[top addSubview:self.connect];
	[top addSubview:self.demo];
	[top addSubview:settings];
	[NSLayoutConstraint activateConstraints:@[
		[field.leadingAnchor constraintEqualToAnchor:top.leadingAnchor constant:86],
		[field.topAnchor constraintEqualToAnchor:top.topAnchor constant:8],
		[field.heightAnchor constraintEqualToConstant:30],
		[field.widthAnchor constraintLessThanOrEqualToConstant:520],
		[self.connect.leadingAnchor constraintEqualToAnchor:field.trailingAnchor constant:8],
		[self.connect.centerYAnchor constraintEqualToAnchor:field.centerYAnchor],
		[self.connect.heightAnchor constraintEqualToConstant:30],
		[self.demo.leadingAnchor constraintEqualToAnchor:self.connect.trailingAnchor constant:8],
		[self.demo.centerYAnchor constraintEqualToAnchor:field.centerYAnchor],
		[self.demo.heightAnchor constraintEqualToConstant:30],
		[settings.leadingAnchor constraintGreaterThanOrEqualToAnchor:self.demo.trailingAnchor constant:8],
		[settings.trailingAnchor constraintEqualToAnchor:top.trailingAnchor constant:-12],
		[settings.centerYAnchor constraintEqualToAnchor:field.centerYAnchor],
		[glyph.leadingAnchor constraintEqualToAnchor:field.leadingAnchor constant:10],
		[glyph.centerYAnchor constraintEqualToAnchor:field.centerYAnchor],
		[self.address.leadingAnchor constraintEqualToAnchor:glyph.trailingAnchor constant:8],
		[self.address.centerYAnchor constraintEqualToAnchor:field.centerYAnchor],
		[self.door.leadingAnchor constraintEqualToAnchor:self.address.trailingAnchor constant:6],
		[self.door.trailingAnchor constraintEqualToAnchor:field.trailingAnchor constant:-6],
		[self.door.centerYAnchor constraintEqualToAnchor:field.centerYAnchor],
	]];
	NSLayoutConstraint *wide = [field.widthAnchor constraintEqualToConstant:520];
	wide.priority = NSLayoutPriorityDefaultLow;
	wide.active = YES;
	[self.door setContentHuggingPriority:NSLayoutPriorityRequired forOrientation:NSLayoutConstraintOrientationHorizontal];

	self.tabs = [NSStackView stackViewWithViews:@[]];
	self.tabs.spacing = 2;
	self.tabs.translatesAutoresizingMaskIntoConstraints = NO;
	[top addSubview:self.tabs];
	NSBox *line = [[NSBox alloc] init];
	line.boxType = NSBoxCustom;
	line.borderWidth = 0;
	line.fillColor = C_BORDER;
	line.translatesAutoresizingMaskIntoConstraints = NO;
	[top addSubview:line];
	[NSLayoutConstraint activateConstraints:@[
		[self.tabs.leadingAnchor constraintEqualToAnchor:top.leadingAnchor constant:10],
		[self.tabs.trailingAnchor constraintLessThanOrEqualToAnchor:top.trailingAnchor constant:-10],
		[self.tabs.topAnchor constraintEqualToAnchor:field.bottomAnchor constant:8],
		[line.topAnchor constraintEqualToAnchor:self.tabs.bottomAnchor],
		[line.leadingAnchor constraintEqualToAnchor:top.leadingAnchor],
		[line.trailingAnchor constraintEqualToAnchor:top.trailingAnchor],
		[line.bottomAnchor constraintEqualToAnchor:top.bottomAnchor],
		[line.heightAnchor constraintEqualToConstant:1],
	]];

	self.content = [[NSView alloc] init];
	self.content.translatesAutoresizingMaskIntoConstraints = NO;
	[root addSubview:top];
	[root addSubview:self.content];
	[NSLayoutConstraint activateConstraints:@[
		[top.topAnchor constraintEqualToAnchor:root.topAnchor],
		[top.leadingAnchor constraintEqualToAnchor:root.leadingAnchor],
		[top.trailingAnchor constraintEqualToAnchor:root.trailingAnchor],
		[self.content.topAnchor constraintEqualToAnchor:top.bottomAnchor],
		[self.content.leadingAnchor constraintEqualToAnchor:root.leadingAnchor],
		[self.content.trailingAnchor constraintEqualToAnchor:root.trailingAnchor],
		[self.content.bottomAnchor constraintEqualToAnchor:root.bottomAnchor],
	]];
	[self buildHome];
	if (self.lastState) [self setState:self.lastState];
	for (RVSession *s in self.sessions) {
		[s restyle];
		[self show:s.page]; // every page back in the window
	}
	[self renderTabs];
	[self show:self.selected ? self.selected.page : self.home];
	// A picture moved to the new window starts again from a whole frame.
	RVSession *sel = self.selected;
	if (sel) {
		sel.shown = NO;
		sel.shown = self.window.isVisible;
	}
}

- (void)buildHome {
	NSView *home = [[NSView alloc] init];

	// This computer, at the top: its code, big, in a panel of the brand's
	// color. Clicking the code or the link copies it.
	NSTextField *thisTitle = label([[self L:@"thisComputer"] uppercaseString], 11, NSFontWeightSemibold, C_TINT_TEXT);
	self.code = [[RVCopyField alloc] init];
	self.code.stringValue = @"— — —";
	self.code.font = [NSFont monospacedDigitSystemFontOfSize:40 weight:NSFontWeightBold];
	self.code.textColor = C_ACCENT;
	self.code.bezeled = NO;
	self.code.drawsBackground = NO;
	self.code.editable = NO;
	self.code.selectable = NO;
	self.code.toolTip = [self L:@"copyHint"];
	[self.code setContentCompressionResistancePriority:NSLayoutPriorityRequired forOrientation:NSLayoutConstraintOrientationVertical];
	self.linkLabel = [[RVCopyField alloc] init];
	self.linkLabel.font = [NSFont systemFontOfSize:12];
	self.linkLabel.textColor = C_MUTED;
	self.linkLabel.bezeled = NO;
	self.linkLabel.drawsBackground = NO;
	self.linkLabel.editable = NO;
	self.linkLabel.selectable = NO;
	self.linkLabel.lineBreakMode = NSLineBreakByTruncatingMiddle;
	self.linkLabel.toolTip = [self L:@"copyHint"];
	[self.linkLabel.widthAnchor constraintLessThanOrEqualToConstant:420].active = YES;
	NSImageView *linkIcon = [NSImageView imageViewWithImage:[NSImage imageWithSystemSymbolName:@"link" accessibilityDescription:nil]];
	linkIcon.contentTintColor = C_FAINT;
	NSStackView *linkRow = [NSStackView stackViewWithViews:@[ linkIcon, self.linkLabel ]];
	linkRow.spacing = 6;
	self.copied = label([self L:@"copied"], 12, NSFontWeightSemibold, C_OK);
	self.copied.hidden = YES;
	NSStackView *codeLine = [NSStackView stackViewWithViews:@[ self.code, self.copied ]];
	codeLine.spacing = 12;
	NSTextField *hint = wrapping([self L:@"hint"], 11.5, C_FAINT, 560);
	NSStackView *left = [NSStackView stackViewWithViews:@[ thisTitle, codeLine, linkRow, hint ]];
	left.orientation = NSUserInterfaceLayoutOrientationVertical;
	left.alignment = NSLayoutAttributeLeading;
	left.spacing = 2;
	[left setCustomSpacing:6 afterView:codeLine];
	[left setCustomSpacing:8 afterView:linkRow];

	self.codeCopyBtn = [RVButton withTitle:@"" symbol:@"doc.on.doc" target:self action:@selector(copyCode:) primary:NO];
	self.codeCopyBtn.toolTip = [self L:@"copy"];
	self.renewBtn = [RVButton withTitle:@"" symbol:@"arrow.clockwise" target:self action:@selector(newCode:) primary:NO];
	self.renewBtn.toolTip = [self L:@"newCode"];
	self.permBtn = [RVButton withTitle:@"" symbol:@"checkmark.shield" target:self action:@selector(permissions:) primary:NO];
	self.permBtn.toolTip = [self L:@"permissions"];
	NSStackView *tools = [NSStackView stackViewWithViews:@[ self.codeCopyBtn, self.renewBtn, self.permBtn ]];
	tools.spacing = 8;
	self.statusDot = [[NSView alloc] init];
	self.statusDot.wantsLayer = YES;
	self.statusDot.layer.cornerRadius = 4;
	[self.statusDot.widthAnchor constraintEqualToConstant:8].active = YES;
	[self.statusDot.heightAnchor constraintEqualToConstant:8].active = YES;
	self.status = label(@"", 12, NSFontWeightRegular, C_MUTED);
	NSStackView *statusRow = [NSStackView stackViewWithViews:@[ self.statusDot, self.status ]];
	statusRow.spacing = 7;
	NSStackView *right = [NSStackView stackViewWithViews:@[ tools, statusRow ]];
	right.orientation = NSUserInterfaceLayoutOrientationVertical;
	right.alignment = NSLayoutAttributeTrailing;
	right.spacing = 12;
	// The panel: the code on the left, the tools on the right, each pinned
	// to the panel's edges (its height follows the taller of the two).
	NSView *panel = [[NSView alloc] init];
	panel.wantsLayer = YES;
	panel.layer.cornerRadius = 14;
	panel.layer.backgroundColor = C_TINT_BG.CGColor;
	panel.layer.borderWidth = 1;
	panel.layer.borderColor = C_TINT_BORDER.CGColor;
	left.translatesAutoresizingMaskIntoConstraints = NO;
	right.translatesAutoresizingMaskIntoConstraints = NO;
	[panel addSubview:left];
	[panel addSubview:right];
	[left setContentHuggingPriority:NSLayoutPriorityRequired forOrientation:NSLayoutConstraintOrientationVertical];
	[NSLayoutConstraint activateConstraints:@[
		[left.topAnchor constraintEqualToAnchor:panel.topAnchor constant:18],
		[left.leadingAnchor constraintEqualToAnchor:panel.leadingAnchor constant:24],
		[left.bottomAnchor constraintLessThanOrEqualToAnchor:panel.bottomAnchor constant:-18],
		[right.topAnchor constraintEqualToAnchor:panel.topAnchor constant:18],
		[right.trailingAnchor constraintEqualToAnchor:panel.trailingAnchor constant:-24],
		[right.bottomAnchor constraintLessThanOrEqualToAnchor:panel.bottomAnchor constant:-18],
		[right.leadingAnchor constraintGreaterThanOrEqualToAnchor:left.trailingAnchor constant:16],
	]];
	NSLayoutConstraint *fit = [panel.bottomAnchor constraintEqualToAnchor:left.bottomAnchor constant:18];
	fit.priority = NSLayoutPriorityDefaultHigh;
	fit.active = YES;
	NSView *header = panel;
	header.translatesAutoresizingMaskIntoConstraints = NO;

	// The filters and the view.
	NSMutableArray *filters = [NSMutableArray array];
	self.filterButtons = [NSMutableDictionary dictionary];
	for (NSArray *f in @[ @[ @"all", [self L:@"myComputers"] ], @[ @"favorites", [self L:@"favorites"] ], @[ @"recents", [self L:@"recents"] ] ]) {
		NSButton *b = [NSButton buttonWithTitle:f[1] target:self action:@selector(pickFilter:)];
		b.bordered = NO;
		b.identifier = f[0];
		self.filterButtons[f[0]] = b;
		[filters addObject:b];
	}
	self.clearBtn = [NSButton buttonWithTitle:[self L:@"clear"] target:self action:@selector(clearRecents:)];
	self.clearBtn.bordered = NO;
	self.clearBtn.attributedTitle = [[NSAttributedString alloc] initWithString:[self L:@"clear"] attributes:@{NSForegroundColorAttributeName : C_FAINT, NSFontAttributeName : [NSFont systemFontOfSize:12]}];
	NSView *spacer = [[NSView alloc] init];
	[spacer setContentHuggingPriority:1 forOrientation:NSLayoutConstraintOrientationHorizontal];
	self.gridBtn = iconButton(@"square.grid.2x2", self, @selector(pickView:), C_TEXT);
	self.gridBtn.tag = 0;
	self.gridBtn.toolTip = [self L:@"viewCards"];
	self.listBtn = iconButton(@"rectangle.grid.1x2", self, @selector(pickView:), C_FAINT);
	self.listBtn.tag = 1;
	self.listBtn.toolTip = [self L:@"viewList"];
	self.detailBtn = iconButton(@"list.bullet", self, @selector(pickView:), C_FAINT);
	self.detailBtn.tag = 2;
	self.detailBtn.toolTip = [self L:@"viewDetails"];
	NSStackView *views = [NSStackView stackViewWithViews:@[ self.gridBtn, self.listBtn, self.detailBtn ]];
	views.spacing = 2;
	views.edgeInsets = NSEdgeInsetsMake(3, 6, 3, 6);
	views.wantsLayer = YES;
	views.layer.cornerRadius = 7;
	views.layer.backgroundColor = C_SURFACE.CGColor;
	[filters addObjectsFromArray:@[ spacer, self.clearBtn, views ]];
	NSStackView *filterRow = [NSStackView stackViewWithViews:filters];
	filterRow.spacing = 18;
	filterRow.translatesAutoresizingMaskIntoConstraints = NO;

	self.grid = [[RVGrid alloc] initWithFrame:NSMakeRect(0, 0, 600, 400)];
	self.scroll = [[NSScrollView alloc] init];
	self.scroll.documentView = self.grid;
	self.scroll.hasVerticalScroller = YES;
	self.scroll.drawsBackground = NO;
	self.scroll.automaticallyAdjustsContentInsets = NO;
	self.scroll.translatesAutoresizingMaskIntoConstraints = NO;
	self.scroll.postsFrameChangedNotifications = YES;
	[NSNotificationCenter.defaultCenter addObserver:self selector:@selector(resized:) name:NSViewFrameDidChangeNotification object:self.scroll];

	self.version = label(@"", 11, NSFontWeightRegular, C_FAINT);
	self.version.translatesAutoresizingMaskIntoConstraints = NO;

	[home addSubview:header];
	[home addSubview:filterRow];
	[home addSubview:self.scroll];
	[home addSubview:self.version];
	[NSLayoutConstraint activateConstraints:@[
		[header.topAnchor constraintEqualToAnchor:home.topAnchor constant:24],
		[header.leadingAnchor constraintEqualToAnchor:home.leadingAnchor constant:28],
		[header.trailingAnchor constraintEqualToAnchor:home.trailingAnchor constant:-28],
		[filterRow.topAnchor constraintEqualToAnchor:header.bottomAnchor constant:26],
		[filterRow.leadingAnchor constraintEqualToAnchor:home.leadingAnchor constant:28],
		[filterRow.trailingAnchor constraintEqualToAnchor:home.trailingAnchor constant:-28],
		[self.scroll.topAnchor constraintEqualToAnchor:filterRow.bottomAnchor constant:14],
		[self.scroll.leadingAnchor constraintEqualToAnchor:home.leadingAnchor],
		[self.scroll.trailingAnchor constraintEqualToAnchor:home.trailingAnchor],
		[self.scroll.bottomAnchor constraintEqualToAnchor:self.version.topAnchor constant:-6],
		[self.version.trailingAnchor constraintEqualToAnchor:home.trailingAnchor constant:-14],
		[self.version.bottomAnchor constraintEqualToAnchor:home.bottomAnchor constant:-8],
	]];
	self.home = home;
	[self renderFilters];
	[self renderTiles];
}

- (void)resized:(NSNotification *)n {
	// Rows follow the width; cards only move.
	if (self.viewMode != 0 && fabs(self.scroll.contentSize.width - self.lastWidth) > 1) [self renderTiles];
	else [(RVGrid *)self.grid relayout];
}

- (void)renderFilters {
	for (NSString *k in self.filterButtons) {
		NSButton *b = self.filterButtons[k];
		BOOL on = [k isEqual:self.filter];
		NSString *t = b.title;
		b.attributedTitle = [[NSAttributedString alloc] initWithString:t
		                                                    attributes:@{
			                                                    NSForegroundColorAttributeName : on ? C_TEXT : C_FAINT,
			                                                    NSFontAttributeName : [NSFont systemFontOfSize:14 weight:on ? NSFontWeightSemibold : NSFontWeightRegular],
			                                                    NSUnderlineStyleAttributeName : on ? @(NSUnderlineStyleThick) : @0,
			                                                    NSUnderlineColorAttributeName : C_ACCENT,
		                                                    }];
	}
	self.clearBtn.hidden = ![self.filter isEqual:@"recents"] || self.recents.count == 0;
	self.gridBtn.contentTintColor = self.viewMode == 0 ? C_ACCENT : C_FAINT;
	self.listBtn.contentTintColor = self.viewMode == 1 ? C_ACCENT : C_FAINT;
	self.detailBtn.contentTintColor = self.viewMode == 2 ? C_ACCENT : C_FAINT;
}

- (void)pickFilter:(NSButton *)b {
	self.filter = b.identifier;
	self.chose = YES;
	[self renderFilters];
	[self renderTiles];
}
- (void)pickView:(NSButton *)b {
	self.viewMode = (int)b.tag;
	[NSUserDefaults.standardUserDefaults setInteger:self.viewMode forKey:@"org.go-link.desk.view"];
	[self renderFilters];
	[self renderTiles];
}

- (NSView *)emptyNote:(NSString *)text {
	NSView *empty = [[NSView alloc] init];
	empty.wantsLayer = YES;
	empty.layer.cornerRadius = 10;
	empty.layer.borderWidth = 1;
	empty.layer.borderColor = C_DASHED.CGColor;
	NSTextField *t = wrapping(text, 13, C_MUTED, 560);
	t.translatesAutoresizingMaskIntoConstraints = NO;
	[empty addSubview:t];
	[NSLayoutConstraint activateConstraints:@[
		[t.leadingAnchor constraintEqualToAnchor:empty.leadingAnchor constant:18],
		[t.trailingAnchor constraintLessThanOrEqualToAnchor:empty.trailingAnchor constant:-18],
		[t.topAnchor constraintEqualToAnchor:empty.topAnchor constant:18],
		[t.bottomAnchor constraintEqualToAnchor:empty.bottomAnchor constant:-18],
	]];
	return empty;
}

- (NSView *)detailHeader:(CGFloat)w {
	NSView *h = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, w, 28)];
	CGFloat cols[4] = {34, w * 0.36, w * 0.58, w * 0.74};
	NSArray *names = @[ [self L:@"colName"], [self L:@"colCode"], [self L:@"colSystem"], [self L:@"colWhen"] ];
	for (int i = 0; i < 4; i++) {
		NSTextField *c = label([names[i] uppercaseString], 10.5, NSFontWeightSemibold, C_FAINT);
		c.frame = NSMakeRect(cols[i], 6, 160, 16);
		[h addSubview:c];
	}
	NSBox *line = [[NSBox alloc] initWithFrame:NSMakeRect(0, 27, w, 1)];
	line.boxType = NSBoxCustom;
	line.borderWidth = 0;
	line.fillColor = C_BORDER;
	line.autoresizingMask = NSViewWidthSizable;
	[h addSubview:line];
	return h;
}

- (void)renderTiles {
	CGFloat width = self.scroll.contentSize.width;
	if (width < 100) width = 880;
	self.lastWidth = width;
	CGFloat rowW = [RVGrid rowWidth:width mode:self.viewMode];
	NSMutableArray *items = [NSMutableArray array];
	BOOL recents = [self.filter isEqual:@"recents"];
	BOOL favorites = [self.filter isEqual:@"favorites"];
	NSMutableArray *rows = [NSMutableArray array];
	if (recents) {
		for (NSDictionary *d in self.recents) [rows addObject:[[RVTile alloc] initWith:d linked:NO mode:self.viewMode width:rowW]];
	} else {
		for (NSDictionary *d in self.devices) {
			if (favorites && ![d[@"favorite"] boolValue]) continue;
			[rows addObject:[[RVTile alloc] initWith:d linked:YES mode:self.viewMode width:rowW]];
		}
	}
	if (rows.count == 0) {
		[items addObject:[self emptyNote:[self L:recents ? @"emptyRecents" : favorites ? @"emptyFavorites" : @"empty"]]];
	} else {
		if (self.viewMode == 2) [items addObject:[self detailHeader:rowW]];
		[items addObjectsFromArray:rows];
	}
	RVGrid *g = (RVGrid *)self.grid;
	g.mode = self.viewMode;
	g.items = items;
}

- (void)setState:(NSDictionary *)st {
	self.lastState = st;
	NSString *code = st[@"code"];
	self.code.stringValue = code.length ? code : @"— — —";
	self.link = st[@"link"] ?: @"";
	self.linkLabel.stringValue = self.link;
	self.linkLabel.superview.hidden = self.link.length == 0;
	((RVCopyField *)self.code).value = code;
	((RVCopyField *)self.linkLabel).value = self.link;
	self.status.stringValue = st[@"status"] ?: @"";
	BOOL ok = [st[@"status_ok"] boolValue];
	BOOL busy = [st[@"in_session"] boolValue];
	self.statusDot.layer.backgroundColor = (busy ? C_ACCENT : ok ? C_OK : C_DANGER).CGColor;
	self.codeCopyBtn.enabled = code.length > 0;
	self.renewBtn.enabled = [st[@"can_new"] boolValue];
	self.version.stringValue = st[@"version"] ?: @"";
	self.perms = arr(st[@"perms"]);
	BOOL allGranted = YES;
	for (NSDictionary *p in self.perms) allGranted = allGranted && [p[@"granted"] boolValue];
	self.permBtn.hidden = self.perms.count == 0;
	self.permBtn.image = [NSImage imageWithSystemSymbolName:allGranted ? @"checkmark.shield" : @"exclamationmark.shield" accessibilityDescription:nil];
	self.permBtn.fill = allGranted ? nil : C_TINT_BG;
	[self.permBtn restyle];
	self.permBtn.contentTintColor = allGranted ? C_OK : C_ACCENT;
	self.devices = arr(st[@"devices"]);
	self.recents = arr(st[@"recents"]);
	// With no linked computer yet, the recent sessions are what there is.
	if (!self.chose && self.devices.count == 0 && self.recents.count > 0) self.filter = @"recents";
	[self renderFilters];
	[self renderTiles];
}

// popover shows a small panel under a button, as wide as width.
- (void)popover:(NSView *)content under:(NSView *)anchor width:(CGFloat)width {
	NSViewController *vc = [[NSViewController alloc] init];
	content.translatesAutoresizingMaskIntoConstraints = NO;
	NSView *wrap = [[NSView alloc] init];
	[wrap addSubview:content];
	[NSLayoutConstraint activateConstraints:@[
		[content.leadingAnchor constraintEqualToAnchor:wrap.leadingAnchor constant:16],
		[content.trailingAnchor constraintEqualToAnchor:wrap.trailingAnchor constant:-16],
		[content.topAnchor constraintEqualToAnchor:wrap.topAnchor constant:14],
		[content.bottomAnchor constraintEqualToAnchor:wrap.bottomAnchor constant:-14],
		[content.widthAnchor constraintEqualToConstant:width],
	]];
	vc.view = wrap;
	[wrap layoutSubtreeIfNeeded];
	vc.preferredContentSize = wrap.fittingSize;
	NSPopover *p = [[NSPopover alloc] init];
	p.contentViewController = vc;
	p.behavior = NSPopoverBehaviorTransient;
	p.appearance = APPEARANCE;
	[p showRelativeToRect:anchor.bounds ofView:anchor preferredEdge:NSRectEdgeMaxY];
}

// fixedText is a wrapping text exactly width wide.
static NSTextField *fixedText(NSString *s, CGFloat size, NSColor *color, CGFloat width) {
	NSTextField *t = wrapping(s, size, color, width);
	t.translatesAutoresizingMaskIntoConstraints = NO;
	[t.widthAnchor constraintEqualToConstant:width].active = YES;
	return t;
}

- (void)permissions:(NSButton *)b {
	NSMutableArray *views = [NSMutableArray arrayWithObject:label([self L:@"permissions"], 13, NSFontWeightSemibold, C_TEXT)];
	for (NSDictionary *p in self.perms) {
		BOOL granted = [p[@"granted"] boolValue];
		NSTextField *name = fixedText(p[@"label"], 12, C_TEXT, 210);
		NSView *right;
		if (granted) {
			right = label([self L:@"allowed"], 12, NSFontWeightMedium, C_OK);
		} else {
			RVButton *allow = [RVButton withTitle:[self L:@"allow"] symbol:nil target:self action:@selector(allow:) primary:YES];
			allow.identifier = p[@"id"];
			right = allow;
		}
		NSView *spacer = [[NSView alloc] init];
		[spacer setContentHuggingPriority:1 forOrientation:NSLayoutConstraintOrientationHorizontal];
		NSStackView *row = [NSStackView stackViewWithViews:@[ name, spacer, right ]];
		[row.widthAnchor constraintEqualToConstant:320].active = YES;
		[views addObject:row];
	}
	NSStackView *s = [NSStackView stackViewWithViews:views];
	s.orientation = NSUserInterfaceLayoutOrientationVertical;
	s.alignment = NSLayoutAttributeLeading;
	s.spacing = 10;
	[self popover:s under:b width:320];
}

- (void)copyCode:(id)s {
	[(RVCopyField *)self.code copyValue];
}
- (void)flashCopied {
	self.copied.hidden = NO;
	self.copied.alphaValue = 1;
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 1400 * NSEC_PER_MSEC), dispatch_get_main_queue(), ^{
		[NSAnimationContext runAnimationGroup:^(NSAnimationContext *c) {
			c.duration = 0.3;
			self.copied.animator.alphaValue = 0;
		}];
	});
}
- (void)newCode:(id)s {
	action(self.handle, @"newcode", @"");
}
- (void)settings:(id)s {
	action(self.handle, @"settings", @"");
}
- (void)about:(id)s {
	action(self.handle, @"about", @"");
}
- (void)updates:(id)s {
	action(self.handle, @"updates", @"");
}
- (void)network:(id)s {
	action(self.handle, @"network", @"");
}
- (void)allow:(NSButton *)s {
	action(self.handle, @"allow", s.identifier);
}
- (void)clearRecents:(id)s {
	action(self.handle, @"clear", @"");
}
- (void)demo:(id)s {
	action(self.handle, @"demo", @"");
}
- (void)connect:(id)s {
	NSString *digits = [[self.address.stringValue componentsSeparatedByCharactersInSet:NSCharacterSet.decimalDigitCharacterSet.invertedSet] componentsJoinedByString:@""];
	if (digits.length != 9) {
		NSBeep();
		[self.window makeFirstResponder:self.address];
		return;
	}
	self.address.stringValue = @"";
	NSString *kind = self.door.indexOfSelectedItem == 1 ? @"claim" : @"join";
	[self.door selectItemAtIndex:0];
	action(self.handle, @"connect", [NSString stringWithFormat:@"%@:%@", kind, digits]);
}

// show brings a page forward. Every page stays in the window, hidden when
// not shown: a picture's layer taken out of the window stops drawing.
- (void)show:(NSView *)page {
	if (page.superview != self.content) {
		page.translatesAutoresizingMaskIntoConstraints = NO;
		[self.content addSubview:page];
		[NSLayoutConstraint activateConstraints:@[
			[page.topAnchor constraintEqualToAnchor:self.content.topAnchor],
			[page.leadingAnchor constraintEqualToAnchor:self.content.leadingAnchor],
			[page.trailingAnchor constraintEqualToAnchor:self.content.trailingAnchor],
			[page.bottomAnchor constraintEqualToAnchor:self.content.bottomAnchor],
		]];
	}
	for (NSView *v in self.content.subviews) v.hidden = v != page;
	if (page == self.home) dispatch_async(dispatch_get_main_queue(), ^{
		[(RVGrid *)self.grid relayout];
	});
}

- (void)renderTabs {
	for (NSView *v in self.tabs.arrangedSubviews) [v removeFromSuperview];
	RVTab *home = [[RVTab alloc] initWithTitle:@"go-link Desk" symbol:@"house.fill" session:nil];
	home.selected = self.selected == nil;
	[self.tabs addArrangedSubview:home];
	for (RVSession *s in self.sessions) {
		RVTab *t = [[RVTab alloc] initWithTitle:s.title symbol:@"display" session:s];
		t.selected = self.selected == s;
		[self.tabs addArrangedSubview:t];
	}
}

- (void)select:(RVSession *)s {
	RVSession *was = self.selected;
	self.selected = s;
	[self show:s ? s.page : self.home];
	[self renderTabs];
	if (was && was != s) was.shown = NO;
	if (s) {
		s.shown = self.window.isVisible;
		[self.window makeFirstResponder:s.view];
	} else {
		[self.window makeFirstResponder:self.address];
	}
}

- (void)addSession:(RVSession *)s {
	[self.sessions addObject:s];
	[self select:s];
	[NSApp activateIgnoringOtherApps:YES];
	[self.window makeKeyAndOrderFront:nil];
}

- (void)removeSession:(RVSession *)s {
	NSUInteger i = [self.sessions indexOfObject:s];
	if (i == NSNotFound) return;
	[self.sessions removeObjectAtIndex:i];
	[s.page removeFromSuperview];
	if (self.selected == s) {
		RVSession *next = self.sessions.count ? self.sessions[MIN(i, self.sessions.count - 1)] : nil;
		self.selected = nil;
		[self select:next];
	} else {
		[self renderTabs];
	}
}

// Closing the window only hides it: the computer stays reachable.
- (BOOL)windowShouldClose:(NSWindow *)sender {
	[self.window orderOut:nil];
	self.selected.shown = NO;
	return NO;
}
- (void)windowDidResignKey:(NSNotification *)n {
	if (self.selected) action(self.selected.handle, @"blur", @"");
}
- (void)windowDidChangeOcclusionState:(NSNotification *)n {
	if (self.selected) self.selected.shown = (self.window.occlusionState & NSWindowOcclusionStateVisible) != 0;
}
@end

#pragma mark - C API

void rv_main_open(uintptr_t handle, const char *labelsJSON) {
	NSData *json = [NSData dataWithBytes:labelsJSON length:strlen(labelsJSON)];
	NSDictionary *labels = [NSJSONSerialization JSONObjectWithData:json options:0 error:nil] ?: @{};
	onMain(^{
		if (gMain) return;
		RVMain *m = [[RVMain alloc] init];
		m.handle = handle;
		m.labels = labels;
		gMain = m;
		[m build];
	});
}

// rv_about shows the About panel: the app's name, version, icon (the one
// in the Dock) and what it is.
void rv_about(const char *version, const char *credits) {
	NSString *v = [NSString stringWithUTF8String:version];
	NSString *c = [NSString stringWithUTF8String:credits];
	onMain(^{
		NSMutableParagraphStyle *p = [[NSMutableParagraphStyle alloc] init];
		p.alignment = NSTextAlignmentCenter;
		NSAttributedString *text = [[NSAttributedString alloc] initWithString:c attributes:@{
			NSFontAttributeName : [NSFont systemFontOfSize:11],
			NSForegroundColorAttributeName : NSColor.secondaryLabelColor,
			NSParagraphStyleAttributeName : p,
		}];
		[NSApp activateIgnoringOtherApps:YES];
		[NSApp orderFrontStandardAboutPanelWithOptions:@{
			NSAboutPanelOptionApplicationName : @"go-link Desk",
			NSAboutPanelOptionApplicationVersion : v,
			NSAboutPanelOptionVersion : @"",
			NSAboutPanelOptionApplicationIcon : NSApp.applicationIconImage,
			NSAboutPanelOptionCredits : text,
			@"Copyright" : @"© 2026 Federico Pereira",
		}];
	});
}

// rv_app_menu puts "About go-link Desk", "Check for Updates…" and
// "Settings…" (⌘,) first in the
// app menu, in place of what the toolkit put there, with the app's name.
void rv_app_menu(const char *labelsJSON) {
	NSData *json = [NSData dataWithBytes:labelsJSON length:strlen(labelsJSON)];
	NSDictionary *l = [NSJSONSerialization JSONObjectWithData:json options:0 error:nil];
	if (![l isKindOfClass:NSDictionary.class]) l = @{};
	NSString *a = l[@"about"] ?: @"About go-link Desk", *st = l[@"settings"] ?: @"Settings…", *q = l[@"quit"] ?: @"Quit go-link Desk";
	onMain(^{
		NSMenu *bar = NSApp.mainMenu;
		if (!bar) {
			bar = [[NSMenu alloc] init];
			NSApp.mainMenu = bar;
		}
		if (bar.numberOfItems == 0) [bar addItem:[[NSMenuItem alloc] init]];
		NSMenuItem *appItem = [bar itemAtIndex:0];
		NSMenu *menu = [[NSMenu alloc] initWithTitle:@"go-link Desk"];
		NSMenuItem *ab = [menu addItemWithTitle:a action:@selector(about:) keyEquivalent:@""];
		ab.target = gMain;
		NSMenuItem *up = [menu addItemWithTitle:l[@"updates"] ?: @"Check for Updates…" action:@selector(updates:) keyEquivalent:@""];
		up.target = gMain;
		NSMenuItem *nt = [menu addItemWithTitle:l[@"network"] ?: @"Test My Connection…" action:@selector(network:) keyEquivalent:@""];
		nt.target = gMain;
		[menu addItem:NSMenuItem.separatorItem];
		NSMenuItem *pr = [menu addItemWithTitle:st action:@selector(settings:) keyEquivalent:@","];
		pr.target = gMain;
		[menu addItem:NSMenuItem.separatorItem];
		NSMenuItem *services = [menu addItemWithTitle:l[@"services"] ?: @"Services" action:nil keyEquivalent:@""];
		services.submenu = [[NSMenu alloc] init];
		NSApp.servicesMenu = services.submenu;
		[menu addItem:NSMenuItem.separatorItem];
		[menu addItemWithTitle:l[@"hide"] ?: @"Hide go-link Desk" action:@selector(hide:) keyEquivalent:@"h"];
		NSMenuItem *others = [menu addItemWithTitle:l[@"hideOthers"] ?: @"Hide Others" action:@selector(hideOtherApplications:) keyEquivalent:@"h"];
		others.keyEquivalentModifierMask = NSEventModifierFlagCommand | NSEventModifierFlagOption;
		[menu addItemWithTitle:l[@"showAll"] ?: @"Show All" action:@selector(unhideAllApplications:) keyEquivalent:@""];
		[menu addItem:NSMenuItem.separatorItem];
		[menu addItemWithTitle:q action:@selector(terminate:) keyEquivalent:@"q"];
		appItem.submenu = menu;
		appItem.title = @"go-link Desk";
	});
}

void rv_main_state(const char *stateJSON) {
	NSData *json = [NSData dataWithBytes:stateJSON length:strlen(stateJSON)];
	onMain(^{
		NSDictionary *st = [NSJSONSerialization JSONObjectWithData:json options:0 error:nil];
		if ([st isKindOfClass:NSDictionary.class]) [gMain setState:st];
	});
}

void rv_main_show(void) {
	onMain(^{
		[NSApp activateIgnoringOtherApps:YES];
		[gMain.window makeKeyAndOrderFront:nil];
		if (gMain.selected) gMain.selected.shown = YES;
	});
}

void *rv_open(uintptr_t handle, const char *title, int tabs, const char *toolsJSON) {
	RVSession *s = [[RVSession alloc] init];
	s.handle = handle;
	s.title = [NSString stringWithUTF8String:title];
	s.queue = dispatch_queue_create("org.go-link.desk.remoteview", DISPATCH_QUEUE_SERIAL);
	NSData *json = [NSData dataWithBytes:toolsJSON length:strlen(toolsJSON)];
	NSArray *tools = arr([NSJSONSerialization JSONObjectWithData:json options:0 error:nil]);
	BOOL asTab = tabs != 0;
	void *ref = (__bridge_retained void *)s;
	onMain(^{
		[s build:tools];
		if (asTab && gMain) {
			[gMain addSession:s];
			return;
		}
		NSRect frame = NSMakeRect(0, 0, 1280, 800);
		NSScreen *screen = NSScreen.mainScreen;
		if (screen) {
			frame.size.width = MIN(frame.size.width, screen.visibleFrame.size.width * 0.9);
			frame.size.height = MIN(frame.size.height, screen.visibleFrame.size.height * 0.9);
		}
		NSWindow *win = [[NSWindow alloc] initWithContentRect:frame
		                                            styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable
		                                              backing:NSBackingStoreBuffered
		                                                defer:NO];
		win.releasedWhenClosed = NO;
		win.title = s.title;
		win.delegate = s;
		win.appearance = APPEARANCE;
		win.tabbingMode = NSWindowTabbingModeDisallowed;
		win.collectionBehavior |= NSWindowCollectionBehaviorFullScreenPrimary;
		win.minSize = NSMakeSize(480, 320);
		win.contentView = s.page;
		s.window = win;
		s.shown = YES;
		[win center];
		[NSApp activateIgnoringOtherApps:YES];
		[win makeKeyAndOrderFront:nil];
		[win makeFirstResponder:s.view];
	});
	return ref;
}

static RVSession *S(void *ref) {
	return (__bridge RVSession *)ref;
}

void rv_title(void *ref, const char *title) {
	NSString *t = [NSString stringWithUTF8String:title];
	RVSession *s = S(ref);
	onMain(^{
		s.title = t;
		s.barTitle.stringValue = t;
		s.window.title = t;
		if (!s.window) [gMain renderTabs];
	});
}

void rv_overlay(void *ref, const char *text, int password, const char *button, int spinner) {
	NSString *t = [NSString stringWithUTF8String:text];
	NSString *b = [NSString stringWithUTF8String:button];
	RVSession *s = S(ref);
	onMain(^{
		s.overlay.hidden = t.length == 0;
		s.message.stringValue = t;
		s.password.hidden = !password;
		s.passwordButton.hidden = !password;
		s.passwordButton.title = b.length ? b : @"OK";
		[s.passwordButton restyle];
		s.spinner.hidden = !spinner;
		if (spinner) [s.spinner startAnimation:nil];
		else [s.spinner stopAnimation:nil];
		if (password) [s.host makeFirstResponder:s.password];
		else if (s.host.firstResponder == s.password) [s.host makeFirstResponder:s.view];
	});
}

void rv_hud(void *ref, const char *text) {
	NSString *t = [NSString stringWithUTF8String:text];
	RVSession *s = S(ref);
	onMain(^{
		s.hud.hidden = t.length == 0;
		s.hud.stringValue = t;
	});
}

void rv_video(void *ref, const uint8_t *data, int n) {
	RVSession *s = S(ref);
	NSData *au = [NSData dataWithBytes:data length:n];
	dispatch_async(s.queue, ^{
		if (!s.closed) [s enqueue:au];
	});
}

void rv_picture(void *ref, int w, int h, const uint8_t *y, const uint8_t *u, const uint8_t *v, int sy, int suv) {
	RVSession *s = S(ref);
	if (s.closed) return;
	CVPixelBufferRef pb = [s pixelBufferWidth:w height:h y:y u:u v:v strideY:sy strideUV:suv];
	if (!pb) return;
	dispatch_async(s.queue, ^{
		if (!s.closed) [s enqueueImage:pb];
		CVPixelBufferRelease(pb);
	});
}

void rv_cursor_shape(void *ref, const uint8_t *png, int n, double hotX, double hotY, double wFrac, double hFrac) {
	NSData *d = [NSData dataWithBytes:png length:n];
	RVSession *s = S(ref);
	onMain(^{
		NSImage *img = [[NSImage alloc] initWithData:d];
		NSBitmapImageRep *rep = (NSBitmapImageRep *)[NSBitmapImageRep imageRepWithData:d];
		if (!img || !rep || rep.pixelsWide == 0 || rep.pixelsHigh == 0) return;
		s.view.cursorImage = img;
		s.view.pointer.contents = img;
		s.view.hotX = hotX / rep.pixelsWide;
		s.view.hotY = hotY / rep.pixelsHigh;
		s.view.cursorW = wFrac;
		s.view.cursorH = hFrac;
		[s.view remakeCursor];
		[CATransaction begin];
		[CATransaction setDisableActions:YES];
		[s.view placePointer];
		[CATransaction commit];
	});
}

void rv_cursor_pos(void *ref, double x, double y, int visible) {
	RVSession *s = S(ref);
	onMain(^{
		s.view.cx = x;
		s.view.cy = y;
		s.view.cvisible = visible != 0;
		[CATransaction begin];
		[CATransaction setDisableActions:YES];
		[s.view placePointer];
		[CATransaction commit];
	});
}

void rv_control(void *ref, int on) {
	RVSession *s = S(ref);
	onMain(^{
		s.view.control = on != 0;
		[s.view remakeCursor];
		[s.view placePointer];
	});
}

void rv_menu(void *ref, const char *tool, const char *itemsJSON) {
	NSString *t = [NSString stringWithUTF8String:tool];
	NSData *json = [NSData dataWithBytes:itemsJSON length:strlen(itemsJSON)];
	RVSession *s = S(ref);
	onMain(^{
		NSArray *items = [NSJSONSerialization JSONObjectWithData:json options:0 error:nil];
		if ([items isKindOfClass:NSArray.class]) [s popMenu:t items:items];
	});
}

void rv_tool_state(void *ref, const char *tool, int on, int enabled) {
	NSString *t = [NSString stringWithUTF8String:tool];
	RVSession *s = S(ref);
	onMain(^{
		RVButton *b = s.buttons[t];
		if (![t isEqual:@"disconnect"]) b.fill = on ? C_TINT_BG : nil;
		b.enabled = enabled != 0;
		[b restyle];
		if (on && enabled) b.contentTintColor = C_ACCENT;
	});
}

void rv_fullscreen(void *ref) {
	RVSession *s = S(ref);
	onMain(^{
		[s.host toggleFullScreen:nil];
	});
}

void rv_open_panel(void *ref, const char *prompt) {
	NSString *p = [NSString stringWithUTF8String:prompt];
	RVSession *s = S(ref);
	onMain(^{
		NSOpenPanel *panel = [NSOpenPanel openPanel];
		panel.canChooseFiles = YES;
		panel.canChooseDirectories = NO;
		panel.allowsMultipleSelection = YES;
		panel.prompt = p;
		[panel beginSheetModalForWindow:s.host
		              completionHandler:^(NSModalResponse r) {
			              if (r != NSModalResponseOK) return;
			              NSMutableArray *paths = [NSMutableArray array];
			              for (NSURL *u in panel.URLs) [paths addObject:u.path];
			              action(s.handle, @"upload", [paths componentsJoinedByString:@"\n"]);
		              }];
	});
}

// rv_thumb takes a small JPEG of the picture, from the window's own
// pixels, while it is the one shown.
void rv_thumb(void *ref) {
	RVSession *s = S(ref);
	onMain(^{
		NSWindow *w = s.host;
		BOOL showing = s.window ? YES : gMain.selected == s;
		if (s.closed || !showing || !w.isVisible || !(w.occlusionState & NSWindowOcclusionStateVisible) || s.view.picture.width <= 0) return;
		NSRect r = [s.view convertRect:[s.view pictureRect] toView:nil];
		r = [w convertRectToScreen:r];
		CGFloat top = NSMaxY(NSScreen.screens.firstObject.frame);
		CGRect cg = CGRectMake(r.origin.x, top - NSMaxY(r), r.size.width, r.size.height);
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
		CGImageRef img = CGWindowListCreateImage(cg, kCGWindowListOptionIncludingWindow, (CGWindowID)w.windowNumber, kCGWindowImageBestResolution);
#pragma clang diagnostic pop
		if (!img) return;
		size_t width = 360, height = MAX(1, (size_t)(360.0 * CGImageGetHeight(img) / MAX(1, CGImageGetWidth(img))));
		NSBitmapImageRep *rep = [[NSBitmapImageRep alloc] initWithBitmapDataPlanes:NULL pixelsWide:width pixelsHigh:height bitsPerSample:8 samplesPerPixel:4 hasAlpha:YES isPlanar:NO colorSpaceName:NSDeviceRGBColorSpace bytesPerRow:0 bitsPerPixel:0];
		[NSGraphicsContext saveGraphicsState];
		NSGraphicsContext.currentContext = [NSGraphicsContext graphicsContextWithBitmapImageRep:rep];
		CGContextDrawImage(NSGraphicsContext.currentContext.CGContext, CGRectMake(0, 0, width, height), img);
		[NSGraphicsContext restoreGraphicsState];
		CGImageRelease(img);
		NSData *jpeg = [rep representationUsingType:NSBitmapImageFileTypeJPEG properties:@{NSImageCompressionFactor : @0.6}];
		if (jpeg) goRVThumb(s.handle, (void *)jpeg.bytes, (int)jpeg.length);
	});
}

// rv_screenshot saves the picture as the window shows it (the computer's
// pointer included), at its full size, as a PNG at path; the outcome comes
// back as the "screenshot" action ("" on success, else why).
void rv_screenshot(void *ref, const char *cpath) {
	RVSession *s = S(ref);
	NSString *path = [NSString stringWithUTF8String:cpath];
	onMain(^{
		NSWindow *w = s.host;
		BOOL showing = s.window ? YES : gMain.selected == s;
		if (s.closed || !showing || !w.isVisible || s.view.picture.width <= 0) {
			action(s.handle, @"screenshot", @"the session is not on screen");
			return;
		}
		NSRect r = [s.view convertRect:[s.view pictureRect] toView:nil];
		r = [w convertRectToScreen:r];
		CGFloat top = NSMaxY(NSScreen.screens.firstObject.frame);
		CGRect cg = CGRectMake(r.origin.x, top - NSMaxY(r), r.size.width, r.size.height);
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
		CGImageRef img = CGWindowListCreateImage(cg, kCGWindowListOptionIncludingWindow, (CGWindowID)w.windowNumber, kCGWindowImageBestResolution | kCGWindowImageBoundsIgnoreFraming);
#pragma clang diagnostic pop
		if (!img) {
			action(s.handle, @"screenshot", @"the system gave no picture");
			return;
		}
		NSBitmapImageRep *rep = [[NSBitmapImageRep alloc] initWithCGImage:img];
		CGImageRelease(img);
		NSData *png = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
		NSError *err = nil;
		if (!png || ![png writeToFile:path options:NSDataWritingAtomic error:&err]) {
			action(s.handle, @"screenshot", err.localizedDescription ?: @"the file could not be written");
			return;
		}
		action(s.handle, @"screenshot", @"");
	});
}

// rv_rec_start starts recording the session's pictures (and, with audio,
// the sound given to rv_rec_audio) to an MPEG-4 file at path.
void rv_rec_start(void *ref, const char *cpath, int audio) {
	RVSession *s = S(ref);
	NSString *path = [NSString stringWithUTF8String:cpath];
	s.recordPath = path;
	s.recorder = [[RVRecorder alloc] initWithURL:[NSURL fileURLWithPath:path] audio:audio != 0];
}

void rv_rec_audio(void *ref, const int16_t *pcm, int n) {
	RVRecorder *rec = S(ref).recorder;
	if (rec) [rec appendPCM:pcm count:n];
}

// rv_rec_stop closes the file; the outcome comes back as the "recorded"
// action ("" on success, else why).
void rv_rec_stop(void *ref) {
	RVSession *s = S(ref);
	RVRecorder *rec = s.recorder;
	s.recorder = nil;
	if (!rec) return;
	uintptr_t h = s.handle;
	[rec finish:^(BOOL ok, NSString *err) {
		action(h, @"recorded", ok ? @"" : err);
	}];
}

void rv_focus(void *ref) {
	RVSession *s = S(ref);
	onMain(^{
		[NSApp activateIgnoringOtherApps:YES];
		if (s.window) {
			[s.window makeKeyAndOrderFront:nil];
		} else {
			[gMain.window makeKeyAndOrderFront:nil];
		}
	});
}

void rv_close(void *ref) {
	RVSession *s = (__bridge_transfer RVSession *)ref;
	onMain(^{
		s.closed = YES;
		if (s.window) {
			s.window.delegate = nil;
			[s.window close];
		} else {
			[gMain removeSession:s];
		}
	});
}

#pragma mark - Settings

// RVPrefs is the Settings window: a page per toolbar button, each a list
// of sections whose rows are a label on the left and a control on the
// right. Go describes it (rv_prefs) and hears every change.
@interface RVPrefs : NSObject <NSToolbarDelegate, NSWindowDelegate, NSTextFieldDelegate>
@property(nonatomic) uintptr_t handle;
@property(nonatomic, strong) NSWindow *window;
@property(nonatomic, strong) NSDictionary *spec;
@property(nonatomic, strong) NSString *page;
@property(nonatomic, strong) NSScrollView *scroll;
@property(nonatomic, strong) NSMutableDictionary<NSString *, NSDictionary *> *rows;
// A change came while a field was being typed in: shown when it ends.
@property(nonatomic) BOOL stale;
@end

static RVPrefs *gPrefs;
static char kFieldKey;
static void objc_setAssociatedObjectField(id o, id v) { objc_setAssociatedObject(o, &kFieldKey, v, OBJC_ASSOCIATION_RETAIN_NONATOMIC); }
static id objc_getAssociatedObjectField(id o) { return objc_getAssociatedObject(o, &kFieldKey); }

@interface RVFlipped : NSView
@end
@implementation RVFlipped
- (BOOL)isFlipped {
	return YES;
}
@end

@implementation RVPrefs

- (NSArray *)pages {
	return arr(self.spec[@"pages"]);
}

- (NSArray<NSToolbarItemIdentifier> *)toolbarDefaultItemIdentifiers:(NSToolbar *)t {
	NSMutableArray *ids = [NSMutableArray array];
	for (NSDictionary *p in self.pages) [ids addObject:p[@"id"]];
	return ids;
}
- (NSArray<NSToolbarItemIdentifier> *)toolbarAllowedItemIdentifiers:(NSToolbar *)t {
	return [self toolbarDefaultItemIdentifiers:t];
}
- (NSArray<NSToolbarItemIdentifier> *)toolbarSelectableItemIdentifiers:(NSToolbar *)t {
	return [self toolbarDefaultItemIdentifiers:t];
}
- (NSToolbarItem *)toolbar:(NSToolbar *)t itemForItemIdentifier:(NSToolbarItemIdentifier)ident willBeInsertedIntoToolbar:(BOOL)flag {
	for (NSDictionary *p in self.pages) {
		if (![p[@"id"] isEqual:ident]) continue;
		NSToolbarItem *item = [[NSToolbarItem alloc] initWithItemIdentifier:ident];
		item.label = p[@"title"];
		item.image = [NSImage imageWithSystemSymbolName:p[@"symbol"] accessibilityDescription:p[@"title"]];
		item.target = self;
		item.action = @selector(pick:);
		return item;
	}
	return nil;
}

- (void)pick:(NSToolbarItem *)item {
	self.page = item.itemIdentifier;
	[self render];
}

- (void)send:(NSString *)ident value:(NSString *)value {
	goRVAction(self.handle, "setting", (char *)[NSString stringWithFormat:@"%@\n%@", ident, value ?: @""].UTF8String);
}

// Controls report through their identifier.
- (void)toggled:(NSSwitch *)s {
	[self send:s.identifier value:s.state == NSControlStateValueOn ? @"1" : @"0"];
}
- (void)picked:(NSPopUpButton *)p {
	[self send:p.identifier value:p.selectedItem.representedObject];
}
- (void)typed:(NSTextField *)f {
	[self send:f.identifier value:f.stringValue];
}
- (void)controlTextDidEndEditing:(NSNotification *)n {
	NSTextField *f = n.object;
	if (![f isKindOfClass:NSSecureTextField.class]) [self typed:f];
	[self renderIfStale];
}
- (void)renderIfStale {
	if (!self.stale) return;
	self.stale = NO;
	dispatch_async(dispatch_get_main_queue(), ^{ [self render]; });
}
- (void)setPassword:(NSButton *)b {
	NSSecureTextField *f = (NSSecureTextField *)objc_getAssociatedObjectField(b);
	NSSecureTextField *f2 = (NSSecureTextField *)objc_getAssociatedObjectField(f);
	// Typed twice: both go, a line apart, and the app compares them.
	NSString *value = f2 ? [NSString stringWithFormat:@"%@\n%@", f.stringValue, f2.stringValue] : f.stringValue;
	f.stringValue = @"";
	f2.stringValue = @"";
	// Out of the field, so the answer (set, or what is wrong) shows at once.
	[self.window makeFirstResponder:nil];
	[self send:b.identifier value:value];
	[self renderIfStale];
}
- (void)pressed:(NSButton *)b {
	NSDictionary *row = self.rows[b.identifier];
	NSDictionary *confirm = [row[@"confirm"] isKindOfClass:NSDictionary.class] ? row[@"confirm"] : nil;
	if (!confirm) {
		[self send:b.identifier value:@""];
		return;
	}
	NSAlert *a = [[NSAlert alloc] init];
	a.messageText = confirm[@"title"] ?: @"";
	a.informativeText = confirm[@"text"] ?: @"";
	NSButton *ok = [a addButtonWithTitle:confirm[@"ok"] ?: @"OK"];
	ok.hasDestructiveAction = YES;
	[a addButtonWithTitle:confirm[@"cancel"] ?: @"Cancel"];
	NSString *ident = b.identifier;
	[a beginSheetModalForWindow:self.window
	          completionHandler:^(NSModalResponse r) {
		          if (r == NSAlertFirstButtonReturn) [self send:ident value:@""];
	          }];
}

static NSColor *tint(NSString *t) {
	if ([t isEqual:@"warn"]) return C_ACCENT;
	if ([t isEqual:@"ok"]) return C_OK;
	if ([t isEqual:@"danger"]) return C_DANGER;
	return C_MUTED;
}

- (NSView *)rowView:(NSDictionary *)r width:(CGFloat)w {
	NSString *kind = r[@"kind"] ?: @"info";
	NSString *ident = r[@"id"] ?: @"";
	if (ident.length) self.rows[ident] = r;
	BOOL enabled = r[@"enabled"] == nil || [r[@"enabled"] boolValue];
	if ([kind isEqual:@"note"]) {
		NSTextField *t = wrapping(r[@"label"], 12, tint(r[@"tint"]), w - 28);
		NSView *v = [[NSView alloc] init];
		t.translatesAutoresizingMaskIntoConstraints = NO;
		[v addSubview:t];
		[NSLayoutConstraint activateConstraints:@[
			[t.leadingAnchor constraintEqualToAnchor:v.leadingAnchor constant:14],
			[t.trailingAnchor constraintEqualToAnchor:v.trailingAnchor constant:-14],
			[t.topAnchor constraintEqualToAnchor:v.topAnchor constant:10],
			[t.bottomAnchor constraintEqualToAnchor:v.bottomAnchor constant:-10],
		]];
		return v;
	}
	NSTextField *name = label(r[@"label"], 13, NSFontWeightRegular, enabled ? C_TEXT : C_FAINT);
	name.lineBreakMode = NSLineBreakByWordWrapping;
	name.maximumNumberOfLines = 2;
	NSView *left = name;
	NSString *detail = r[@"detail"];
	if (detail.length) {
		NSTextField *d = wrapping(detail, 11, C_FAINT, w * 0.55);
		NSStackView *s = [NSStackView stackViewWithViews:@[ name, d ]];
		s.orientation = NSUserInterfaceLayoutOrientationVertical;
		s.alignment = NSLayoutAttributeLeading;
		s.spacing = 2;
		left = s;
	}
	NSView *right = nil;
	if ([kind isEqual:@"toggle"]) {
		NSSwitch *sw = [[NSSwitch alloc] init];
		sw.controlSize = NSControlSizeSmall;
		sw.state = [r[@"on"] boolValue] ? NSControlStateValueOn : NSControlStateValueOff;
		sw.identifier = ident;
		sw.target = self;
		sw.action = @selector(toggled:);
		sw.enabled = enabled;
		right = sw;
	} else if ([kind isEqual:@"select"]) {
		NSPopUpButton *p = [[NSPopUpButton alloc] init];
		p.controlSize = NSControlSizeSmall;
		p.font = [NSFont systemFontOfSize:12];
		for (NSDictionary *o in arr(r[@"options"])) {
			[p addItemWithTitle:o[@"title"] ?: @""];
			p.lastItem.representedObject = o[@"id"] ?: @"";
			if ([o[@"id"] isEqual:r[@"value"]]) [p selectItem:p.lastItem];
		}
		p.identifier = ident;
		p.target = self;
		p.action = @selector(picked:);
		p.enabled = enabled;
		right = p;
	} else if ([kind isEqual:@"text"]) {
		NSTextField *f = [[NSTextField alloc] init];
		f.stringValue = r[@"value"] ?: @"";
		f.placeholderString = r[@"placeholder"];
		f.identifier = ident;
		f.target = self;
		f.action = @selector(typed:);
		f.delegate = self;
		f.font = [NSFont systemFontOfSize:12];
		[f.widthAnchor constraintEqualToConstant:220].active = YES;
		right = f;
	} else if ([kind isEqual:@"password"]) {
		NSSecureTextField *f = [[NSSecureTextField alloc] init];
		f.placeholderString = r[@"placeholder"];
		f.font = [NSFont systemFontOfSize:12];
		[f.widthAnchor constraintEqualToConstant:170].active = YES;
		NSButton *b = [NSButton buttonWithTitle:r[@"button"] ?: @"OK" target:self action:@selector(setPassword:)];
		b.controlSize = NSControlSizeSmall;
		b.identifier = ident;
		objc_setAssociatedObjectField(b, f);
		NSView *fields = f;
		if ([r[@"repeat"] length]) {
			// Typed twice, to be sure of it: Return in the first goes to the second.
			NSSecureTextField *f2 = [[NSSecureTextField alloc] init];
			f2.placeholderString = r[@"repeat"];
			f2.font = [NSFont systemFontOfSize:12];
			[f2.widthAnchor constraintEqualToConstant:170].active = YES;
			f2.target = b;
			f2.action = @selector(performClick:);
			f.nextKeyView = f2;
			f.target = f2;
			f.action = @selector(selectText:);
			objc_setAssociatedObjectField(f, f2);
			NSStackView *col = [NSStackView stackViewWithViews:@[ f, f2 ]];
			col.orientation = NSUserInterfaceLayoutOrientationVertical;
			col.spacing = 4;
			fields = col;
		} else {
			f.target = b;
			f.action = @selector(performClick:);
		}
		NSStackView *s = [NSStackView stackViewWithViews:@[ fields, b ]];
		s.spacing = 6;
		right = s;
	} else if ([kind isEqual:@"button"]) {
		NSButton *b = [NSButton buttonWithTitle:r[@"button"] ?: @"" target:self action:@selector(pressed:)];
		b.controlSize = NSControlSizeSmall;
		b.identifier = ident;
		b.enabled = enabled;
		if ([r[@"tint"] isEqual:@"danger"]) b.hasDestructiveAction = YES;
		right = b;
	} else if ([kind isEqual:@"code"]) {
		NSTextField *c = label(r[@"value"], 17, NSFontWeightSemibold, C_ACCENT);
		c.font = [NSFont monospacedDigitSystemFontOfSize:17 weight:NSFontWeightSemibold];
		c.selectable = YES;
		right = c;
	} else {
		NSString *value = r[@"value"] ?: @"";
		NSTextField *v = label(value, 13, NSFontWeightRegular, tint(r[@"tint"]));
		v.selectable = YES;
		if ([r[@"tint"] isEqual:@"warn"] || [r[@"tint"] isEqual:@"danger"]) {
			NSImageView *dot = [NSImageView imageViewWithImage:[NSImage imageWithSystemSymbolName:@"exclamationmark.circle.fill" accessibilityDescription:nil]];
			dot.contentTintColor = tint(r[@"tint"]);
			NSStackView *s = [NSStackView stackViewWithViews:@[ dot, v ]];
			s.spacing = 5;
			right = s;
		} else {
			right = v;
		}
	}
	NSView *row = [[NSView alloc] init];
	left.translatesAutoresizingMaskIntoConstraints = NO;
	right.translatesAutoresizingMaskIntoConstraints = NO;
	[row addSubview:left];
	[row addSubview:right];
	[left setContentCompressionResistancePriority:NSLayoutPriorityDefaultLow forOrientation:NSLayoutConstraintOrientationHorizontal];
	[NSLayoutConstraint activateConstraints:@[
		[left.leadingAnchor constraintEqualToAnchor:row.leadingAnchor constant:14],
		[left.topAnchor constraintGreaterThanOrEqualToAnchor:row.topAnchor constant:10],
		[left.bottomAnchor constraintLessThanOrEqualToAnchor:row.bottomAnchor constant:-10],
		[left.centerYAnchor constraintEqualToAnchor:row.centerYAnchor],
		[right.trailingAnchor constraintEqualToAnchor:row.trailingAnchor constant:-14],
		[right.centerYAnchor constraintEqualToAnchor:row.centerYAnchor],
		[right.leadingAnchor constraintGreaterThanOrEqualToAnchor:left.trailingAnchor constant:16],
		[row.heightAnchor constraintGreaterThanOrEqualToConstant:40],
	]];
	return row;
}

- (void)render {
	NSDictionary *page = nil;
	for (NSDictionary *p in self.pages) {
		if ([p[@"id"] isEqual:self.page]) page = p;
	}
	if (!page) {
		page = self.pages.firstObject;
		self.page = page[@"id"];
	}
	self.window.toolbar.selectedItemIdentifier = self.page;
	self.window.title = page[@"title"] ?: @"";
	self.rows = [NSMutableDictionary dictionary];
	CGFloat w = 600;
	NSStackView *stack = [NSStackView stackViewWithViews:@[]];
	stack.orientation = NSUserInterfaceLayoutOrientationVertical;
	stack.alignment = NSLayoutAttributeLeading;
	stack.spacing = 8;
	stack.edgeInsets = NSEdgeInsetsMake(20, 20, 24, 20);
	for (NSDictionary *sec in arr(page[@"sections"])) {
		NSString *title = sec[@"title"];
		if (title.length) {
			NSTextField *t = label(title, 13, NSFontWeightBold, C_TEXT);
			[stack addArrangedSubview:t];
			if (stack.arrangedSubviews.count > 1) [stack setCustomSpacing:22 afterView:stack.arrangedSubviews[stack.arrangedSubviews.count - 2]];
		}
		NSStackView *box = [NSStackView stackViewWithViews:@[]];
		box.orientation = NSUserInterfaceLayoutOrientationVertical;
		box.spacing = 0;
		box.wantsLayer = YES;
		box.layer.cornerRadius = 8;
		box.layer.backgroundColor = C_SURFACE.CGColor;
		box.layer.borderWidth = 1;
		box.layer.borderColor = C_BORDER.CGColor;
		NSArray *rows = arr(sec[@"rows"]);
		for (NSUInteger i = 0; i < rows.count; i++) {
			NSView *rv = [self rowView:rows[i] width:w];
			[box addArrangedSubview:rv];
			[rv.widthAnchor constraintEqualToAnchor:box.widthAnchor].active = YES;
			if (i + 1 < rows.count) {
				NSBox *line = [[NSBox alloc] init];
				line.boxType = NSBoxCustom;
				line.borderWidth = 0;
				line.fillColor = C_BORDER;
				[line.heightAnchor constraintEqualToConstant:1].active = YES;
				[box addArrangedSubview:line];
				[line.widthAnchor constraintEqualToAnchor:box.widthAnchor constant:-28].active = YES;
			}
		}
		[stack addArrangedSubview:box];
		[box.widthAnchor constraintEqualToAnchor:stack.widthAnchor constant:-40].active = YES;
		NSString *footer = sec[@"footer"];
		if (footer.length) [stack addArrangedSubview:wrapping(footer, 11, C_FAINT, w - 40)];
	}
	RVFlipped *doc = [[RVFlipped alloc] init];
	doc.translatesAutoresizingMaskIntoConstraints = NO;
	stack.translatesAutoresizingMaskIntoConstraints = NO;
	[doc addSubview:stack];
	NSPoint keep = self.scroll.contentView.bounds.origin;
	self.scroll.documentView = doc;
	[NSLayoutConstraint activateConstraints:@[
		[stack.topAnchor constraintEqualToAnchor:doc.topAnchor],
		[stack.leadingAnchor constraintEqualToAnchor:doc.leadingAnchor],
		[stack.trailingAnchor constraintEqualToAnchor:doc.trailingAnchor],
		[stack.bottomAnchor constraintEqualToAnchor:doc.bottomAnchor],
		[doc.widthAnchor constraintEqualToAnchor:self.scroll.contentView.widthAnchor],
	]];
	[self.scroll.contentView scrollToPoint:keep];
}

- (void)build {
	NSWindow *w = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 640, 640)
	                                          styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskResizable
	                                            backing:NSBackingStoreBuffered
	                                              defer:NO];
	w.releasedWhenClosed = NO;
	w.appearance = APPEARANCE;
	w.backgroundColor = C_BG;
	w.minSize = NSMakeSize(560, 420);
	w.delegate = self;
	w.tabbingMode = NSWindowTabbingModeDisallowed;
	NSToolbar *bar = [[NSToolbar alloc] initWithIdentifier:@"org.go-link.desk.settings"];
	bar.delegate = self;
	bar.displayMode = NSToolbarDisplayModeIconAndLabel;
	bar.allowsUserCustomization = NO;
	w.toolbar = bar;
	w.toolbarStyle = NSWindowToolbarStylePreference;
	self.scroll = [[NSScrollView alloc] init];
	self.scroll.hasVerticalScroller = YES;
	self.scroll.drawsBackground = NO;
	w.contentView = self.scroll;
	self.window = w;
	[w center];
}
@end

void rv_prefs(uintptr_t handle, const char *specJSON, int show) {
	NSData *json = [NSData dataWithBytes:specJSON length:strlen(specJSON)];
	BOOL bringUp = show != 0;
	onMain(^{
		NSDictionary *spec = [NSJSONSerialization JSONObjectWithData:json options:0 error:nil];
		if (![spec isKindOfClass:NSDictionary.class]) return;
		if (!gPrefs) {
			gPrefs = [[RVPrefs alloc] init];
			gPrefs.handle = handle;
			gPrefs.spec = spec;
			[gPrefs build];
		}
		gPrefs.spec = spec;
		// Typing in a field: leave it, the change came from it.
		NSResponder *fr = gPrefs.window.firstResponder;
		BOOL typing = [fr isKindOfClass:NSTextView.class] && gPrefs.window.isKeyWindow;
		if (!typing || bringUp) {
			gPrefs.stale = NO;
			[gPrefs render];
		} else {
			gPrefs.stale = YES;
		}
		if (bringUp) {
			[NSApp activateIgnoringOtherApps:YES];
			[gPrefs.window makeKeyAndOrderFront:nil];
		}
	});
}

// rv_dock_icon sets the running app's icon in the Dock.
void rv_dock_icon(const uint8_t *png, int n) {
	NSData *d = [NSData dataWithBytes:png length:n];
	onMain(^{
		NSImage *img = [[NSImage alloc] initWithData:d];
		if (img) NSApp.applicationIconImage = img;
	});
}

// rv_theme picks the theme: 0 dark, 1 light, 2 the system's (and follows
// it when it changes).
static int gThemeMode;
static void applyTheme(void) {
	BOOL light = gThemeMode == 1;
	if (gThemeMode == 2) {
		NSAppearanceName n = [NSApp.effectiveAppearance bestMatchFromAppearancesWithNames:@[ NSAppearanceNameAqua, NSAppearanceNameDarkAqua ]];
		light = [n isEqual:NSAppearanceNameAqua];
	}
	if (light == gLight && gMain.window.contentView) return;
	gLight = light;
	if (gMain.window) [gMain buildContent];
	if (gPrefs.window) {
		gPrefs.window.appearance = APPEARANCE;
		gPrefs.window.backgroundColor = C_BG;
		[gPrefs render];
	}
}

void rv_theme(int mode) {
	onMain(^{
		static BOOL watching;
		gThemeMode = mode;
		if (!watching) {
			watching = YES;
			[NSDistributedNotificationCenter.defaultCenter addObserverForName:@"AppleInterfaceThemeChangedNotification"
			                                                           object:nil
			                                                            queue:NSOperationQueue.mainQueue
			                                                       usingBlock:^(NSNotification *n) {
				                                                       if (gThemeMode == 2) dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 200 * NSEC_PER_MSEC), dispatch_get_main_queue(), ^{
					                                                       applyTheme();
				                                                       });
			                                                       }];
		}
		applyTheme();
	});
}
