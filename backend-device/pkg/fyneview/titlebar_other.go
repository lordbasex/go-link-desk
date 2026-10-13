// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !windows && !linux && !headless

package fyneview

import "fyne.io/fyne/v2"

// styleFrame leaves the frame to the system: macOS draws it in the
// app's appearance.
func styleFrame(fyne.Window) {}

// StyleWindow: the system draws the other windows' frames too.
func StyleWindow(fyne.Window) {}
