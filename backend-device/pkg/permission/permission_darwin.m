// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

#import <AppKit/AppKit.h>
#import <ApplicationServices/ApplicationServices.h>
#import <CoreGraphics/CoreGraphics.h>

int perm_screen(void) { return CGPreflightScreenCaptureAccess() ? 1 : 0; }
int perm_request_screen(void) { return CGRequestScreenCaptureAccess() ? 1 : 0; }
int perm_accessibility(void) { return AXIsProcessTrusted() ? 1 : 0; }

// perm_request_accessibility asks the system to show its dialog, which
// adds the app to the Accessibility list for the person to switch on.
int perm_request_accessibility(void) {
	NSDictionary *opts = @{(__bridge NSString *)kAXTrustedCheckOptionPrompt : @YES};
	return AXIsProcessTrustedWithOptions((__bridge CFDictionaryRef)opts) ? 1 : 0;
}

// perm_open_settings opens System Settings at a privacy pane.
void perm_open_settings(const char *pane) {
	NSString *url = [NSString stringWithFormat:@"x-apple.systempreferences:com.apple.preference.security?%s", pane];
	[[NSWorkspace sharedWorkspace] openURL:[NSURL URLWithString:url]];
}
