// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package encoder

import (
	"bytes"
	"testing"
	"time"
)

// Windows' encoder makes a stream a viewer can start: the first access
// unit is a keyframe with its SPS and PPS, a keyframe asked for comes
// with them too, and times follow the frames'.
func TestWindowsEncodesH264(t *testing.T) {
	type au struct {
		data []byte
		pts  time.Duration
	}
	var got []au
	enc, err := NewH264(Config{Width: 640, Height: 360, FPS: 30, BitrateKbps: 2000, GOPFrames: 300}, func(b []byte, pts time.Duration) {
		got = append(got, au{b, pts})
	})
	if err != nil {
		t.Skipf("no Media Foundation H.264 encoder here: %v", err)
	}
	defer enc.Close()
	frame := make([]byte, FrameSize(640, 360))
	for i := 0; i < 60; i++ {
		for j := range frame[:640*360] {
			frame[j] = byte(i*4 + j%200) // a moving picture
		}
		if err := enc.WriteAt(frame, time.Duration(i)*time.Second/30, i == 45); err != nil {
			t.Fatal(err)
		}
	}
	if len(got) < 50 {
		t.Fatalf("%d access units for 60 frames", len(got))
	}
	types := func(b []byte) (out []byte) {
		for _, n := range nalUnits(b) {
			out = append(out, n[0]&0x1f)
		}
		return
	}
	first := types(got[0].data)
	if !bytes.Contains(first, []byte{7}) || !bytes.Contains(first, []byte{8}) || !bytes.Contains(first, []byte{5}) {
		t.Fatalf("the first access unit has NAL types %v, want SPS, PPS and an IDR", first)
	}
	keys := 0
	for _, a := range got[1:] {
		ts := types(a.data)
		if bytes.Contains(ts, []byte{5}) {
			keys++
			if !bytes.Contains(ts, []byte{7}) {
				t.Fatalf("a keyframe without its SPS: %v", ts)
			}
		}
	}
	if keys == 0 {
		t.Fatal("the keyframe asked for never came")
	}
	if last := got[len(got)-1].pts; last < time.Second || last > 2*time.Second {
		t.Fatalf("the last frame's time is %v", last)
	}
}

func TestNALUnitsSplitAnnexB(t *testing.T) {
	b := []byte{0, 0, 0, 1, 0x67, 1, 2, 0, 0, 1, 0x68, 3, 0, 0, 0, 1, 0x65, 4, 5}
	got := nalUnits(b)
	if len(got) != 3 || got[0][0] != 0x67 || len(got[0]) != 3 || got[1][0] != 0x68 || len(got[1]) != 2 || got[2][0] != 0x65 {
		t.Fatalf("%x", got)
	}
}
