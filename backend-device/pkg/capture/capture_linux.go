// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux && !headless

package capture

/*
#cgo LDFLAGS: -lX11 -lXext -lXfixes -lXrandr
#include <stdlib.h>
#include "capture_linux.h"
*/
import "C"

import (
	"errors"
	"runtime"
	"sync"
	"time"
	"unsafe"
)

// Linux: the screen of an X11 desktop (XShm). A Wayland session lets no
// program read the screen this way: there HasAccess is false and Problem
// says why, and sessions show the test card.

// Supported reports whether this build captures the screen.
const Supported = true

// Problem is why the screen cannot be captured now ("" when it can).
func Problem() string {
	if C.dk_x11_ready() == 0 {
		return ProblemWayland
	}
	return ""
}

// HasAccess reports whether the screen can be captured: an X11 desktop.
func HasAccess() bool { return Problem() == "" }

// RequestAccess has nothing to ask on Linux.
func RequestAccess() bool { return HasAccess() }

// Displays lists the monitors, the primary one first-class (IDs from 1).
func Displays() ([]Display, error) {
	var mons [16]C.dk_monitor
	n := int(C.dk_monitors(&mons[0], 16))
	if n < 0 {
		return nil, ErrUnsupported
	}
	out := make([]Display, 0, n)
	for i := 0; i < n; i++ {
		m := mons[i]
		w, h := int(m.w), int(m.h)
		out = append(out, Display{
			ID: uint32(i + 1), Name: C.GoString(&m.name[0]),
			Width: w, Height: h, PointsWidth: w, PointsHeight: h,
			X: int(m.x), Y: int(m.y), Primary: m.primary != 0,
		})
	}
	return out, nil
}

// Stream is a running capture: a goroutine on its own thread that grabs
// the display at most FPS times a second and hands over what changed.
type Stream struct {
	mu      sync.Mutex
	cfg     Config
	stop    chan struct{}
	done    chan struct{}
	onFrame func(Frame)
	onErr   func(error)
}

// Start captures cfg's display, calling onFrame with each changed picture
// and onErr when the capture stops by itself.
func Start(cfg Config, onFrame func(Frame), onErr func(error)) (*Stream, error) {
	if !HasAccess() {
		return nil, ErrUnsupported
	}
	s := &Stream{cfg: cfg, stop: make(chan struct{}), done: make(chan struct{}), onFrame: onFrame, onErr: onErr}
	started := make(chan error, 1)
	go s.run(started)
	if err := <-started; err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Stream) run(started chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(s.done)
	c := C.dk_open()
	if c == nil {
		started <- errors.New("capture: cannot open the X display")
		return
	}
	defer C.dk_close(c)
	started <- nil
	start := time.Now()
	for {
		cfg := s.Config()
		fps := cfg.FPS
		if fps <= 0 {
			fps = 30
		}
		next := time.Now().Add(time.Second / time.Duration(fps))
		if f, err := s.grab(c, cfg); err != nil {
			if s.onErr != nil {
				s.onErr(err)
			}
			return
		} else if f != nil {
			s.onFrame(Frame{PixelBuffer: unsafe.Pointer(f), Width: int(f.w), Height: int(f.h), PTS: time.Since(start)})
			C.dk_release(f)
		}
		select {
		case <-s.stop:
			return
		case <-time.After(time.Until(next)):
		}
	}
}

func (s *Stream) grab(c *C.dk_cap, cfg Config) (*C.dk_frame, error) {
	ds, err := Displays()
	if err != nil || len(ds) == 0 {
		return nil, errors.New("capture: no display")
	}
	d := ds[0]
	for _, x := range ds {
		if (cfg.Display == 0 && x.Primary) || x.ID == cfg.Display {
			d = x
			break
		}
	}
	w, h := cfg.Width, cfg.Height
	if w <= 0 || h <= 0 {
		w, h = d.Width, d.Height
	}
	cursor := C.int(0)
	if cfg.ShowCursor {
		cursor = 1
	}
	var cerr C.int
	f := C.dk_grab(c, C.int(d.X), C.int(d.Y), C.int(d.Width), C.int(d.Height), C.int(w), C.int(h), cursor, &cerr)
	if cerr != 0 {
		return nil, errors.New("capture: the screen could not be read")
	}
	return f, nil
}

// Update changes what is captured, without stopping.
func (s *Stream) Update(cfg Config) error {
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	return nil
}

// Config is what is captured now.
func (s *Stream) Config() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// Stop ends the capture and waits for it.
func (s *Stream) Stop() {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	<-s.done
}

// OnAudio: the computer's sound is not captured on Linux yet.
func (s *Stream) OnAudio(func([]int16)) {}

// Retain keeps a frame past its callback.
func Retain(p unsafe.Pointer) { C.dk_retain((*C.dk_frame)(p)) }

// Release lets go of a retained frame.
func Release(p unsafe.Pointer) { C.dk_release((*C.dk_frame)(p)) }

// ToI420 copies a frame's picture, which is I420 already, into dst (w x h).
func ToI420(p unsafe.Pointer, dst []byte, w, h int) error {
	f := (*C.dk_frame)(p)
	if int(f.w) != w || int(f.h) != h {
		return errors.New("capture: the frame's size changed")
	}
	n := w * h * 3 / 2
	if len(dst) < n {
		return errors.New("capture: the buffer is too small")
	}
	copy(dst, unsafe.Slice((*byte)(unsafe.Pointer(f.data)), n))
	return nil
}
