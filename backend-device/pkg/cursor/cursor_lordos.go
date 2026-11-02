// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux && headless && lordos

package cursor

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/png"
	"sync"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/lordshare"
)

// Supported reports whether this build reads the pointer.
const Supported = true

// Position is the pointer in session pixels; outside the picture while
// the system says it is hidden.
func Position() (x, y float64) {
	c := lordshare.Shared()
	if c == nil {
		return -1, -1
	}
	p := c.Pointer()
	if !p.Seen || p.Mode == 2 {
		return -1, -1
	}
	return float64(p.X), float64(p.Y)
}

var (
	mu       sync.Mutex
	lastGen  = -1
	lastMode = -1
	last     Shape
)

// Current is the pointer's image: the app's, or the arrow.
func Current() (Shape, error) {
	c := lordshare.Shared()
	if c == nil {
		return Shape{}, errors.New("cursor: no share")
	}
	p, s := c.Pointer(), c.Shape()
	mode := 0
	if p.Mode == 1 && s.ARGB != nil {
		mode = 1
	}
	mu.Lock()
	defer mu.Unlock()
	if mode == lastMode && (mode == 0 || s.Gen == lastGen) {
		return last, nil
	}
	shape, err := arrow()
	if mode == 1 {
		shape, err = fromARGB(s)
	}
	if err != nil {
		return Shape{}, err
	}
	lastMode, lastGen, last = mode, s.Gen, shape
	return shape, nil
}

func fromARGB(s lordshare.Shape) (Shape, error) {
	img := image.NewNRGBA(image.Rect(0, 0, s.W, s.H))
	for y := range s.H {
		for x := range s.W {
			v := binary.LittleEndian.Uint32(s.ARGB[(y*s.W+x)*4:])
			img.SetNRGBA(x, y, color.NRGBA{R: byte(v >> 16), G: byte(v >> 8), B: byte(v), A: byte(v >> 24)})
		}
	}
	return shapeOf(img, s.HotX, s.HotY)
}

// arrow is drawn here: the system sends no image for its own.
func arrow() (Shape, error) {
	rows := []string{
		"X", "XX", "X.X", "X..X", "X...X", "X....X", "X.....X", "X......X", "X.......X", "X........X",
		"X.....XXXXX", "X..X..X", "X.X X..X", "XX  X..X", "X    X..X", "     X..X", "      XX",
	}
	img := image.NewNRGBA(image.Rect(0, 0, 11, len(rows)))
	for y, r := range rows {
		for x, ch := range r {
			switch ch {
			case 'X':
				img.SetNRGBA(x, y, color.NRGBA{A: 255})
			case '.':
				img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			}
		}
	}
	return shapeOf(img, 0, 0)
}

func shapeOf(img *image.NRGBA, hx, hy int) (Shape, error) {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return Shape{}, err
	}
	sum := sha256.Sum256(b.Bytes())
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	return Shape{ID: hex.EncodeToString(sum[:8]), PNG: b.Bytes(), HotX: hx, HotY: hy, Width: w, Height: h, PointsWidth: w, PointsHeight: h}, nil
}
