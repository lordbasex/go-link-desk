// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package trayicon draws the app and tray icons at runtime from the brand
// mark (the same gamepad SVG as the web's logo), so the repository does not
// need binary image files.
package trayicon

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"

	"github.com/srwiley/rasterx"
	"golang.org/x/image/math/fixed"
)

const size = 64

var (
	amber = color.NRGBA{0xF2, 0xA3, 0x3A, 0xFF} // --color-accent
	dark  = color.NRGBA{0x1A, 0x12, 0x06, 0xFF} // --color-on-accent
)

// Render draws the brand mark (the web's logo: a dark gamepad on an amber
// rounded square, the gamepad SVG's shapes on a 32x32 canvas) as a px x px
// PNG. With inset, the square leaves a margin around it, as macOS app icons
// do (824 of 1024).
func Render(px int, inset bool) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, px, px))
	side, off := float64(px), 0.0
	if inset {
		side = float64(px) * 824 / 1024
		off = (float64(px) - side) / 2
	}
	u := side / 32 // one unit of the 32x32 canvas
	at := func(x, y float64) fixed.Point26_6 { return rasterx.ToFixedP(off+x*u, off+y*u) }
	scanner := rasterx.NewScannerGV(px, px, img, img.Bounds())

	fill := rasterx.NewFiller(px, px, scanner)
	fill.SetColor(amber)
	rasterx.AddRoundRect(off, off, off+side, off+side, 8*u, 8*u, 0, rasterx.RoundGap, fill)
	fill.Draw()

	// The gamepad: the 24x24 icon drawn 4 units in, stroke width 2.
	stroke := rasterx.NewStroker(px, px, scanner)
	stroke.SetColor(dark)
	stroke.SetStroke(fixed.Int26_6(2*u*64), fixed.Int26_6(4*64), rasterx.RoundCap, rasterx.RoundCap, rasterx.RoundGap, rasterx.Round)
	g := func(v float64) float64 { return off + (v+4)*u }
	rasterx.AddRoundRect(g(2), g(7), g(22), g(18), 4*u, 4*u, 0, rasterx.RoundGap, stroke)
	line := func(x1, y1, x2, y2 float64) {
		stroke.Start(at(x1+4, y1+4))
		stroke.Line(at(x2+4, y2+4))
		stroke.Stop(false)
	}
	line(7, 11, 7, 14)
	line(5.5, 12.5, 8.5, 12.5)
	rasterx.AddCircle(g(16), g(11.5), u, stroke)
	rasterx.AddCircle(g(18), g(13.5), u, stroke)
	stroke.Draw()

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// PNG returns the 64x64 tray icon.
func PNG() []byte { return Render(size, false) }

// AppPNG returns the 512x512 app icon (Dock, task bar, window).
func AppPNG() []byte { return Render(512, true) }

// ICO wraps the PNG in an .ico container, which Windows requires for
// tray icons. Since Windows Vista an ICO entry may hold PNG data as is.
func ICO() []byte {
	data := PNG()
	var buf bytes.Buffer
	// ICONDIR: reserved, type 1 (icon), 1 image.
	_ = binary.Write(&buf, binary.LittleEndian, [3]uint16{0, 1, 1})
	// ICONDIRENTRY: width, height, colors, reserved, planes, bpp, size, offset.
	buf.Write([]byte{size, size, 0, 0})
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(32))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(len(data)))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(6+16))
	buf.Write(data)
	return buf.Bytes()
}
