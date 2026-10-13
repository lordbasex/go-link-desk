// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !windows && !headless

package fyneview

import "fyne.io/fyne/v2"

// styleFrame leaves the frame to the system: macOS draws it in the
// app's appearance, and on Linux the window manager draws its own.
func styleFrame(fyne.Window) {}
