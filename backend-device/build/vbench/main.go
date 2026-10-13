// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Command vbench measures how many frames a second this computer encodes
// a busy desktop at: a terminal's text scrolling without end, at the
// sizes sessions send, with the host's encoders and tuning (VP8 for
// screens, and the system's H.264 where there is one). It is what the
// minimum hardware in the docs comes from. Pin it to fewer cores to see a
// smaller computer (taskset -c 0 on Linux).
//
//	go run ./build/vbench -seconds 8
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"runtime"
	"strings"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/encoder"
)

var cpuUsed, maxQ int

func main() {
	seconds := flag.Float64("seconds", 6, "how long each size is encoded")
	sizes := flag.String("sizes", "1280x800,1920x1080,2560x1440,3840x2160", "picture sizes")
	h264 := flag.Bool("h264", true, "also the system's H.264 encoder, where there is one")
	flag.IntVar(&cpuUsed, "cpu-used", 0, "libvpx's speed (0: the host's; negative: fixed, positive: adaptive)")
	flag.IntVar(&maxQ, "max-q", 0, "libvpx's highest quantizer (0: the host's)")
	flag.Parse()

	fmt.Printf("cpu cores %d, %s/%s\n", runtime.NumCPU(), runtime.GOOS, runtime.GOARCH)
	fmt.Printf("%-10s %-6s %8s %8s %9s\n", "size", "codec", "fps", "ms/frame", "kbps")
	for _, s := range strings.Split(*sizes, ",") {
		var w, h int
		if _, err := fmt.Sscanf(s, "%dx%d", &w, &h); err != nil {
			fmt.Fprintln(os.Stderr, "bad size", s)
			os.Exit(2)
		}
		frames := scrolling(w, h)
		run(w, h, "vp8", frames, *seconds)
		if *h264 && encoder.H264Available() {
			run(w, h, "h264", frames, *seconds)
		}
	}
}

// run encodes the frames over and over for a while, as fast as the
// encoder goes, each a thirtieth of a second after the last (the rate
// control's view), and prints the frames a second and the bitrate.
func run(w, h int, codec string, frames [][]byte, seconds float64) {
	const fps, kbps = 30, 8000
	var bytes int
	var write func(i int, pts time.Duration) error
	var done func()
	switch codec {
	case "vp8":
		enc, err := encoder.NewVP8(encoder.Config{Width: w, Height: h, FPS: 60, BitrateKbps: kbps, Threads: vp8Threads(w, h), Screen: true, CPUUsed: cpuUsed, MaxQuantizer: maxQ})
		if err != nil {
			fmt.Println(codec, err)
			return
		}
		write = func(i int, pts time.Duration) error {
			data, _, err := enc.EncodeAt(frames[i%len(frames)], pts, i == 0)
			bytes += len(data)
			return err
		}
		done = enc.Close
	default:
		enc, err := encoder.NewH264(encoder.Config{Width: w, Height: h, FPS: 60, BitrateKbps: kbps, GOPFrames: 600}, func(au []byte, _ time.Duration) { bytes += len(au) })
		if err != nil {
			fmt.Println(codec, err)
			return
		}
		write = func(i int, pts time.Duration) error { return enc.WriteAt(frames[i%len(frames)], pts, i == 0) }
		done = enc.Close
	}
	start := time.Now()
	n := 0
	for time.Since(start).Seconds() < seconds {
		if err := write(n, time.Duration(n)*time.Second/fps); err != nil {
			fmt.Println(codec, err)
			return
		}
		n++
	}
	done()
	took := time.Since(start)
	media := float64(n) / fps // the seconds of picture the frames stand for
	fmt.Printf("%-10s %-6s %8.1f %8.2f %9.0f\n", fmt.Sprintf("%dx%d", w, h), codec,
		float64(n)/took.Seconds(), took.Seconds()*1000/float64(n), float64(bytes)*8/1000/media)
}

// vp8Threads is the host's (internal/services/video.go).
func vp8Threads(w, h int) int {
	switch px := w * h; {
	case px >= 2560*1440:
		return 6
	case px >= 1920*1080:
		return 4
	case px >= 1280*720:
		return 3
	default:
		return 2
	}
}

// scrolling makes a terminal full of text and the frames of it scrolling
// up by a line, as I420: a busy desktop, mostly text.
func scrolling(w, h int) [][]byte {
	const line = 16
	page := image.NewRGBA(image.Rect(0, 0, w, h+line*24))
	draw.Draw(page, page.Bounds(), &image.Uniform{color.RGBA{0x1e, 0x1f, 0x26, 0xff}}, image.Point{}, draw.Src)
	d := font.Drawer{Dst: page, Src: image.NewUniform(color.RGBA{0xd8, 0xdc, 0xe4, 0xff}), Face: basicfont.Face7x13}
	words := []string{"-rw-r--r--", "1", "root", "root", "4096", "Oct", "12", "11:07", "drwxr-xr-x", "lib", "share", "icons", "Yaru", "scalable", "actions", "x-cursor.cur", "zoom-in", "org.gnome.Settings", "changelog.Debian.gz", "copyright"}
	for y, k := line, 0; y < page.Bounds().Dy(); y += line {
		var b strings.Builder
		for b.Len() < w/7 {
			b.WriteString(words[k%len(words)])
			b.WriteString("  ")
			k += 7
		}
		d.Dot = fixed.P(8, y)
		d.DrawString(b.String())
	}
	var frames [][]byte
	for i := 0; i < 24; i++ {
		frames = append(frames, i420(page, i*line, w, h))
	}
	return frames
}

// i420 is the w x h window of img from row top, as packed I420 (BT.601).
func i420(img *image.RGBA, top, w, h int) []byte {
	out := make([]byte, encoder.FrameSize(w, h))
	cw := (w + 1) / 2
	u, v := out[w*h:], out[w*h+cw*((h+1)/2):]
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := img.RGBAAt(x, top+y)
			r, g, b := int(c.R), int(c.G), int(c.B)
			out[y*w+x] = uint8((66*r + 129*g + 25*b + 128 + 4096) >> 8)
			if x%2 == 0 && y%2 == 0 {
				u[(y/2)*cw+x/2] = uint8((-38*r - 74*g + 112*b + 128 + 32768) >> 8)
				v[(y/2)*cw+x/2] = uint8((112*r - 94*g - 18*b + 128 + 32768) >> 8)
			}
		}
	}
	return out
}
