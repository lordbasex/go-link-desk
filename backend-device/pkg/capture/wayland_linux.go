// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux && !headless

package capture

/*
#cgo CFLAGS: -I/usr/include/pipewire-0.3 -I/usr/include/spa-0.2
#cgo LDFLAGS: -ldl -lpthread
#include <stdlib.h>
#include "pipewire_linux.h"
*/
import "C"

import (
	"context"
	"errors"
	"time"
	"unsafe"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/portal"
)

// A Wayland desktop shares its screen through the remote desktop portal:
// the desktop asks the person at the computer once (later sessions reuse
// the permission), then the monitors arrive as PipeWire streams. The
// pointer comes apart from the picture: it is drawn in here when the
// viewer wants it in the picture, and handed to the cursor package
// otherwise.

// waylandReady reports whether a Wayland desktop can be captured.
func waylandReady() bool { return portal.Available() && C.dk_pw_load() != 0 }

// RemoteControl reports whether the desktop lets the viewers use the mouse
// and keyboard: a Wayland desktop's person may share the screen only.
func RemoteControl() bool {
	if !portal.Wayland() {
		return true
	}
	s := portal.Active()
	return s == nil || s.Control()
}

// waylandDisplays are the shared monitors while a session runs, else the
// monitors as XWayland sees them (they match the desktop's at scale 1).
func waylandDisplays() []Display {
	if s := portal.Active(); s != nil {
		out := make([]Display, 0, len(s.Streams()))
		for i, st := range s.Streams() {
			out = append(out, Display{
				ID: uint32(i + 1), Name: "Monitor " + string(rune('1'+i)),
				Width: st.W, Height: st.H, PointsWidth: st.W, PointsHeight: st.H,
				X: st.X, Y: st.Y, Primary: i == 0,
			})
		}
		return out
	}
	if ds := x11Displays(); len(ds) > 0 {
		return ds
	}
	return []Display{{ID: 1, Name: "Screen", Width: 1920, Height: 1080, PointsWidth: 1920, PointsHeight: 1080, Primary: true}}
}

// streamFor is the session's stream that shows display id: the one under
// its middle.
func streamFor(s *portal.Session, id uint32) portal.Stream {
	sts := s.Streams()
	var d *Display
	for _, x := range waylandDisplays() {
		if (id == 0 && x.Primary) || x.ID == id {
			d = &x
			break
		}
	}
	if d != nil {
		cx, cy := d.X+d.PointsWidth/2, d.Y+d.PointsHeight/2
		for _, st := range sts {
			if cx >= st.X && cx < st.X+st.W && cy >= st.Y && cy < st.Y+st.H {
				return st
			}
		}
	}
	return sts[0]
}

// errStopped: the desktop ended the sharing (its stop button, or the
// stream failed).
var errStopped = errors.New("capture: the desktop stopped sharing the screen")

func (s *Stream) runWayland(started chan<- error) {
	defer close(s.done)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-s.stop:
			cancel()
		case <-ctx.Done():
		}
	}()
	sess, err := portal.Acquire(ctx)
	if err != nil {
		if errors.Is(err, portal.ErrDenied) {
			err = ErrDenied
		}
		started <- err
		return
	}
	defer sess.Release()

	var p *C.dk_pw
	var st portal.Stream
	open := func(cfg Config) error {
		if p != nil {
			C.dk_pw_close(p)
			p = nil
		}
		st = streamFor(sess, cfg.Display)
		fd, err := sess.PipeWire()
		if err != nil {
			return err
		}
		var msg [256]C.char
		p = C.dk_pw_open(C.int(fd), C.uint32_t(st.Node), &msg[0], C.int(len(msg)))
		if p == nil {
			return errors.New("capture: " + C.GoString(&msg[0]))
		}
		return nil
	}
	cfg := s.Config()
	if err := open(cfg); err != nil {
		started <- err
		return
	}
	defer func() {
		if p != nil {
			C.dk_pw_close(p)
		}
	}()
	started <- nil

	start := time.Now()
	node := st.Node
	var shape C.uint
	fail := func(err error) {
		if s.onErr != nil {
			s.onErr(err)
		}
	}
	for {
		cfg := s.Config()
		fps := cfg.FPS
		if fps <= 0 {
			fps = 30
		}
		next := time.Now().Add(time.Second / time.Duration(fps))
		if streamFor(sess, cfg.Display).Node != node {
			if err := open(cfg); err != nil {
				fail(err)
				return
			}
			node = st.Node
		}
		if C.dk_pw_failed(p) != 0 {
			fail(errStopped)
			return
		}
		var pw, ph C.int
		C.dk_pw_size(p, &pw, &ph)
		if pw > 0 && ph > 0 {
			s.followPointer(p, st, int(pw), int(ph), &shape)
			w, h := cfg.Width, cfg.Height
			if w <= 0 || h <= 0 {
				w, h = int(pw), int(ph)
			}
			cursor := C.int(0)
			if cfg.ShowCursor {
				cursor = 1
			}
			if f := C.dk_pw_take(p, C.int(w), C.int(h), cursor); f != nil {
				s.onFrame(Frame{PixelBuffer: unsafe.Pointer(f), Width: int(f.w), Height: int(f.h), PTS: time.Since(start)})
				C.dk_release(f)
			}
		}
		select {
		case <-s.stop:
			return
		case <-sess.Closed():
			fail(errStopped)
			return
		case <-time.After(time.Until(next)):
		}
	}
}

// followPointer hands the stream's pointer to the cursor package, on the
// desktop's coordinates.
func (s *Stream) followPointer(p *C.dk_pw, st portal.Stream, pw, ph int, shape *C.uint) {
	var c C.dk_pw_pointer
	C.dk_pw_pointer_get(p, &c, *shape)
	if c.known == 0 {
		return
	}
	if c.rgba != nil {
		w, h := int(c.w), int(c.h)
		portal.SetPointerShape(C.GoBytes(unsafe.Pointer(c.rgba), C.int(w*h*4)), w, h, int(c.hot_x), int(c.hot_y))
		C.free(unsafe.Pointer(c.rgba))
		*shape = c.serial
	}
	x := float64(st.X) + float64(c.x)*float64(st.W)/float64(pw)
	y := float64(st.Y) + float64(c.y)*float64(st.H)/float64(ph)
	portal.SetPointerPosition(x, y, c.visible != 0)
}
