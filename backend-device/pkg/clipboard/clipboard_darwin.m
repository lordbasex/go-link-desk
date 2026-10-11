// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

#import <AppKit/AppKit.h>
#include <stdlib.h>
#include <string.h>

long clip_change_count(void) { return (long)NSPasteboard.generalPasteboard.changeCount; }

char *clip_text(void) {
	@autoreleasepool {
		NSString *s = [NSPasteboard.generalPasteboard stringForType:NSPasteboardTypeString];
		if (!s)
			return NULL;
		const char *u = s.UTF8String;
		return u ? strdup(u) : NULL;
	}
}

long clip_set_text(const char *s) {
	@autoreleasepool {
		NSPasteboard *pb = NSPasteboard.generalPasteboard;
		[pb clearContents];
		[pb setString:[NSString stringWithUTF8String:s] forType:NSPasteboardTypeString];
		return (long)pb.changeCount;
	}
}
