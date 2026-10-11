// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package permission checks and asks for what macOS makes an app ask the
// person at the computer for: Screen Recording (to show the screen) and
// Accessibility (to move the pointer and type, from phase 3). Both belong
// to the app's signature, and a grant usually needs the app to start
// again. Elsewhere there is nothing to ask.
package permission

// Kind is one permission.
type Kind string

const (
	ScreenRecording Kind = "screen"
	Accessibility   Kind = "accessibility"
)

// Needed reports whether this system asks for permissions at all.
const Needed = needed

// Granted reports whether the app has the permission now.
func Granted(k Kind) bool {
	if k == Accessibility {
		return accessibility()
	}
	return screenRecording()
}

// Request asks the system for the permission: the first time it shows its
// own dialog; after that it only reports. It also opens the right page of
// System Settings, where the switch is.
func Request(k Kind) bool {
	if k == Accessibility {
		ok := requestAccessibility()
		if !ok {
			openSettings("Privacy_Accessibility")
		}
		return ok
	}
	ok := requestScreenRecording()
	if !ok {
		openSettings("Privacy_ScreenCapture")
	}
	return ok
}
