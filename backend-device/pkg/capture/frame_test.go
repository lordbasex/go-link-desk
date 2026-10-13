// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build (linux || windows) && !headless

package capture

import "testing"

// solid is a w x h picture of one color, in BGRx (or RGBx).
func solid(w, h int, r, g, b byte, rgb bool) []byte {
	p := make([]byte, w*h*4)
	for i := 0; i < len(p); i += 4 {
		if rgb {
			p[i], p[i+1], p[i+2] = r, g, b
		} else {
			p[i], p[i+1], p[i+2] = b, g, r
		}
	}
	return p
}

// near is a within 2 of want: libyuv rounds a little differently from
// the formula.
func near(a, want byte) bool { return int(a)-int(want) <= 2 && int(want)-int(a) <= 2 }

func TestColorsBecomeBT601(t *testing.T) {
	cases := []struct {
		name    string
		r, g, b byte
		y, u, v byte
	}{
		{"white", 255, 255, 255, 235, 128, 128},
		{"black", 0, 0, 0, 16, 128, 128},
		{"red", 255, 0, 0, 82, 90, 240},
		{"blue", 0, 0, 255, 41, 240, 110},
	}
	for _, c := range cases {
		for _, rgb := range []bool{false, true} {
			f := scaleI420(solid(64, 32, c.r, c.g, c.b, rgb), 64, 32, 64, 32, rgb)
			y, u, v := f[0], f[64*32], f[64*32+32*16]
			if !near(y, c.y) || !near(u, c.u) || !near(v, c.v) {
				t.Errorf("%s (rgb %v): YUV %d %d %d, want %d %d %d", c.name, rgb, y, u, v, c.y, c.u, c.v)
			}
		}
	}
}

func TestASmallerPictureAveragesThePixels(t *testing.T) {
	// Black and white columns, one pixel each: at half the width, grey.
	w, h := 64, 32
	src := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x += 2 {
			i := (y*w + x) * 4
			src[i], src[i+1], src[i+2] = 255, 255, 255
		}
	}
	f := scaleI420(src, w, h, 32, 16, false)
	if y := f[8*32+8]; y < 115 || y > 135 {
		t.Errorf("Y %d, want the grey between 16 and 235", y)
	}
}

func TestOtherShapesGetBlackBars(t *testing.T) {
	// A wide white picture in a square: black above and below.
	f := scaleI420(solid(64, 16, 255, 255, 255, false), 64, 16, 32, 32, false)
	if top, mid := f[0], f[16*32+16]; top != 16 || !near(mid, 235) {
		t.Errorf("top %d middle %d, want 16 (black) and 235 (white)", top, mid)
	}
	if u := f[32*32]; u != 128 {
		t.Errorf("the bars' U is %d, want 128", u)
	}
}

func BenchmarkScale1080p(b *testing.B) {
	src := solid(1920, 1080, 30, 120, 200, false)
	for b.Loop() {
		scaleI420(src, 1920, 1080, 1920, 1080, false)
	}
}

func BenchmarkScale4KTo1080p(b *testing.B) {
	src := solid(3840, 2160, 30, 120, 200, false)
	for b.Loop() {
		scaleI420(src, 3840, 2160, 1920, 1080, false)
	}
}
