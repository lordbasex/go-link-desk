// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

package decoder

import (
	"testing"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/encoder"
)

// What the host's own encoder makes, this decoder shows: a grey picture
// comes back grey, at its size.
func TestH264RoundTrip(t *testing.T) {
	const w, h = 320, 240
	var aus [][]byte
	enc, err := encoder.NewH264(encoder.Config{Width: w, Height: h, FPS: 30, BitrateKbps: 1000}, func(au []byte, _ time.Duration) {
		aus = append(aus, append([]byte(nil), au...))
	})
	if err != nil {
		t.Skip("no H.264 encoder here:", err)
	}
	frame := make([]byte, w*h*3/2)
	for i := range frame {
		frame[i] = 128
	}
	for i := 0; i < 5; i++ {
		if err := enc.WriteKey(frame, i == 0); err != nil {
			t.Fatal(err)
		}
	}
	enc.Close()
	if len(aus) == 0 {
		t.Fatal("the encoder made nothing")
	}
	var pics []Picture
	d := NewH264(func(p Picture) { pics = append(pics, p) })
	defer d.Close()
	for i, au := range aus {
		if err := d.Decode(au, time.Duration(i)*33*time.Millisecond); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
	}
	if len(pics) == 0 {
		t.Fatal("nothing decoded")
	}
	p := pics[len(pics)-1]
	if p.Bounds().Dx() != w || p.Bounds().Dy() != h {
		t.Fatalf("size %v", p.Bounds())
	}
	c := p.RGBAAt(w/2, h/2)
	if c.R < 100 || c.R > 156 || c.G < 100 || c.G > 156 {
		t.Fatalf("the middle is %v, not grey", c)
	}
	// A frame before any keyframe is refused, asking for one.
	d2 := NewH264(func(Picture) {})
	defer d2.Close()
	if err := d2.Decode(aus[len(aus)-1], 0); err != ErrNeedKeyframe {
		t.Fatalf("a delta frame first: %v", err)
	}
}
