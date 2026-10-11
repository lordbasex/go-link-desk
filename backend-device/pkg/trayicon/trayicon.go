// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package trayicon draws the app and tray icons at runtime from the brand
// mark (the web's screen icon, with a pointer on it), so the repository
// does not need binary image files.
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
	// live is the tray icon's square while a session is on: red, so the
	// person at the computer always sees that someone is watching.
	live = color.NRGBA{0xE5, 0x48, 0x4D, 0xFF}
	// graphite is the dark tray icon's square.
	graphite = color.NRGBA{0x22, 0x27, 0x33, 0xFF}
)

// Render draws the brand mark (the web's screen icon: a dark screen on its
// stand, on an amber rounded square, with a pointer on the screen; the
// 24x24 icon drawn 4 units into a 32x32 canvas) as a px x px PNG. With
// inset, the square leaves a margin around it, as macOS app icons do (824
// of 1024).
func Render(px int, inset bool) []byte { return render(px, inset, amber, dark) }

func render(px int, inset bool, square, ink color.NRGBA) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, px, px))
	side, off := float64(px), 0.0
	if inset {
		side = float64(px) * 824 / 1024
		off = (float64(px) - side) / 2
	}
	u := side / 32 // one unit of the 32x32 canvas
	at := func(x, y float64) fixed.Point26_6 { return rasterx.ToFixedP(off+(x+4)*u, off+(y+4)*u) }
	g := func(v float64) float64 { return off + (v+4)*u }
	scanner := rasterx.NewScannerGV(px, px, img, img.Bounds())

	fill := rasterx.NewFiller(px, px, scanner)
	fill.SetColor(square)
	rasterx.AddRoundRect(off, off, off+side, off+side, 8*u, 8*u, 0, rasterx.RoundGap, fill)
	fill.Draw()

	// The screen and its stand, stroke width 2.
	stroke := rasterx.NewStroker(px, px, scanner)
	stroke.SetColor(ink)
	stroke.SetStroke(fixed.Int26_6(2*u*64), fixed.Int26_6(4*64), rasterx.RoundCap, rasterx.RoundCap, rasterx.RoundGap, rasterx.Round)
	rasterx.AddRoundRect(g(2), g(4), g(22), g(17), 2*u, 2*u, 0, rasterx.RoundGap, stroke)
	line := func(x1, y1, x2, y2 float64) {
		stroke.Start(at(x1, y1))
		stroke.Line(at(x2, y2))
		stroke.Stop(false)
	}
	line(8, 21, 16, 21)
	line(12, 17, 12, 21)
	stroke.Draw()

	// The pointer, on the screen.
	pointer := rasterx.NewFiller(px, px, scanner)
	pointer.SetColor(ink)
	pts := [][2]float64{{9.6, 7}, {9.6, 14.2}, {11.4, 12.5}, {12.7, 15.1}, {14.1, 14.4}, {12.8, 11.9}, {15.2, 11.9}}
	pointer.Start(at(pts[0][0], pts[0][1]))
	for _, p := range pts[1:] {
		pointer.Line(at(p[0], p[1]))
	}
	pointer.Stop(true)
	pointer.Draw()

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// PNG returns the 64x64 tray icon.
func PNG() []byte { return Render(size, false) }

// LivePNG returns the 64x64 tray icon shown while a session is on: the
// same mark on a red square.
func LivePNG() []byte { return render(size, false, live, dark) }

// DarkPNG returns the 64x64 dark tray icon: the mark in amber on a
// graphite square.
func DarkPNG() []byte { return render(size, false, graphite, amber) }

// AppPNG returns the 512x512 app icon (Dock, task bar, window).
func AppPNG() []byte { return RenderApp(512, false) }

// AppDarkPNG returns the 512x512 dark app icon.
func AppDarkPNG() []byte { return RenderApp(512, true) }

// RenderApp draws the app icon with depth, as macOS app icons have: the
// amber square lit from above with a soft shadow under it, the screen as
// dark glass with a reflection, and a light pointer on it. The tray icon
// stays flat (Render). graphite draws the dark variant: a dark square, the
// screen's bezel and the pointer in amber.
func RenderApp(px int, graphite bool) []byte { return renderApp(px, graphite, false) }

