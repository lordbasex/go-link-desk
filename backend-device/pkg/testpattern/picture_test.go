// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package testpattern

import (
	"image"
	"image/color"
	"testing"
)

func TestPictureIsScaledToTheFrame(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			c := color.RGBA{A: 255}
			if x >= 2 {
				c = color.RGBA{255, 255, 255, 255}
			}
			img.Set(x, y, c)
		}
	}
	buf := Picture(img, 8, 4)
	if len(buf) != 8*4+2*4*2 {
		t.Fatalf("size %d", len(buf))
	}
	// Left half black (Y 16), right half white (Y 235), neutral chroma.
	if buf[0] != 16 || buf[7] != 235 || buf[3*8+7] != 235 {
		t.Fatalf("luma %v", buf[:8])
	}
	if buf[32] != 128 || buf[40] != 128 {
		t.Fatalf("chroma %d %d", buf[32], buf[40])
	}
}
