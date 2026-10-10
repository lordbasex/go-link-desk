// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package encoder

import "testing"

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
