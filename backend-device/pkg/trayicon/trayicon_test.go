// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package trayicon

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"testing"
)

func TestPNG(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(PNG()))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != size || b.Dy() != size {
		t.Fatalf("size %v", b)
	}
}

func TestICO(t *testing.T) {
	ico := ICO()
	var header [3]uint16
	_ = binary.Read(bytes.NewReader(ico[:6]), binary.LittleEndian, &header)
	if header != [3]uint16{0, 1, 1} {
		t.Fatalf("header %v", header)
	}
	length := binary.LittleEndian.Uint32(ico[14:18])
	offset := binary.LittleEndian.Uint32(ico[18:22])
	if int(offset+length) != len(ico) {
		t.Fatalf("entry points outside the file: offset %d length %d total %d", offset, length, len(ico))
	}
	if _, err := png.Decode(bytes.NewReader(ico[offset:])); err != nil {
		t.Fatalf("embedded png: %v", err)
	}
}

func TestTheAppIconHasAMarginAndTheMarkInTheMiddle(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(AppPNG()))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 512 || b.Dy() != 512 {
		t.Fatalf("size %v", img.Bounds())
	}
	if _, _, _, a := img.At(4, 4).RGBA(); a != 0 {
		t.Fatalf("corner should be transparent, alpha %d", a)
	}
	// The amber square fills the space between the gamepad's buttons.
	if r, g, _, a := img.At(256, 150).RGBA(); a == 0 || r>>8 < 0xE0 || g>>8 < 0x90 {
		t.Fatalf("expected amber at the top middle, got %d %d alpha %d", r>>8, g>>8, a)
	}
}
