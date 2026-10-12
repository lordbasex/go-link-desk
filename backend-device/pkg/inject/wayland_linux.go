// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux && !headless

package inject

import (
	"strconv"
	"sync"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/keymap"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/portal"
)

// A Wayland desktop takes the mouse and keyboard through its remote
// desktop portal, in the session the screen capture started (nothing
// moves before it). Keys go by position (evdev codes), so the desktop's
// layout applies as with a real keyboard. Typed text goes by the keys of
// the desktop's layout (read through XWayland), each with the Shift or
// AltGr its level takes: the portals' own keysym lookups miss levels. An
// accented letter the layout lacks goes by its dead key (á: ´ then a), and
// anything else through the Unicode entry of an input method (IBus,
// Fcitx: Ctrl+Shift+U, the code, space), when the desktop runs one.

type wayland struct {
	mu      sync.Mutex
	keys    map[int32]bool // evdev codes held
	syms    map[int32]bool // keysyms held (keys with no evdev code)
	buttons map[int32]bool // evdev buttons held
	mods    []int32        // modifier codes pressed for a key's mods
	accX    int
	accY    int
}

func newWayland() Injector {
	return &wayland{keys: map[int32]bool{}, syms: map[int32]bool{}, buttons: map[int32]bool{}}
}

// Linux's evdev button and modifier codes.
const (
	btnLeft   = 0x110
	btnRight  = 0x111
	btnMiddle = 0x112
	btnSide   = 0x113
	btnExtra  = 0x114
	keyShiftL = 42
	keyCtrlL  = 29
	keyAltL   = 56
	keyMetaL  = 125
	keyAltGr  = 100 // KEY_RIGHTALT
	keyU      = 22
	keySpace  = 57
	axisVert  = 0
	axisHoriz = 1
	maxClicks = 20
)

func evdevButton(b int) int32 {
	switch b {
	case Left:
		return btnLeft
	case Middle:
		return btnMiddle
	case Right:
		return btnRight
	case Back:
		return btnSide
	case Forward:
		return btnExtra
	}
	return 0
}

func (w *wayland) Move(x, y float64) {
	if s := portal.Active(); s != nil {
		s.MoveTo(x, y)
	}
}

func (w *wayland) MoveBy(dx, dy int) {
	if s := portal.Active(); s != nil {
		s.MoveBy(float64(dx), float64(dy))
	}
}

func (w *wayland) Button(b int, down bool, x, y float64, at bool) {
	code := evdevButton(b)
	s := portal.Active()
	if code == 0 || s == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if at {
		s.MoveTo(x, y)
	}
	if down {
		w.buttons[code] = true
	} else {
		delete(w.buttons, code)
	}
	s.Button(code, down)
}

func (w *wayland) Scroll(dx, dy int, lines bool) {
	s := portal.Active()
	if s == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	clicks := func(acc *int, d int) int32 {
		if lines {
			return int32(d)
		}
		*acc += d
		n := *acc / wheelStep
		*acc -= n * wheelStep
		return int32(max(min(n, maxClicks), -maxClicks))
	}
	if n := clicks(&w.accY, dy); n != 0 {
		s.Scroll(axisVert, n)
	}
	if n := clicks(&w.accX, dx); n != 0 {
		s.Scroll(axisHoriz, n)
	}
}

var waylandMods = []struct {
	bit  uint8
	code int32
}{
	{ModShift, keyShiftL}, {ModCtrl, keyCtrlL}, {ModAlt, keyAltL}, {ModMeta, keyMetaL},
}

func (w *wayland) Key(sym int, down, repeat bool, mods uint8) {
	s := portal.Active()
	if sym < 0 || s == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	code := int32(keymap.EvdevOfX11(sym))
	if code < 0 {
		// No such key on a US keyboard: by its symbol.
		ks := int32(sym)
		if down {
			if w.syms[ks] {
				return // the desktop repeats held keys itself
			}
			w.pressMods(s, mods, -1)
			w.syms[ks] = true
			s.Keysym(ks, true)
			return
		}
		if w.syms[ks] {
			delete(w.syms, ks)
			s.Keysym(ks, false)
		}
		w.releaseMods(s)
		return
	}
	if down {
		if w.keys[code] {
			return
		}
		w.pressMods(s, mods, code)
		w.keys[code] = true
		s.Keycode(code, true)
		return
	}
	if w.keys[code] {
		delete(w.keys, code)
		s.Keycode(code, false)
	}
	w.releaseMods(s)
}

