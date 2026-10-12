// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux

package inject

/*
#cgo LDFLAGS: -lX11 -lXtst
#include <stdlib.h>
#include <X11/Xlib.h>
#include <X11/XKBlib.h>
#include <X11/keysym.h>
#include <X11/extensions/XTest.h>

static Display *inj_dpy;

static int inj_open(void) {
	if (!inj_dpy) {
		inj_dpy = XOpenDisplay(NULL);
		int ev, er, maj, min;
		if (inj_dpy && !XTestQueryExtension(inj_dpy, &ev, &er, &maj, &min)) {
			XCloseDisplay(inj_dpy);
			inj_dpy = NULL;
		}
	}
	return inj_dpy != NULL;
}

static void inj_move(int x, int y) {
	if (!inj_open()) return;
	XTestFakeMotionEvent(inj_dpy, -1, x, y, CurrentTime);
	XFlush(inj_dpy);
}

static void inj_move_by(int dx, int dy) {
	if (!inj_open()) return;
	XTestFakeRelativeMotionEvent(inj_dpy, dx, dy, CurrentTime);
	XFlush(inj_dpy);
}

static void inj_button(int b, int down) {
	if (!inj_open()) return;
	XTestFakeButtonEvent(inj_dpy, b, down, CurrentTime);
	XFlush(inj_dpy);
}

static int inj_keycode(unsigned long sym) {
	if (!inj_open()) return 0;
	return XKeysymToKeycode(inj_dpy, sym);
}

// Whether a keysym needs Shift on its keycode (it is the second level).
static int inj_shifted(int kc, unsigned long sym) {
	if (!inj_open()) return 0;
	return XkbKeycodeToKeysym(inj_dpy, kc, 0, 0) != sym && XkbKeycodeToKeysym(inj_dpy, kc, 0, 1) == sym;
}

static void inj_key(int kc, int down) {
	if (!inj_open() || kc <= 0) return;
	XTestFakeKeyEvent(inj_dpy, kc, down, CurrentTime);
	XFlush(inj_dpy);
}

// A keycode with no keysym, to type characters the layout lacks: it is
// mapped to the character, pressed, and freed again.
static int inj_spare(void) {
	if (!inj_open()) return 0;
	int lo, hi, per;
	XDisplayKeycodes(inj_dpy, &lo, &hi);
	KeySym *map = XGetKeyboardMapping(inj_dpy, lo, hi - lo + 1, &per);
	int spare = 0;
	for (int kc = hi; kc >= lo && !spare; kc--) {
		int empty = 1;
		for (int i = 0; i < per; i++) {
			if (map[(kc - lo) * per + i] != NoSymbol) empty = 0;
		}
		if (empty) spare = kc;
	}
	XFree(map);
	return spare;
}

static void inj_map(int kc, unsigned long sym) {
	if (!inj_open()) return;
	KeySym s[2] = {sym, sym};
	XChangeKeyboardMapping(inj_dpy, kc, 2, s, 1);
	XSync(inj_dpy, False);
}
*/
import "C"

import (
	"sync"
	"time"
)

// Linux drives an X11 desktop's mouse and keyboard with XTest, on a display
// connection of its own. Positions are the root window's pixels. Keys come
// as X11 keysyms (keymap.Key.Native) and go by the keycode the layout gives
// them.

type x11 struct {
	mu      sync.Mutex
	keys    map[int]bool // keycodes held
	buttons map[int]bool // X buttons held
	mods    []int        // modifier keycodes pressed for a key's mods, let go after it
	accX    int          // wheel pixels not yet a click
	accY    int
	spare   int
}

// New returns the X11 injector.
func New() Injector { return &x11{keys: map[int]bool{}, buttons: map[int]bool{}} }

// Supported reports whether this build injects input.
const Supported = true

func xButton(b int) int {
	switch b {
	case Left:
		return 1
	case Middle:
		return 2
	case Right:
		return 3
	case Back:
		return 8
	case Forward:
		return 9
	}
	return 0
}

func (x *x11) Move(px, py float64) {
	x.mu.Lock()
	defer x.mu.Unlock()
	C.inj_move(C.int(px), C.int(py))
}

func (x *x11) MoveBy(dx, dy int) {
	x.mu.Lock()
	defer x.mu.Unlock()
	C.inj_move_by(C.int(dx), C.int(dy))
}