// RenderAppSquare draws the color app icon to the edges, without rounded
// corners or a shadow and with no transparency, as iOS wants its icon (the
// system rounds it).
func RenderAppSquare(px int) []byte { return renderApp(px, false, true) }

func renderApp(px int, graphite, square bool) []byte {
	side := float64(px) * 824 / 1024
	if square {
		side = float64(px)
	}
	off := (float64(px) - side) / 2
	u := side / 32
	g := func(v float64) float64 { return off + (v+4)*u }
	at := func(x, y float64) fixed.Point26_6 { return rasterx.ToFixedP(g(x), g(y)) }
	f := float64(px)

	shape := func(dy float64) *image.Alpha {
		return mask(px, func(sc *rasterx.ScannerGV) {
			fl := rasterx.NewFiller(px, px, sc)
			if square {
				rasterx.AddRect(0, 0, f, f, 0, fl)
			} else {
				rasterx.AddRoundRect(off, off+dy, off+side, off+side+dy, 8*u, 8*u, 0, rasterx.RoundGap, fl)
			}
			fl.Draw()
		})
	}
	img := image.NewNRGBA(image.Rect(0, 0, px, px))

	// The shadow: the square a little lower, blurred, faint.
	if !square {
		shadow := blur(shape(f*0.012), max(1, px/64))
		paint(img, shadow, func(x, y int) color.NRGBA { return color.NRGBA{0, 0, 0, 0x55} })
	}

	// The square: amber, lighter at the top; a gloss on its upper half.
	body := shape(0)
	top, bottom := color.NRGBA{0xFF, 0xC2, 0x5E, 0xFF}, color.NRGBA{0xE5, 0x86, 0x22, 0xFF}
	ink := dark
	lightTop, lightBottom := color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}, color.NRGBA{0xFF, 0xE0, 0xB0, 0xFF}
	if graphite {
		top, bottom = color.NRGBA{0x3A, 0x41, 0x52, 0xFF}, color.NRGBA{0x12, 0x15, 0x1D, 0xFF}
		ink = amber
		lightTop, lightBottom = color.NRGBA{0xFF, 0xC9, 0x7A, 0xFF}, color.NRGBA{0xF2, 0xA3, 0x3A, 0xFF}
	}
	paint(img, body, func(x, y int) color.NRGBA { return mix(top, bottom, (float64(y)-off)/side) })
	paint(img, body, func(x, y int) color.NRGBA {
		t := (float64(y) - off) / side
		return color.NRGBA{0xFF, 0xFF, 0xFF, uint8(max(0, 0.22-t*0.5) * 255)}
	})

	// The screen: dark glass, a reflection across its top-left.
	glass := mask(px, func(sc *rasterx.ScannerGV) {
		fl := rasterx.NewFiller(px, px, sc)
		rasterx.AddRoundRect(g(2), g(4), g(22), g(17), 2*u, 2*u, 0, rasterx.RoundGap, fl)
		fl.Draw()
	})
	paint(img, glass, func(x, y int) color.NRGBA {
		return mix(color.NRGBA{0x2C, 0x33, 0x45, 0xFF}, color.NRGBA{0x10, 0x13, 0x1B, 0xFF}, (float64(y)-g(4))/(13*u))
	})
	shine := mask(px, func(sc *rasterx.ScannerGV) {
		fl := rasterx.NewFiller(px, px, sc)
		fl.Start(at(3, 5))
		fl.Line(at(13, 5))
		fl.Line(at(5.5, 16))
		fl.Line(at(3, 16))
		fl.Stop(true)
		fl.Draw()
	})
	paint(img, and(shine, glass), func(x, y int) color.NRGBA { return color.NRGBA{0xFF, 0xFF, 0xFF, 0x14} })

	// The bezel and the stand, dark; the pointer, light with a dark edge.
	edge := mask(px, func(sc *rasterx.ScannerGV) {
		st := rasterx.NewStroker(px, px, sc)
		st.SetStroke(fixed.Int26_6(2*u*64), fixed.Int26_6(4*64), rasterx.RoundCap, rasterx.RoundCap, rasterx.RoundGap, rasterx.Round)
		rasterx.AddRoundRect(g(2), g(4), g(22), g(17), 2*u, 2*u, 0, rasterx.RoundGap, st)
		st.Start(at(8, 21))
		st.Line(at(16, 21))
		st.Stop(false)
		st.Start(at(12, 17))
		st.Line(at(12, 21))
		st.Stop(false)
		st.Draw()
	})
	paint(img, edge, func(x, y int) color.NRGBA { return ink })
	arrow := func(a rasterx.Adder) {
		pts := [][2]float64{{9.6, 7}, {9.6, 14.2}, {11.4, 12.5}, {12.7, 15.1}, {14.1, 14.4}, {12.8, 11.9}, {15.2, 11.9}}
		a.Start(at(pts[0][0], pts[0][1]))
		for _, p := range pts[1:] {
			a.Line(at(p[0], p[1]))
		}
		a.Stop(true)
	}
	outline := mask(px, func(sc *rasterx.ScannerGV) {
		st := rasterx.NewStroker(px, px, sc)
		st.SetStroke(fixed.Int26_6(0.9*u*64), fixed.Int26_6(4*64), rasterx.RoundCap, rasterx.RoundCap, rasterx.RoundGap, rasterx.Round)
		arrow(st)
		st.Draw()
	})
	paint(img, outline, func(x, y int) color.NRGBA { return color.NRGBA{0x1A, 0x12, 0x06, 0xFF} })
	pointer := mask(px, func(sc *rasterx.ScannerGV) {
		fl := rasterx.NewFiller(px, px, sc)
		arrow(fl)
		fl.Draw()
	})
	paint(img, pointer, func(x, y int) color.NRGBA {
		return mix(lightTop, lightBottom, (float64(y)-g(7))/(8*u))
	})

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// mask rasterizes draw's shapes into an alpha mask.
func mask(px int, draw func(*rasterx.ScannerGV)) *image.Alpha {
	img := image.NewNRGBA(image.Rect(0, 0, px, px))
	sc := rasterx.NewScannerGV(px, px, img, img.Bounds())
	sc.SetColor(color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF})
	draw(sc)
	m := image.NewAlpha(img.Bounds())
	for i := range m.Pix {
		m.Pix[i] = img.Pix[i*4+3]
	}
	return m
}

