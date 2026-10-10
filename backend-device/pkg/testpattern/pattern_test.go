// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package testpattern

import (
	"bytes"
	"testing"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/encoder"
)

func TestFrameSizeAndClock(t *testing.T) {
	p := New(640, 480)
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	a := bytes.Clone(p.Frame(t0, Controls{}))
	if len(a) != encoder.FrameSize(640, 480) {
		t.Fatalf("frame size %d", len(a))
	}
	if bytes.Equal(a, p.Frame(t0.Add(time.Second), Controls{})) {
		t.Fatal("the clock does not change the picture")
	}
	if !bytes.Equal(a, p.Frame(t0, Controls{})) {
		t.Fatal("the same time must draw the same frame")
	}
}

func TestInputIsVisible(t *testing.T) {
	p := New(640, 480)
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	idle := bytes.Clone(p.Frame(t0, Controls{}))
	at := Controls{PointerOn: true, PointerX: 20000, PointerY: 40000}
	pointer := bytes.Clone(p.Frame(t0, at))
	if bytes.Equal(idle, pointer) {
		t.Fatal("the pointer is not drawn")
	}
	at.Pressed = true
	if bytes.Equal(pointer, p.Frame(t0, at)) {
		t.Fatal("a press does not light the pointer")
	}
	if bytes.Equal(idle, p.Frame(t0, Controls{FPS: 59.9})) {
		t.Fatal("the frame rate is not drawn")
	}
	// The corners never draw outside the frame.
	p.Frame(t0, Controls{PointerOn: true, PointerX: 65535, PointerY: 65535})
	p.Frame(t0, Controls{PointerOn: true})
}

func TestCardEncodes(t *testing.T) {
	p := New(640, 480)
	enc, err := encoder.NewVP8(encoder.Config{Width: 640, Height: 480, FPS: 30, BitrateKbps: 1500})
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	if data, key, err := enc.Encode(p.Frame(time.Now(), Controls{}), false); err != nil || !key || len(data) == 0 {
		t.Fatalf("encode: %v %v %d", err, key, len(data))
	}
}
