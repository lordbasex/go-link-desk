// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

package inject

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=13.0
#cgo LDFLAGS: -framework AppKit -framework ApplicationServices -framework Carbon
#include <stdint.h>
#include <stdlib.h>
typedef unsigned short UniChar;
void inj_mouse(int type, double x, double y, int button, int clicks, uint64_t flags);
void inj_mouse_rel(int type, double x, double y, int dx, int dy, int button, uint64_t flags);
void inj_scroll(int dx, int dy, int lines, uint64_t flags);
void inj_key(int vk, int down, int repeat, uint64_t flags);
void inj_text(const UniChar *s, int n);
void inj_position(double *x, double *y);
double inj_double_click_interval(void);
int inj_trusted(void);
*/
import "C"

import (
	"math"
	"sync"
	"time"
	"unicode/utf16"
	"unsafe"
)

// CGEventType values.
const (
	evLeftDown     = 1
	evLeftUp       = 2
	evRightDown    = 3
	evRightUp      = 4
	evMoved        = 5
	evLeftDragged  = 6
	evRightDragged = 7
	evOtherDown    = 25
	evOtherUp      = 26
	evOtherDragged = 27
)

// Modifier keys (kVK_*) and their CGEventFlags.
var modifierFlags = map[int]uint64{
	0x38: 1 << 17, 0x3C: 1 << 17, // shift
	0x3B: 1 << 18, 0x3E: 1 << 18, // control
	0x3A: 1 << 19, 0x3D: 1 << 19, // option
	0x37: 1 << 20, 0x36: 1 << 20, // command
	0x3F: 1 << 23, // fn
}

// mac is the CGEvent injector.
type mac struct {
	mu      sync.Mutex
	keys    map[int]bool // held down
	buttons map[int]bool
	x, y    float64 // where the pointer was put
	// the last press, for double and triple clicks
	lastButton int
	lastDown   time.Time
	lastX      float64
	lastY      float64
	clicks     int
}

// New returns the system's injector.
func New() Injector {
	m := &mac{keys: map[int]bool{}, buttons: map[int]bool{}}
	var x, y C.double
	C.inj_position(&x, &y)
	m.x, m.y = float64(x), float64(y)
	return m
}

// modsFlags maps a key's modifier bits to CGEventFlags.
var modsFlags = []struct {
	bit  uint8
	flag uint64
}{{ModShift, 1 << 17}, {ModCtrl, 1 << 18}, {ModAlt, 1 << 19}, {ModMeta, 1 << 20}}

func (m *mac) flags() uint64 {
	var f uint64
	for k := range m.keys {
		f |= modifierFlags[k]
	}
	return f
}

// cgButton maps the protocol's buttons to CGEvent's: 0 left, 1 right,
// 2 middle, 3 back, 4 forward.
func cgButton(b int) int {
	switch b {
	case Left:
		return 0
	case Right:
		return 1
	case Middle:
		return 2
	default:
		return b
	}
}

// moveType is a move's event type: dragged while a button is held.
func (m *mac) moveType() (int, int) {
	switch {
	case m.buttons[Left]:
		return evLeftDragged, 0
	case m.buttons[Right]:
		return evRightDragged, 1
	}
	for b := range m.buttons {
		return evOtherDragged, cgButton(b)
	}
	return evMoved, 0
}

func (m *mac) Move(x, y float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.x, m.y = x, y
	t, b := m.moveType()
	C.inj_mouse(C.int(t), C.double(x), C.double(y), C.int(b), 0, C.uint64_t(m.flags()))
}

func (m *mac) MoveBy(dx, dy int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var x, y C.double
	C.inj_position(&x, &y)
	m.x, m.y = float64(x)+float64(dx), float64(y)+float64(dy)
	t, b := m.moveType()
	C.inj_mouse_rel(C.int(t), C.double(m.x), C.double(m.y), C.int(dx), C.int(dy), C.int(b), C.uint64_t(m.flags()))
}

func (m *mac) Button(button int, down bool, x, y float64, at bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !at {
		var cx, cy C.double
		C.inj_position(&cx, &cy)
		x, y = float64(cx), float64(cy)
	}
	if down == m.buttons[button] {
		return // already so: a repeated packet
	}
	m.x, m.y = x, y
	if down {
		// A press soon after another of the same button, nearby, counts
		// as a double (or triple) click.
		interval := time.Duration(float64(C.inj_double_click_interval()) * float64(time.Second))
		if button == m.lastButton && time.Since(m.lastDown) <= interval && math.Abs(x-m.lastX) <= 4 && math.Abs(y-m.lastY) <= 4 {
			m.clicks++
		} else {
			m.clicks = 1
		}
		m.lastButton, m.lastDown, m.lastX, m.lastY = button, time.Now(), x, y
		m.buttons[button] = true
	} else {
		delete(m.buttons, button)
	}
	var t int
	switch button {
	case Left:
		t = evLeftUp
		if down {
			t = evLeftDown
		}
	case Right:
		t = evRightUp
		if down {
			t = evRightDown
		}
	default:
		t = evOtherUp
		if down {
			t = evOtherDown
		}
	}
	C.inj_mouse(C.int(t), C.double(x), C.double(y), C.int(cgButton(button)), C.int(m.clicks), C.uint64_t(m.flags()))
}

func (m *mac) Scroll(dx, dy int, lines bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l := 0
	if lines {
		l = 1
	}
	// The browser's positive dy scrolls the content down; a scroll event's
	// positive wheel scrolls it up.
	C.inj_scroll(C.int(-dx), C.int(-dy), C.int(l), C.uint64_t(m.flags()))
}

func (m *mac) Key(vk int, down, repeat bool, mods uint8) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if vk < 0 || vk > 0x7F {
		return
	}
	if down {
		m.keys[vk] = true
	} else {
		delete(m.keys, vk)
	}
	r := 0
	if repeat && down {
		r = 1
	}
	f := m.flags()
	for _, mf := range modsFlags {
		if mods&mf.bit != 0 {
			f |= mf.flag
		}
	}
	C.inj_key(C.int(vk), cbool(down), C.int(r), C.uint64_t(f))
}

func (m *mac) Text(s string) {
	u := utf16.Encode([]rune(s))
	if len(u) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	C.inj_text((*C.UniChar)(unsafe.Pointer(&u[0])), C.int(len(u)))
}

func (m *mac) ReleaseAll() {
	m.mu.Lock()
	keys := make([]int, 0, len(m.keys))
	for k := range m.keys {
		keys = append(keys, k)
	}
	buttons := make([]int, 0, len(m.buttons))
	for b := range m.buttons {
		buttons = append(buttons, b)
	}
	x, y := m.x, m.y
	m.mu.Unlock()
	for _, b := range buttons {
		m.Button(b, false, x, y, false)
	}
	// Ordinary keys first, modifiers last, so nothing typed with a
	// modifier still held comes out as a shortcut.
	for _, k := range keys {
		if modifierFlags[k] == 0 {
			m.Key(k, false, false, 0)
		}
	}
	for _, k := range keys {
		if modifierFlags[k] != 0 {
			m.Key(k, false, false, 0)
		}
	}
}

func (m *mac) Trusted() bool { return C.inj_trusted() != 0 }

func cbool(v bool) C.int {
	if v {
		return 1
	}
	return 0
}

// Supported reports whether this build injects input.
const Supported = true
