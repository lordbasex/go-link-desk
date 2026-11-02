// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux && headless && lordos

package inject

import (
	"sync"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/keymap"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/lordshare"
)

// Supported reports whether this build injects input.
const Supported = true

// New returns the injector over LordOS's screen-sharing API. What the
// system refuses (a surface of its own or a protected window in the way) it
// drops by itself. The API has three buttons, so back and forward do
// nothing here.
func New() Injector {
	return &injector{share: func() sharer {
		if c := lordshare.Shared(); c != nil {
			return c
		}
		return nil
	}, keys: map[int]bool{}, btns: map[int]bool{}, synth: map[int]bool{}}
}

// sharer is the part of lordshare.Client this uses.
type sharer interface {
	Move(x, y int)
	Button(b int, down bool)
	Wheel(steps int)
	Scroll(phase int, dx, dy float64)
	Key(code, value int)
	Text(s string)
	State() (lordshare.State, bool)
}

// scrollEnd is how long without a scroll ends a gesture.
const scrollEnd = 150 * time.Millisecond

var modKeys = []struct {
	bit  uint8
	code int
}{{ModShift, 42}, {ModCtrl, 29}, {ModAlt, 56}, {ModMeta, 125}}

type injector struct {
	share func() sharer

	mu     sync.Mutex
	x, y   int
	keys   map[int]bool // evdev codes held
	btns   map[int]bool
	synth  map[int]bool // modifiers pressed on a key's behalf
	scroll *time.Timer  // running while a fine scroll lasts
}

func (i *injector) Move(x, y float64) {
	i.mu.Lock()
	i.x, i.y = int(x), int(y)
	i.mu.Unlock()
	if s := i.share(); s != nil {
		s.Move(int(x), int(y))
	}
}

func (i *injector) MoveBy(dx, dy int) {
	i.mu.Lock()
	i.x, i.y = max(i.x+dx, 0), max(i.y+dy, 0)
	x, y := i.x, i.y
	i.mu.Unlock()
	if s := i.share(); s != nil {
		s.Move(x, y)
	}
}

func (i *injector) Button(b int, down bool, x, y float64, at bool) {
	s := i.share()
	if s == nil || b < Left || b > Right {
		return
	}
	if at {
		i.Move(x, y)
	}
	i.mu.Lock()
	if down {
		i.btns[b] = true
	} else {
		delete(i.btns, b)
	}
	i.mu.Unlock()
	s.Button(b, down)
}

// Scroll: lines are the wheel, whose positive steps go up; pixels are a
// gesture, with the browser's own signs, as the session's page sends it.
func (i *injector) Scroll(dx, dy int, lines bool) {
	s := i.share()
	if s == nil {
		return
	}
	if lines {
		s.Wheel(-dy)
		return
	}
	i.mu.Lock()
	begin := i.scroll == nil
	if begin {
		i.scroll = time.AfterFunc(scrollEnd, i.endScroll)
	} else {
		i.scroll.Reset(scrollEnd)
	}
	i.mu.Unlock()
	if begin {
		s.Scroll(1, 0, 0)
	}
	s.Scroll(2, float64(dx), float64(dy))
}

func (i *injector) endScroll() {
	i.mu.Lock()
	i.scroll = nil
	i.mu.Unlock()
	if s := i.share(); s != nil {
		s.Scroll(3, 0, 0)
	}
}

// Key takes the X11 keysym the keyboard table gives on Linux. Modifiers held
// on the viewer's keyboard that never arrived as keys (held before the page
// had the focus) are pressed around the key.
func (i *injector) Key(sym int, down, repeat bool, mods uint8) {
	s := i.share()
	code := keymap.EvdevOfX11(sym)
	if s == nil || code <= 0 {
		return
	}
	i.mu.Lock()
	var press, free []int
	if down && !repeat {
		for _, m := range modKeys {
			if mods&m.bit != 0 && !i.keys[m.code] && m.code != code {
				press = append(press, m.code)
				i.keys[m.code], i.synth[m.code] = true, true
			}
		}
		i.keys[code] = true
	} else if !down {
		delete(i.keys, code)
		for m := range i.synth {
			free = append(free, m)
			delete(i.keys, m)
			delete(i.synth, m)
		}
	}
	i.mu.Unlock()
	for _, m := range press {
		s.Key(m, 1)
	}
	switch {
	case !down:
		s.Key(code, 0)
	case repeat:
		s.Key(code, 2)
	default:
		s.Key(code, 1)
	}
	for _, m := range free {
		s.Key(m, 0)
	}
}

func (i *injector) Text(t string) {
	if s := i.share(); s != nil {
		s.Text(t)
	}
}

func (i *injector) ReleaseAll() {
	s := i.share()
	i.mu.Lock()
	keys, btns := i.keys, i.btns
	i.keys, i.btns, i.synth = map[int]bool{}, map[int]bool{}, map[int]bool{}
	i.mu.Unlock()
	if s == nil {
		return
	}
	for c := range keys {
		s.Key(c, 0)
	}
	for b := range btns {
		s.Button(b, false)
	}
}

// Trusted answers from the system's remote-input permission.
func (i *injector) Trusted() bool {
	s := i.share()
	if s == nil {
		return false
	}
	st, ok := s.State()
	return ok && st.Granted&lordshare.FlagInput != 0
}
