// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build windows && !headless

package capture

/*
#cgo LDFLAGS: -ld3d11 -ldxgi -lgdi32 -luser32 -lole32
#include <stdlib.h>
#include "capture_windows.h"
*/
import "C"

import (
	"errors"
	"log/slog"
	"runtime"
	"sync"
	"time"
	"unsafe"
)

// Windows: the screen through the Desktop Duplication API (the GPU), or
// GDI where that cannot run (capture_windows.c); the sound through a
// WASAPI loopback stream (audio_windows.c). Windows asks nobody for them:
// any program of the person at the computer may read their screen and
// sound, so there is nothing to allow.

func init() { C.dk_win_dpi() }

// Supported reports whether this build captures the screen.
const Supported = true

// Problem is why the screen cannot be captured now ("" when it can).
func Problem() string { return "" }

// HasAccess reports whether the screen can be captured (always, here).
func HasAccess() bool { return true }

// RequestAccess has nothing to ask on Windows.
func RequestAccess() bool { return true }

// RemoteControl reports whether the desktop lets the viewers use the mouse
// and keyboard (always, here).
func RemoteControl() bool { return true }

// AsksInputApart: only a Wayland desktop asks for the mouse and keyboard
// apart.
func AsksInputApart() bool { return false }

// Displays lists the monitors (IDs from 1), with their own names.
func Displays() ([]Display, error) {
	ds := monitors(true)
	if ds == nil {
		return nil, ErrUnsupported
	}
	return ds, nil
}

// monitors are the desktop's monitors in real pixels (the process is DPI
// aware), named only when names (it asks the display configuration).
func monitors(names bool) []Display {
	var mons [16]C.dk_monitor
	n := int(C.dk_monitors(&mons[0], 16, cBool(names)))
	if n <= 0 {
		return nil
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
	return out
}

func cBool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

// Stream is a running capture: a goroutine on its own thread (the GPU's
// objects belong to it) that reads the display at most FPS times a second
// and hands over what changed.
type Stream struct {
	mu      sync.Mutex
	cfg     Config
	stop    chan struct{}
	done    chan struct{}
	onFrame func(Frame)
	onErr   func(error)
	onAudio func([]int16)
	audio   *audioTap // reading the sound while cfg.Audio
	how     C.int     // how the screen was read last: 1 the GPU, 2 GDI
}

// Start captures cfg's display, calling onFrame with each changed picture
// and onErr when the capture stops by itself.
func Start(cfg Config, onFrame func(Frame), onErr func(error)) (*Stream, error) {
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
		started <- errors.New("capture: out of memory")
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
	ds := monitors(false)
	if len(ds) == 0 {
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
	var cerr, how C.int
	f := C.dk_grab(c, C.int(d.X), C.int(d.Y), C.int(d.Width), C.int(d.Height), C.int(w), C.int(h), cBool(cfg.ShowCursor), &cerr, &how)
	if cerr != 0 {
		return nil, errors.New("capture: the screen could not be read")
	}
	if how != 0 && how != s.how {
		s.how = how
		slog.Info("capture: reading the screen", "with", map[C.int]string{1: "Desktop Duplication", 2: "GDI"}[how])
	}
	return f, nil
}

// Update changes what is captured, without stopping.
func (s *Stream) Update(cfg Config) error {
	s.mu.Lock()
	s.cfg = cfg
	s.syncAudio()
	s.mu.Unlock()
	return nil
}

// syncAudio reads the sound while it is wanted and someone takes it
// (under s.mu).
func (s *Stream) syncAudio() {
	want := s.cfg.Audio && s.onAudio != nil
	switch {
	case want && s.audio == nil:
		s.audio = startAudio(s.onAudio)
	case !want && s.audio != nil:
		s.audio.Stop()
		s.audio = nil
	}
}

// Config is what is captured now.
func (s *Stream) Config() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// Stop ends the capture and waits for it.
func (s *Stream) Stop() {
	s.mu.Lock()
	if s.audio != nil {
		s.audio.Stop()
		s.audio = nil
	}
	s.onAudio = nil
	s.mu.Unlock()
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	<-s.done
}

// OnAudio hands the computer's sound (interleaved stereo at 48 kHz) to f
// while the capture's Config asks for it. It is what the speakers play,
// the host's own sound included.
func (s *Stream) OnAudio(f func([]int16)) {
	s.mu.Lock()
	s.onAudio = f
	s.syncAudio()
	s.mu.Unlock()
}

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
