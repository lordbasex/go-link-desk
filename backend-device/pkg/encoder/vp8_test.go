// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build cgo

package encoder

import (
	"testing"
	"time"
)

func grayFrame(w, h int, luma byte) []byte {
	f := make([]byte, FrameSize(w, h))
	for i := range f {
		f[i] = 128
	}
	for i := 0; i < w*h; i++ {
		f[i] = luma
	}
	return f
}

// isVP8Keyframe checks bit 0 of the VP8 frame tag (0 = keyframe).
func isVP8Keyframe(b []byte) bool { return len(b) > 0 && b[0]&1 == 0 }

func TestEncodeAndForceKeyframe(t *testing.T) {
	enc, err := NewVP8(Config{Width: 320, Height: 240, FPS: 30, BitrateKbps: 500})
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()

	data, key, err := enc.Encode(grayFrame(320, 240, 60), false)
	if err != nil || len(data) == 0 || !key || !isVP8Keyframe(data) {
		t.Fatalf("first frame must be a keyframe: len=%d key=%v err=%v", len(data), key, err)
	}
	data, key, err = enc.Encode(grayFrame(320, 240, 70), false)
	if err != nil || key || isVP8Keyframe(data) {
		t.Fatalf("second frame should be a delta frame: key=%v err=%v", key, err)
	}
	data, key, err = enc.Encode(grayFrame(320, 240, 80), true)
	if err != nil || !key || !isVP8Keyframe(data) {
		t.Fatalf("forced keyframe missing: key=%v err=%v", key, err)
	}
}

func TestRejectsBadInput(t *testing.T) {
	if _, err := NewVP8(Config{}); err == nil {
		t.Fatal("empty config accepted")
	}
	enc, _ := NewVP8(Config{Width: 64, Height: 48, FPS: 30, BitrateKbps: 100})
	defer enc.Close()
	if _, _, err := enc.Encode(make([]byte, 10), false); err == nil {
		t.Fatal("short frame accepted")
	}
	enc.Close()
	if _, _, err := enc.Encode(grayFrame(64, 48, 0), false); err == nil {
		t.Fatal("encode after close accepted")
	}
}

func TestTuning(t *testing.T) {
	if _, err := NewVP8(Config{Width: 64, Height: 48, FPS: 30, BitrateKbps: 100, MinQuantizer: 40, MaxQuantizer: 20}); err == nil {
		t.Fatal("min quantizer above max accepted")
	}
	if _, err := NewVP8(Config{Width: 64, Height: 48, FPS: 30, BitrateKbps: 100, CPUUsed: 17}); err == nil {
		t.Fatal("cpu-used 17 accepted")
	}
	// A noisy keyframe is bigger with a lower quantizer ceiling.
	noisy := grayFrame(128, 96, 0)
	seed := uint32(1)
	for i := 0; i < 128*96; i++ {
		seed = seed*1664525 + 1013904223
		noisy[i] = byte(seed >> 24)
	}
	size := func(cfg Config) int {
		enc, err := NewVP8(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer enc.Close()
		data, _, err := enc.Encode(noisy, true)
		if err != nil {
			t.Fatal(err)
		}
		return len(data)
	}
	coarse := size(Config{Width: 128, Height: 96, FPS: 30, BitrateKbps: 50000, MinQuantizer: 50, MaxQuantizer: 63})
	fine := size(Config{Width: 128, Height: 96, FPS: 30, BitrateKbps: 50000, MinQuantizer: 1, MaxQuantizer: 10})
	if fine <= coarse {
		t.Fatalf("fine quantizer gave %d bytes, coarse %d", fine, coarse)
	}
}

// grain is a grey frame with faint pseudo-random grain from a seed: what
// it costs depends on the quantizer, unlike loud noise, which takes the
// coarsest one at any bitrate.
func grain(w, h int, seed uint32) []byte {
	f := grayFrame(w, h, 0)
	for i := 0; i < w*h; i++ {
		seed = seed*1664525 + 1013904223
		f[i] = 120 + byte(seed>>28)
	}
	return f
}

func TestTheBitrateChangesWithoutAKeyframe(t *testing.T) {
	enc, err := NewVP8(Config{Width: 320, Height: 240, FPS: 30, BitrateKbps: 500, Screen: true})
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	if _, _, err := enc.Encode(grayFrame(320, 240, 60), false); err != nil {
		t.Fatal(err)
	}
	if err := enc.SetBitrate(2000); err != nil {
		t.Fatal(err)
	}
	if _, key, err := enc.Encode(grayFrame(320, 240, 70), false); err != nil || key {
		t.Fatalf("after a bitrate change: key=%v err=%v, want a delta frame", key, err)
	}
	if err := enc.SetBitrate(0); err == nil {
		t.Fatal("bitrate 0 accepted")
	}
}

// Frames far apart get the bits of the time between them: a screen that
// changes ten times a second is not encoded as if it were sixty.
func TestFramesGetTheBitsOfTheirTime(t *testing.T) {
	total := func(gap time.Duration) int {
		enc, err := NewVP8(Config{Width: 320, Height: 240, FPS: 60, BitrateKbps: 600})
		if err != nil {
			t.Fatal(err)
		}
		defer enc.Close()
		n := 0
		for i := range 40 {
			data, _, err := enc.EncodeAt(grain(320, 240, uint32(i+1)), time.Duration(i)*gap, false)
			if err != nil {
				t.Fatal(err)
			}
			if i >= 10 { // past the first keyframe and the buffer filling
				n += len(data)
			}
		}
		return n
	}
	fast, slow := total(time.Second/60), total(time.Second/10)
	if slow < fast*2 {
		t.Errorf("frames 100 ms apart took %d bytes, 16 ms apart %d: want several times more", slow, fast)
	}
}

// A source whose clock starts again (a capture restarted) goes on from
// the last frame, with its frames as far apart as before.
func TestAClockThatStartsAgainGoesOn(t *testing.T) {
	enc, err := NewVP8(Config{Width: 64, Height: 48, FPS: 30, BitrateKbps: 100})
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	for _, at := range []time.Duration{time.Second, time.Second, 0, 100 * time.Millisecond} {
		if _, _, err := enc.EncodeAt(grayFrame(64, 48, 50), at, false); err != nil {
			t.Fatalf("at %v: %v", at, err)
		}
	}
	// 1 s; 1 s again (a period on); 0 (a period on); 100 ms after that.
	want := int64(timebase) + 2*int64(timebase/30) + int64(timebase/10)
	if enc.pts != want {
		t.Errorf("last time %d ticks, want %d", enc.pts, want)
	}
}
