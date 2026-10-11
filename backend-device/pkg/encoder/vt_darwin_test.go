// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

package encoder

import (
	"bytes"
	"sync"
	"testing"
	"time"
)

// VideoToolbox called directly: 1080p frames in, Annex B access units out, the first a keyframe with its
// SPS and PPS, and a keyframe again when asked for one.
func TestVideoToolbox(t *testing.T) {
	const w, h, n = 1920, 1080, 60
	var mu sync.Mutex
	var units [][]byte
	var pts []time.Duration
	enc, err := NewH264(Config{Width: w, Height: h, FPS: 60, BitrateKbps: 8000, GOPFrames: 600}, func(au []byte, at time.Duration) {
		pts = append(pts, at)
		mu.Lock()
		units = append(units, au)
		mu.Unlock()
	})
	if err != nil {
		t.Skip("no VideoToolbox H.264 here:", err)
	}
	frame := make([]byte, w*h*3/2)
	start := time.Now()
	for i := 0; i < n; i++ {
		for k := range frame[:w*h] {
			frame[k] = byte(k/w + i*3) // a moving gradient
		}
		if err := enc.WriteKey(frame, i == 40); err != nil {
			t.Fatal(err)
		}
	}
	enc.Close()
	t.Logf("%d frames of %dx%d in %v", n, w, h, time.Since(start))
	mu.Lock()
	defer mu.Unlock()
	if len(units) < n-2 {
		t.Fatalf("got %d access units for %d frames", len(units), n)
	}
	nal := func(au []byte, typ byte) bool {
		for i := 0; i+4 < len(au); i++ {
			if bytes.Equal(au[i:i+4], []byte{0, 0, 0, 1}) && au[i+4]&0x1f == typ {
				return true
			}
		}
		return false
	}
	if !bytes.HasPrefix(units[0], []byte{0, 0, 0, 1, 9}) || !nal(units[0], 7) || !nal(units[0], 8) || !nal(units[0], 5) {
		t.Error("the first access unit is not a delimited keyframe with SPS and PPS")
	}
	keys := 0
	for _, au := range units[1:] {
		if nal(au, 5) {
			keys++
		}
	}
	if keys < 1 {
		t.Error("the keyframe asked for at frame 40 never came")
	}
	for i := 1; i < len(pts); i++ {
		if pts[i] <= pts[i-1] {
			t.Fatalf("frame %d at %v after %v", i, pts[i], pts[i-1])
		}
	}
	if d := pts[1] - pts[0] - time.Second/60; d < -time.Millisecond || d > time.Millisecond {
		t.Errorf("frames %v apart, want 1/60 s", pts[1]-pts[0])
	}
}
