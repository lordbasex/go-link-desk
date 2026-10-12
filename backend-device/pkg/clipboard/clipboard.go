// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package clipboard reads and writes the computer's clipboard as text:
// NSPasteboard on macOS; on Linux, the X11 CLIPBOARD, or a Wayland
// desktop's data-control protocol. Elsewhere Supported is false and
// nothing is shared.
package clipboard

// Board is the clipboard (an interface so tests can stand in for it).
type Board interface {
	// ChangeCount moves on every time anything is copied.
	ChangeCount() int
	// Text is the clipboard's text, if it holds any.
	Text() (string, bool)
	// SetText replaces the clipboard with text and returns the new
	// ChangeCount.
	SetText(s string) int
}
