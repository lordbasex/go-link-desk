// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package inject drives the host's mouse and keyboard: CGEvent on macOS
// (it needs the Accessibility permission). Other systems come in phase 7.
// An Injector keeps which keys and buttons it holds down, so that
// ReleaseAll can let them all go: a viewer that disconnects never leaves a
// key stuck on the host.
package inject

// Button numbers (the protocol's).
const (
	Left    = 0
	Middle  = 1
	Right   = 2
	Back    = 3
	Forward = 4
)

// Modifier bits a key comes with (the protocol's).
const (
	ModShift = 1 << 0
	ModCtrl  = 1 << 1
	ModAlt   = 1 << 2
	ModMeta  = 1 << 3
)

// Injector is the host's mouse and keyboard. Positions are the desktop's
// points (top-left of the main display is 0, 0).
type Injector interface {
	// Move puts the pointer at x, y.
	Move(x, y float64)
	// MoveBy moves the pointer by dx, dy (pointer lock).
	MoveBy(dx, dy int)
	// Button presses or releases a button at x, y (where the pointer is
	// when at is false).
	Button(button int, down bool, x, y float64, at bool)
	// Scroll scrolls by dx, dy: pixels, or lines. Positive dy scrolls the
	// content down, like the browser's wheel.
	Scroll(dx, dy int, lines bool)
	// Key presses or releases a key by its virtual key code; repeat is the
	// keyboard's own repeat. mods are the modifiers held on the viewer's
	// keyboard: the key comes with them even if their own presses never
	// arrived (held before the page had the focus).
	Key(vk int, down, repeat bool, mods uint8)
	// Text types text, character by character.
	Text(s string)
	// ReleaseAll lets go of every key and button held.
	ReleaseAll()
	// Trusted reports whether the system lets this app inject.
	Trusted() bool
}