func (x *x11) Button(b int, down bool, px, py float64, at bool) {
	xb := xButton(b)
	if xb == 0 {
		return
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if at {
		C.inj_move(C.int(px), C.int(py))
	}
	if down {
		x.buttons[xb] = true
	} else {
		delete(x.buttons, xb)
	}
	C.inj_button(C.int(xb), boolInt(down))
}

// wheelStep is how many of the browser's pixels make one click of an X
// wheel.
const wheelStep = 50

func (x *x11) Scroll(dx, dy int, lines bool) {
	x.mu.Lock()
	defer x.mu.Unlock()
	clicks := func(acc *int, d int) int {
		if lines {
			return d
		}
		*acc += d
		n := *acc / wheelStep
		*acc -= n * wheelStep
		return n
	}
	// Buttons 4 and 5 scroll up and down; 6 and 7 left and right.
	press := func(n, neg, pos int) {
		b := pos
		if n < 0 {
			b, n = neg, -n
		}
		for i := 0; i < n && i < 20; i++ {
			C.inj_button(C.int(b), 1)
			C.inj_button(C.int(b), 0)
		}
	}
	press(clicks(&x.accY, dy), 4, 5)
	press(clicks(&x.accX, dx), 6, 7)
}

var modKeys = []struct {
	bit uint8
	sym C.ulong
}{
	{ModShift, C.XK_Shift_L}, {ModCtrl, C.XK_Control_L}, {ModAlt, C.XK_Alt_L}, {ModMeta, C.XK_Super_L},
}

func (x *x11) Key(sym int, down, repeat bool, mods uint8) {
	if sym < 0 {
		return
	}
	kc := int(C.inj_keycode(C.ulong(sym)))
	if kc == 0 {
		return
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if down {
		// The modifiers the key comes with, pressed if nobody holds them.
		for _, m := range modKeys {
			if mods&m.bit == 0 {
				continue
			}
			mk := int(C.inj_keycode(m.sym))
			if mk == 0 || mk == kc || x.keys[mk] {
				continue
			}
			C.inj_key(C.int(mk), 1)
			x.mods = append(x.mods, mk)
		}
		x.keys[kc] = true
		C.inj_key(C.int(kc), 1)
		return
	}
	if x.keys[kc] {
		delete(x.keys, kc)
		C.inj_key(C.int(kc), 0)
	}
	for _, mk := range x.mods {
		C.inj_key(C.int(mk), 0)
	}
	x.mods = nil
}

func (x *x11) Text(s string) {
	x.mu.Lock()
	defer x.mu.Unlock()
	shift := int(C.inj_keycode(C.XK_Shift_L))
	// Characters the layout lacks go on a free keycode, mapped to them as
	// they come. Every change of the mapping makes the programs read it
	// again, so it changes only for a new character, with a pause for them,
	// and goes back once at the end.
	var spareSym C.ulong
	defer func() {
		if spareSym != 0 {
			time.Sleep(30 * time.Millisecond)
			C.inj_map(C.int(x.spare), 0)
		}
	}()
	for _, r := range s {
		sym := C.ulong(r)
		switch {
		case r == '\n':
			sym = C.XK_Return
		case r == '\t':
			sym = C.XK_Tab
		case r >= 0x100:
			sym = C.ulong(0x01000000 | r) // X11's Unicode keysyms
		}
		kc := int(C.inj_keycode(sym))
		if kc == 0 || (spareSym != 0 && kc == x.spare) {
			if x.spare == 0 {
				x.spare = int(C.inj_spare())
			}
			if x.spare == 0 {
				continue
			}
			kc = x.spare
			if spareSym != sym {
				C.inj_map(C.int(kc), sym)
				spareSym = sym
				time.Sleep(20 * time.Millisecond)
			}
		}
		shifted := kc != x.spare && C.inj_shifted(C.int(kc), sym) != 0
		if shifted && shift != 0 {
			C.inj_key(C.int(shift), 1)
		}
		C.inj_key(C.int(kc), 1)
		C.inj_key(C.int(kc), 0)
		if shifted && shift != 0 {
			C.inj_key(C.int(shift), 0)
		}
		if kc == x.spare {
			// Read before the mapping can change again.
			time.Sleep(15 * time.Millisecond)
		}
	}
}

func (x *x11) ReleaseAll() {
	x.mu.Lock()
	defer x.mu.Unlock()
	for kc := range x.keys {
		C.inj_key(C.int(kc), 0)
	}
	for _, mk := range x.mods {
		C.inj_key(C.int(mk), 0)
	}
	for b := range x.buttons {
		C.inj_button(C.int(b), 0)
	}
	x.keys, x.buttons, x.mods = map[int]bool{}, map[int]bool{}, nil
}

// Trusted: X11 lets any program of the session inject.
func (x *x11) Trusted() bool {
	x.mu.Lock()
	defer x.mu.Unlock()
	return C.inj_open() != 0
}

func boolInt(b bool) C.int {
	if b {
		return 1
	}
	return 0
}
