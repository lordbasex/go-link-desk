// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

package awake

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation
#include <IOKit/pwr_mgt/IOPMLib.h>

static IOPMAssertionID awake_id = kIOPMNullAssertionID;

static int awake_hold(void) {
	CFStringRef reason = CFSTR("go-link Desk: a remote session is on");
	IOReturn r = IOPMAssertionCreateWithName(kIOPMAssertionTypePreventUserIdleDisplaySleep, kIOPMAssertionLevelOn, reason, &awake_id);
	return r == kIOReturnSuccess;
}

static void awake_release(void) {
	if (awake_id != kIOPMNullAssertionID) {
		IOPMAssertionRelease(awake_id);
		awake_id = kIOPMNullAssertionID;
	}
}
*/
import "C"

func hold() bool { return C.awake_hold() != 0 }

func release() { C.awake_release() }