// and keeps what is in both masks.
func and(a, b *image.Alpha) *image.Alpha {
	m := image.NewAlpha(a.Bounds())
	for i := range m.Pix {
		m.Pix[i] = uint8(uint16(a.Pix[i]) * uint16(b.Pix[i]) / 255)
	}
	return m
}

// blur spreads a mask (a box blur, three times: nearly Gaussian).
func blur(m *image.Alpha, r int) *image.Alpha {
	w, h := m.Bounds().Dx(), m.Bounds().Dy()
	cur := m.Pix
	for pass := 0; pass < 6; pass++ {
		next := make([]uint8, len(cur))
		horizontal := pass%2 == 0
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				sum, n := 0, 0
				for d := -r; d <= r; d++ {
					xx, yy := x, y
					if horizontal {
						xx += d
					} else {
						yy += d
					}
					if xx < 0 || yy < 0 || xx >= w || yy >= h {
						n++
						continue
					}
					sum += int(cur[yy*w+xx])
					n++
				}
				next[y*w+x] = uint8(sum / n)
			}
		}
		cur = next
	}
	out := image.NewAlpha(m.Bounds())
	out.Pix = cur
	return out
}

// paint lays color over img where the mask is, blending.
func paint(img *image.NRGBA, m *image.Alpha, col func(x, y int) color.NRGBA) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			a := float64(m.Pix[y*m.Stride+x]) / 255
			if a == 0 {
				continue
			}
			c := col(x, y)
			a *= float64(c.A) / 255
			i := y*img.Stride + x*4
			da := float64(img.Pix[i+3]) / 255
			oa := a + da*(1-a)
			if oa == 0 {
				continue
			}
			for k, v := range []uint8{c.R, c.G, c.B} {
				img.Pix[i+k] = uint8((float64(v)*a + float64(img.Pix[i+k])*da*(1-a)) / oa)
			}
			img.Pix[i+3] = uint8(oa * 255)
		}
	}
}

// mix goes from a to b as t goes from 0 to 1.
func mix(a, b color.NRGBA, t float64) color.NRGBA {
	t = min(1, max(0, t))
	l := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return color.NRGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), l(a.A, b.A)}
}

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
