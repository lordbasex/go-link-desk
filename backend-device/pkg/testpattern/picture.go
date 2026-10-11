// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package testpattern

import (
	"image"
	"image/color"
)

// Picture is a still picture as an I420 frame of w x h (even sizes), the
// picture scaled to fill it: what a test host shows instead of the card
// (screenshots of a desktop, --test-picture).
func Picture(img image.Image, w, h int) []byte {
	cw, ch := w/2, h/2
	buf := make([]byte, w*h+2*cw*ch)
	b := img.Bounds()
	for y := 0; y < h; y++ {
		sy := b.Min.Y + y*b.Dy()/h
		for x := 0; x < w; x++ {
			sx := b.Min.X + x*b.Dx()/w
			c := color.RGBAModel.Convert(img.At(sx, sy)).(color.RGBA)
			v := yuv(rgb{float64(c.R), float64(c.G), float64(c.B)})
			buf[y*w+x] = v[0]
			if x%2 == 0 && y%2 == 0 {
				i := (y/2)*cw + x/2
				buf[w*h+i] = v[1]
				buf[w*h+cw*ch+i] = v[2]
			}
		}
	}
	return buf
}
