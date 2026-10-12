// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"

	screencap "github.com/lordbasex/go-link-desk/backend-device/pkg/capture"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/keymap"
)

// fakeInjector records what it is asked to do.
type fakeInjector struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeInjector) rec(format string, a ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fmt.Sprintf(format, a...))
}

func (f *fakeInjector) Move(x, y float64) { f.rec("move %.0f,%.0f", x, y) }
func (f *fakeInjector) MoveBy(dx, dy int) { f.rec("moveby %d,%d", dx, dy) }
func (f *fakeInjector) Button(b int, down bool, x, y float64, at bool) {
	f.rec("button %d %v %.0f,%.0f %v", b, down, x, y, at)
}
func (f *fakeInjector) Scroll(dx, dy int, lines bool) { f.rec("scroll %d,%d %v", dx, dy, lines) }
func (f *fakeInjector) Key(vk int, down, repeat bool, mods uint8) {
	f.rec("key %d %v %v %d", vk, down, repeat, mods)
}
func (f *fakeInjector) Text(s string) { f.rec("text %s", s) }
func (f *fakeInjector) ReleaseAll()   { f.rec("release") }
func (f *fakeInjector) Trusted() bool { return true }

func (f *fakeInjector) take() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.calls
	f.calls = nil
	return c
}

func newInput() (*InputService, *fakeInjector) {
	f := &fakeInjector{}
	displays := []screencap.Display{
		{ID: 1, PointsWidth: 1440, PointsHeight: 900, Primary: true},
		{ID: 2, X: 1440, Y: -100, PointsWidth: 1920, PointsHeight: 1080},
	}
	return NewInputService(f, func() []screencap.Display { return displays }, slog.New(slog.NewTextHandler(io.Discard, nil))), f
}

func abs(seq uint16, display uint8, x, y uint16) []byte {
	return []byte{0x01, byte(seq >> 8), byte(seq), display, byte(x >> 8), byte(x), byte(y >> 8), byte(y)}
}

func TestInputOnlyWithControl(t *testing.T) {
	s, f := newInput()
	s.HandlePointer("P", abs(1, 0, 0, 0))
	s.HandleInput("P", []byte{0x3f})
	if c := f.take(); len(c) != 0 {
		t.Fatalf("a viewer without control moved the host: %v", c)
	}
	s.Allow("P", true)
	s.HandlePointer("Q", abs(1, 0, 0, 0)) // another viewer
	if c := f.take(); len(c) != 0 {
		t.Fatalf("another viewer moved the host: %v", c)
	}
	s.HandlePointer("P", abs(1, 0, 32768, 65535))
	if c := f.take(); !slices.Equal(c, []string{"move 720,900"}) {
		t.Fatalf("calls %v", c)
	}
	// Taking control away lets go of everything.
	s.Allow("P", false)
	if c := f.take(); !slices.Equal(c, []string{"release"}) {
		t.Fatalf("calls %v", c)
	}
}

func TestPointerMovesInOrderAndOnTheirDisplay(t *testing.T) {
	s, f := newInput()
	s.Allow("P", true)
	s.HandlePointer("P", abs(10, 1, 0, 0))
	s.HandlePointer("P", abs(9, 1, 100, 100))  // late
	s.HandlePointer("P", abs(10, 1, 100, 100)) // the same again
	s.HandlePointer("P", abs(11, 1, 65535, 32768))
	s.HandlePointer("P", abs(12, 7, 0, 0))                       // no such display: the main one
	s.HandlePointer("P", []byte{0x02, 0, 13, 0xff, 0xfb, 0, 10}) // relative
	if c := f.take(); !slices.Equal(c, []string{"move 1440,-100", "move 3360,440", "move 0,0", "moveby -5,10"}) {
		t.Fatalf("calls %v", c)
	}
}

func TestButtonsWheelKeysAndText(t *testing.T) {
	s, f := newInput()
	s.Allow("P", true)
	a := keymap.Index("KeyA")
	for _, pkt := range [][]byte{
		{0x10, 0, 1, 0, 0x80, 0, 0x80, 0},          // left down, middle of the main display
		{0x10, 2, 0, 0xff, 0xff, 0xff, 0xff, 0xff}, // right up, where the pointer is
		{0x20, 0, 0, 0xff, 0x88, 0},                // wheel -120 px
		{0x30, byte(a >> 8), byte(a), 1, 0},        // A down
		{0x30, byte(a >> 8), byte(a), 2, 0x11},     // A repeat, Shift and Caps Lock held
		{0x30, 0, 0, 1, 0},                         // keycode 0: unknown
		{0x31, 'h', 'i'},
		{0x3f},
		{0x99}, // garbage
	} {
		s.HandleInput("P", pkt)
	}
	// The key arrives as this system's own (macOS's 0, X11's keysym 'a').
	ka, _ := keymap.Lookup(a)
	nk := ka.Native()
	want := []string{"button 0 true 720,450 true", "button 2 false 0,0 false", "scroll 0,-120 false", fmt.Sprintf("key %d true false 0", nk), fmt.Sprintf("key %d true true 1", nk), "text hi", "release"}
	if c := f.take(); !slices.Equal(c, want) {
		t.Fatalf("calls\n %v\nwant\n %v", c, want)
	}
}

func TestReleaseOnStallAndForget(t *testing.T) {
	s, f := newInput()
	s.Release("P") // not allowed: nothing to release
	s.Allow("P", true)
	s.Release("P")
	s.Forget("P")
	s.Forget("P")
	if c := f.take(); !slices.Equal(c, []string{"release", "release"}) {
		t.Fatalf("calls %v", c)
	}
}
