// compare.go: go run compare.go ref.png ours.png W H out.png — go-link on the left, LordOS's on the right.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"strconv"
)

func load(p string) image.Image {
	f, err := os.Open(p)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	m, err := png.Decode(f)
	if err != nil {
		panic(err)
	}
	return m
}
func main() {
	a := load(os.Args[1])
	b := load(os.Args[2])
	w, _ := strconv.Atoi(os.Args[3])
	h, _ := strconv.Atoi(os.Args[4])
	out := image.NewRGBA(image.Rect(0, 0, w*2+12, h))
	draw.Draw(out, out.Bounds(), image.NewUniform(color.RGBA{255, 0, 255, 255}), image.Point{}, draw.Src)
	draw.Draw(out, image.Rect(0, 0, w, h), a, a.Bounds().Min, draw.Src)
	draw.Draw(out, image.Rect(w+12, 0, w*2+12, h), b, image.Point{}, draw.Src)
	var diff, n uint64
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r1, g1, b1, _ := a.At(a.Bounds().Min.X+x, a.Bounds().Min.Y+y).RGBA()
			r2, g2, b2, _ := b.At(x, y).RGBA()
			d := func(p, q uint32) uint64 {
				if p > q {
					return uint64(p - q)
				}
				return uint64(q - p)
			}
			if d(r1, r2)+d(g1, g2)+d(b1, b2) > 0x6000 {
				diff++
			}
			n++
		}
	}
	f, _ := os.Create(os.Args[5])
	defer f.Close()
	png.Encode(f, out)
	fmt.Printf("%s differs %.1f%%\n", os.Args[5], float64(diff)*100/float64(n))
}
