// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build windows && !headless

package capture

import (
	"testing"
	"time"
	"unsafe"
)

func TestTheMonitorsAreListed(t *testing.T) {
	ds, err := Displays()
	if err != nil || len(ds) == 0 {
		t.Fatalf("displays %v %v", ds, err)
	}
	primary := 0
	for _, d := range ds {
		if d.Width <= 0 || d.Height <= 0 || d.Name == "" {
			t.Fatalf("display %+v", d)
		}
		if d.Primary {
			primary++
		}
	}
	if primary != 1 {
		t.Fatalf("%d primary displays in %+v", primary, ds)
	}
}

// The first grab always hands over a picture, at the size asked, as I420.
func TestTheScreenIsCaptured(t *testing.T) {
	got := make(chan [2]int, 1)
	var first []byte
	s, err := Start(Config{Width: 640, Height: 360, FPS: 30, ShowCursor: true}, func(f Frame) {
		buf := make([]byte, f.Width*f.Height*3/2)
		if ToI420(f.PixelBuffer, buf, f.Width, f.Height) != nil {
			return
		}
		select {
		case got <- [2]int{f.Width, f.Height}:
			first = buf
		default:
		}
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	select {
	case size := <-got:
		if size != [2]int{640, 360} {
			t.Fatalf("frame %v", size)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no frame")
	}
	if len(first) != 640*360*3/2 {
		t.Fatalf("I420 of %d bytes", len(first))
	}
}

func TestAFrameOutlivesItsCallback(t *testing.T) {
	kept := make(chan unsafe.Pointer, 1)
	s, err := Start(Config{Width: 320, Height: 240, FPS: 30}, func(f Frame) {
		Retain(f.PixelBuffer)
		select {
		case kept <- f.PixelBuffer:
		default:
			Release(f.PixelBuffer)
		}
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var p unsafe.Pointer
	select {
	case p = <-kept:
	case <-time.After(5 * time.Second):
		t.Fatal("no frame")
	}
	s.Stop()
	buf := make([]byte, 320*240*3/2)
	if err := ToI420(p, buf, 320, 240); err != nil {
		t.Fatal(err)
	}
	Release(p)
}

// The computer's sound comes 10 ms at a time, silence included while
// nothing plays (a machine with no output device has none to read).
func TestTheSoundComesAtItsPace(t *testing.T) {
	got := make(chan int, 10)
	tap := startAudio(func(s []int16) {
		select {
		case got <- len(s):
		default:
		}
	})
	if tap == nil {
		t.Skip("no output device to read")
	}
	defer tap.Stop()
	start := time.Now()
	for i := 0; i < 5; i++ {
		select {
		case n := <-got:
			if n != AudioRate/100*AudioChannels {
				t.Fatalf("a read of %d samples", n)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("no sound read")
		}
	}
	if time.Since(start) > time.Second {
		t.Fatalf("5 reads of 10 ms took %v", time.Since(start))
	}
}
