// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux && !headless

package portal

import "sync"

// On Wayland no program can ask where the pointer is: the screen's
// streams carry it (cursor metadata), and the capture leaves it here for
// the cursor package to read.

// Pointer is the pointer as the streams last showed it.
type Pointer struct {
	X, Y    float64 // on the desktop, in logical pixels
	Visible bool
	// The image: straight RGBA, W x H, with its hot spot; Serial changes
	// with it.
	RGBA         []byte
	W, H         int
	HotX, HotY   int
	Serial       uint64
	Known, Shape bool // a position, an image arrived
}

var pointer struct {
	sync.Mutex
	p Pointer
}

// SetPointerPosition records where the pointer is.
func SetPointerPosition(x, y float64, visible bool) {
	pointer.Lock()
	pointer.p.X, pointer.p.Y, pointer.p.Visible, pointer.p.Known = x, y, visible, true
	pointer.Unlock()
}

// SetPointerShape records the pointer's image.
func SetPointerShape(rgba []byte, w, h, hotX, hotY int) {
	pointer.Lock()
	p := &pointer.p
	p.RGBA, p.W, p.H, p.HotX, p.HotY, p.Shape = rgba, w, h, hotX, hotY, true
	p.Serial++
	pointer.Unlock()
}

// CurrentPointer is the pointer as last recorded.
func CurrentPointer() Pointer {
	pointer.Lock()
	defer pointer.Unlock()
	return pointer.p
}
