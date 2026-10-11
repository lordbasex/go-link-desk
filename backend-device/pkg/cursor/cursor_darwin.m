// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

#import <AppKit/AppKit.h>
#import <CoreGraphics/CoreGraphics.h>
#include <stdlib.h>
#include <string.h>

// cur_position is where the pointer is, in the desktop's points (top-left
// origin of the main display).
void cur_position(double *x, double *y) {
	CGEventRef ev = CGEventCreate(NULL);
	CGPoint p = CGEventGetLocation(ev);
	CFRelease(ev);
	*x = p.x;
	*y = p.y;
}

// cur_shape returns the pointer's current image as PNG (malloc'd; the
// sharpest version up to twice its size, enough for Retina, since macOS
// keeps much larger ones for accessibility zoom) with its hot spot in that
// image's pixels, and its size on screen in points.
int cur_shape(uint8_t **png, int *len, int *hx, int *hy, int *w, int *h, int *pw, int *ph) {
	@autoreleasepool {
		NSCursor *c = [NSCursor currentSystemCursor];
		if (!c)
			return -1;
		NSImage *img = c.image;
		NSSize pts = img.size;
		if (pts.width <= 0 || pts.height <= 0)
			return -1;
		CGImageRef cg = [img CGImageForProposedRect:NULL context:nil hints:nil];
		NSBitmapImageRep *best = nil, *smallest = nil;
		for (NSImageRep *r in img.representations) {
			if (![r isKindOfClass:NSBitmapImageRep.class])
				continue;
			if (!smallest || r.pixelsWide < smallest.pixelsWide)
				smallest = (NSBitmapImageRep *)r;
			if (r.pixelsWide <= pts.width * 2 + 1 && (!best || r.pixelsWide > best.pixelsWide))
				best = (NSBitmapImageRep *)r;
		}
		if (!best)
			best = smallest;
		if (!best && cg) {
			// one image only: draw it at twice its size in points
			best = [[NSBitmapImageRep alloc] initWithBitmapDataPlanes:NULL pixelsWide:(NSInteger)(pts.width * 2) pixelsHigh:(NSInteger)(pts.height * 2)
				bitsPerSample:8 samplesPerPixel:4 hasAlpha:YES isPlanar:NO colorSpaceName:NSDeviceRGBColorSpace bytesPerRow:0 bitsPerPixel:0];
			[NSGraphicsContext saveGraphicsState];
			NSGraphicsContext.currentContext = [NSGraphicsContext graphicsContextWithBitmapImageRep:best];
			[img drawInRect:NSMakeRect(0, 0, pts.width * 2, pts.height * 2)];
			[NSGraphicsContext restoreGraphicsState];
		}
		if (!best)
			return -1;
		NSData *data = [best representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
		if (!data || data.length == 0)
			return -1;
		double scale = (double)best.pixelsWide / pts.width;
		*hx = (int)(c.hotSpot.x * scale);
		*hy = (int)(c.hotSpot.y * scale);
		*w = (int)best.pixelsWide;
		*h = (int)best.pixelsHigh;
		*pw = (int)pts.width;
		*ph = (int)pts.height;
		*len = (int)data.length;
		*png = malloc(data.length);
		memcpy(*png, data.bytes, data.length);
		return 0;
	}
}
