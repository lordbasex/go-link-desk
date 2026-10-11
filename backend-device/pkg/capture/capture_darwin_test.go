// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

package capture

import (
	"sync/atomic"
	"testing"
	"time"
)

// A real capture: needs the Screen Recording permission for whatever runs
// the test (the terminal), so it is skipped without it.
func TestCaptureTheMainDisplay(t *testing.T) {
	if !HasAccess() {
		t.Skip("no Screen Recording permission")
	}
	ds, err := Displays()
	if err != nil || len(ds) == 0 {
		t.Fatalf("displays %v %v", ds, err)
	}
	d := ds[0]
	t.Logf("display %+v", d)
	if !d.Primary || d.Width < d.PointsWidth || d.Width == 0 {
		t.Fatalf("main display %+v", d)
	}
	w, h := 1280, 1280*d.Height/d.Width&^1
	var frames atomic.Int32
	var bad atomic.Value
	buf := make([]byte, w*h*3/2)
	s, err := Start(Config{Display: d.ID, Width: w, Height: h, FPS: 30}, func(f Frame) {
		if frames.Add(1) == 1 {
			if f.Width != w || f.Height != h {
				bad.Store("size")
			}
			if err := ToI420(f.PixelBuffer, buf, f.Width, f.Height); err != nil {
				bad.Store(err.Error())
			}
		}
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Something has to change for frames to come: the first always does.
	time.Sleep(time.Second)
	if err := s.Update(Config{Display: d.ID, Width: w / 2, Height: h / 2 &^ 1, FPS: 10}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	s.Stop()
	s.Stop()
	if frames.Load() == 0 {
		t.Fatal("no frames")
	}
	if v := bad.Load(); v != nil {
		t.Fatal(v)
	}
	t.Logf("%d frames", frames.Load())
}
