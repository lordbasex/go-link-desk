// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package cursor reads the host's mouse pointer: where it is and what it
// looks like. The screen is captured without the pointer; the viewer draws
// it from these, sharp at any size and with no delay of its own.
package cursor

// Shape is the pointer's image.
type Shape struct {
	ID   string // a hash of the image: the viewer caches shapes by it
	PNG  []byte
	HotX int // the hot spot, in the image's pixels
	HotY int
	// Width and Height are the image's size in pixels (twice its size
	// on screen when the image is a Retina one).
	Width, Height int
	// PointsWidth and PointsHeight are its size on screen, in points.
	PointsWidth, PointsHeight int
}
