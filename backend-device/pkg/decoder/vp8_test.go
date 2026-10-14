// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build cgo

package decoder

import (
	"testing"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/encoder"
)

// What the host's VP8 encoder sends comes back at its size, close to the
// picture sent; a frame whose reference was lost says so.
func TestVP8RoundTrip(t *testing.T) {
	const w, h = 322, 182 // odd chroma sizes on purpose
	enc, err := encoder.NewVP8(encoder.Config{Width: w, Height: h, FPS: 30, BitrateKbps: 2000, Threads: 1, Screen: true})
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	dec, err := NewVP8(2)
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()

	frame := make([]byte, encoder.FrameSize(w, h))
	var frames [][]byte
	for i := 0; i < 4; i++ {
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				frame[y*w+x] = byte(16 + (x+i*8)%200) // a moving ramp
			}
		}
		for j := w * h; j < len(frame); j++ {
			frame[j] = 128
		}
		data, _, err := enc.EncodeAt(frame, time.Duration(i)*100*time.Millisecond, i == 0)
		if err != nil {
			t.Fatal(err)
		}
		frames = append(frames, data)
	}

	for i, f := range frames {
		p, err := dec.Decode(f)
		if err != nil || p == nil {
			t.Fatalf("frame %d: %v, %v", i, p, err)
		}
		if p.Width != w || p.Height != h || p.StrideY < w || p.StrideUV < (w+1)/2 {
			t.Fatalf("frame %d: %dx%d strides %d/%d", i, p.Width, p.Height, p.StrideY, p.StrideUV)
		}
		want := 16 + (100+i*8)%200
		if got := int(p.Y[(h/2)*p.StrideY+100]); got < want-12 || got > want+12 {
			t.Errorf("frame %d: luma %d, want about %d", i, got, want)
		}
		if got := int(p.U[(h/4)*p.StrideUV+40]); got < 120 || got > 136 {
			t.Errorf("frame %d: chroma %d, want about 128", i, got)
		}
	}

	// A new decoder that missed the keyframe: P frames build on nothing.
	late, err := NewVP8(1)
	if err != nil {
		t.Fatal(err)
	}
	defer late.Close()
	if p, err := late.Decode(frames[2]); err == nil && p != nil {
		t.Error("a P frame decoded without its keyframe")
	}
	if p, err := late.Decode(frames[0]); err != nil || p == nil {
		t.Errorf("the keyframe after: %v, %v", p, err)
	}
}
