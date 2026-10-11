// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

package cursor

import (
	"bytes"
	"image/png"
	"testing"
)

func TestThePointer(t *testing.T) {
	x, y := Position()
	t.Logf("pointer at %.0f,%.0f", x, y)
	s, err := Current()
	if err != nil {
		t.Skip("no pointer image here (no window server?):", err)
	}
	img, err := png.Decode(bytes.NewReader(s.PNG))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != s.Width || b.Dy() != s.Height || s.HotX < 0 || s.HotX > s.Width || len(s.ID) != 16 {
		t.Fatalf("shape %dx%d hot %d,%d id %q, image %v", s.Width, s.Height, s.HotX, s.HotY, s.ID, b)
	}
	if s.Width > 2*s.PointsWidth+1 || s.PointsWidth == 0 {
		t.Fatalf("shape %dx%d for %dx%d points: more than Retina needs", s.Width, s.Height, s.PointsWidth, s.PointsHeight)
	}
	t.Logf("shape %dx%d (%dx%d points) hot %d,%d, %d bytes", s.Width, s.Height, s.PointsWidth, s.PointsHeight, s.HotX, s.HotY, len(s.PNG))
}
