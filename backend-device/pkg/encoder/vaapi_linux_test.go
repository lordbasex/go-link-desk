// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package encoder

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Without a GPU (CI, a virtual machine) the encoder does not open, says
// why, and H.264 is not offered: sessions use VP8.
func TestVAAPIWithoutAGPUSaysWhy(t *testing.T) {
	if nodes, _ := filepath.Glob("/dev/dri/renderD*"); len(nodes) > 0 {
		t.Skip("this computer has a GPU: TestVAAPIEncodes runs instead")
	}
	_, err := NewH264(Config{Width: 256, Height: 144, FPS: 30, BitrateKbps: 500}, func([]byte, time.Duration) {})
	if err == nil || !strings.Contains(err.Error(), "VA-API") {
		t.Fatalf("err %v, want why VA-API did not open", err)
	}
	if H264Available() {
		t.Error("H.264 offered without an encoder")
	}
}

// With a GPU whose driver encodes H.264 (Intel, AMD): an IDR with its SPS
// and PPS first, P frames after, a keyframe when asked, at any size.
func TestVAAPIEncodes(t *testing.T) {
	if nodes, _ := filepath.Glob("/dev/dri/renderD*"); len(nodes) == 0 {
		t.Skip("no GPU")
	}
	if !H264Available() {
		t.Skip("the GPU's driver does not encode H.264")
	}
	for _, size := range [][2]int{{1280, 720}, {1366, 768}, {1920, 1080}} {
		w, h := size[0], size[1]
		var aus [][]byte
		var times []time.Duration
		e, err := NewH264(Config{Width: w, Height: h, FPS: 60, BitrateKbps: 4000}, func(au []byte, pts time.Duration) {
			aus = append(aus, au)
			times = append(times, pts)
		})
		if err != nil {
			t.Fatalf("%dx%d: %v", w, h, err)
		}
		frame := make([]byte, FrameSize(w, h))
		for i := 0; i < 6; i++ {
			for j := range frame {
				frame[j] = byte(j*7 + i*13)
			}
			if err := e.WriteAt(frame, time.Duration(i)*100*time.Millisecond, i == 4); err != nil {
				t.Fatalf("%dx%d frame %d: %v", w, h, i, err)
			}
		}
		e.SetBitrate(1500)
		if err := e.WriteAt(frame, 700*time.Millisecond, false); err != nil {
			t.Fatalf("%dx%d after a bitrate change: %v", w, h, err)
		}
		e.Close()
		if len(aus) != 7 {
			t.Fatalf("%dx%d: %d access units, want 7", w, h, len(aus))
		}
		kinds := func(au []byte) map[byte]bool {
			k := map[byte]bool{}
			for _, nal := range nalUnits(au) {
				k[nal[0]&0x1f] = true
			}
			return k
		}
		if k := kinds(aus[0]); !k[7] || !k[8] || !k[5] {
			t.Errorf("%dx%d: the first frame has NAL types %v, want SPS, PPS and IDR", w, h, k)
		}
		if k := kinds(aus[1]); k[5] || !k[1] {
			t.Errorf("%dx%d: the second frame has NAL types %v, want a P slice", w, h, k)
		}
		if k := kinds(aus[4]); !k[7] || !k[5] {
			t.Errorf("%dx%d: the asked keyframe has NAL types %v, want SPS and IDR", w, h, k)
		}
		if times[3] != 300*time.Millisecond {
			t.Errorf("%dx%d: the fourth frame's time %v, want 300ms", w, h, times[3])
		}
	}
}