// pressMods presses the modifiers a key comes with, those nobody holds.
func (w *wayland) pressMods(s *portal.Session, mods uint8, key int32) {
	for _, m := range waylandMods {
		if mods&m.bit == 0 || m.code == key || w.keys[m.code] {
			continue
		}
		s.Keycode(m.code, true)
		w.mods = append(w.mods, m.code)
	}
}

func (w *wayland) releaseMods(s *portal.Session) {
	for _, c := range w.mods {
		s.Keycode(c, false)
	}
	w.mods = nil
}

func (w *wayland) Text(text string) {
	s := portal.Active()
	if s == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if !layoutKeys() {
		// No layout to read: the desktop finds the keys.
		for _, r := range text {
			sym := int32(keymap.Keysym(r))
			s.Keysym(sym, true)
			s.Keysym(sym, false)
		}
		return
	}
	for _, r := range text {
		if w.typeSym(s, keymap.Keysym(r)) {
			continue
		}
		// Not on the layout: an accented letter by its dead key, as a
		// person types it (á: ´ then a), else by its code point.
		if dead, base, ok := keymap.DeadKey(r); ok {
			_, _, hasDead := layoutKey(dead)
			_, _, hasBase := layoutKey(keymap.Keysym(base))
			if hasDead && hasBase {
				// Composed apart from the keys around it: a pause each side
				// keeps the order.
				s.Pause(imePause)
				w.typeSym(s, dead)
				w.typeSym(s, keymap.Keysym(base))
				s.Pause(imePause)
				continue
			}
		}
		if portal.InputMethod() {
			w.unicode(s, r)
		}
		// Else the layout cannot type it at all: left out.
	}
}

// typeSym presses and releases the key that types sym, with the Shift and
// AltGr its level takes; false when the layout has no such key.
func (w *wayland) typeSym(s *portal.Session, sym int) bool {
	code, level, ok := layoutKey(sym)
	if !ok {
		return false
	}
	var mods []int32
	if level&1 != 0 {
		mods = append(mods, keyShiftL)
	}
	if level&2 != 0 {
		mods = append(mods, keyAltGr)
	}
	for _, m := range mods {
		s.Keycode(m, true)
	}
	s.Keycode(int32(code), true)
	s.Keycode(int32(code), false)
	for i := len(mods) - 1; i >= 0; i-- {
		s.Keycode(mods[i], false)
	}
	return true
}

// unicode types r by its code point: Ctrl+Shift+U, the hex digits, space.
// The input method reads them at its own pace: keys typed right after
// would overtake the character.
func (w *wayland) unicode(s *portal.Session, r rune) {
	for _, c := range []int32{keyCtrlL, keyShiftL, keyU} {
		s.Keycode(c, true)
	}
	for _, c := range []int32{keyU, keyShiftL, keyCtrlL} {
		s.Keycode(c, false)
	}
	s.Pause(imePause)
	for _, d := range strconv.FormatInt(int64(r), 16) {
		w.typeSym(s, int(d))
	}
	s.Pause(imePause)
	s.Keycode(keySpace, true)
	s.Keycode(keySpace, false)
	s.Pause(imePause)
}

// imePause is how long the input method gets at each step of a Unicode
// entry.
const imePause = 40 * time.Millisecond

func (w *wayland) ReleaseAll() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if s := portal.Active(); s != nil {
		for c := range w.keys {
			s.Keycode(c, false)
		}
		for ks := range w.syms {
			s.Keysym(ks, false)
		}
		w.releaseMods(s)
		for b := range w.buttons {
			s.Button(b, false)
		}
	}
	w.keys, w.syms, w.buttons, w.mods = map[int32]bool{}, map[int32]bool{}, map[int32]bool{}, nil
}

// Trusted: the desktop has the portal (it asks the person when the
// session starts).
func (w *wayland) Trusted() bool { return portal.Available() }
