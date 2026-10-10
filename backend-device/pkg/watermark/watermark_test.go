// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package watermark

import (
	"testing"
	"time"
)

func gray(w, h int) []byte {
	b := make([]byte, w*h*3/2)
	for i := range b {
		b[i] = 128
	}
	return b
}

func TestCornersChangeEvery8To15Seconds(t *testing.T) {
	s := NewWithSeed(42)
	s.Corner(10 * time.Minute)
	for i := 1; i < len(s.plan); i++ {
		gap := s.plan[i].at - s.plan[i-1].at
		if gap < 8*time.Second || gap > 15*time.Second {
			t.Fatalf("gap %v", gap)
		}
		if s.plan[i].corner == s.plan[i-1].corner {
			t.Fatalf("same corner twice at %v", s.plan[i].at)
		}
	}
}

func TestHeartbeatIsSlowAndSmall(t *testing.T) {
	peak, rest := 0.0, 0
	for ms := 0; ms < 3000; ms += 10 {
		v := Heartbeat(time.Duration(ms)*time.Millisecond, 0.12)
		peak = max(peak, v)
		if v < 1.005 {
			rest++
		}
	}
	if peak < 1.1 || peak > 1.12 || rest < 120 {
		t.Fatalf("peak %.3f, rest %d of 300", peak, rest)
	}
}

// The mark lands in the corner the plan says, in color, and the source
// frame is left alone.
func TestDrawStampsTheCorner(t *testing.T) {
	const w, h = 288, 224
	src := gray(w, h)
	s := NewWithSeed(1)
	out := s.Draw(src, w, h, 0)
	for _, v := range src {
		if v != 128 {
			t.Fatal("the source frame was changed")
		}
	}
	corner := s.Corner(0)
	quadrant := func(x, y int) int {
		switch {
		case x < w/2 && y < h/2:
			return 0
		case x >= w/2 && y < h/2:
			return 1
		case x >= w/2:
			return 2
		}
		return 3
	}
	changed := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if out[y*w+x] != 128 {
				changed++
				if q := quadrant(x, y); q != corner {
					t.Fatalf("pixel %d,%d in corner %d, want %d", x, y, q, corner)
				}
			}
		}
	}
	if min := Size(h) * Size(h) / 2; changed < min {
		t.Fatalf("only %d pixels changed", changed)
	}
	// Amber shows in the chroma (V well above neutral).
	v := out[w*h+(w/2)*(h/2):]
	hot := 0
	for _, c := range v {
		if c > 150 {
			hot++
		}
	}
	if hot == 0 {
		t.Fatal("the mark has no color")
	}
}

func TestDrawLeavesTinyFramesAlone(t *testing.T) {
	src := gray(32, 32)
	if out := NewWithSeed(1).Draw(src, 32, 32, 0); &out[0] != &src[0] {
		t.Fatal("a tiny frame should be returned as is")
	}
}
