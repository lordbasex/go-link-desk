// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package viewer

import (
	"math"
	"sync"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/inputproto"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/keymap"
)

// InputSink is where Input sends its packets: the session.
type InputSink interface {
	SendPointer(packet []byte)
	SendInput(packet []byte)
	SendClipboard(text string, paste bool) bool
}

// Input turns the window's mouse and keyboard into the input protocol's
// packets (docs/protocol.md, section 5), as the website does: keys by
// position, nothing left pressed, Cmd+V pasting this computer's clipboard
// there.
type Input struct {
	sink InputSink
	// Clipboard reads this computer's clipboard (for Cmd+V).
	Clipboard func() (string, bool)

	mu        sync.Mutex
	control   bool
	paste     bool // the clipboard permission: Cmd+V pastes this computer's text there
	display   uint8
	seq       uint16
	keys      map[uint16]bool // pressed there
	withMeta  map[uint16]bool // pressed while Cmd was held: macOS may never report their release
	swallowed map[uint16]bool // a press not sent (Cmd+V pasted instead): drop its release too
	buttons   map[int]bool
	wheelX    float64
	wheelY    float64
}

// NewInput sends to sink.
func NewInput(sink InputSink) *Input {
	return &Input{sink: sink, keys: map[uint16]bool{}, withMeta: map[uint16]bool{}, swallowed: map[uint16]bool{}, buttons: map[int]bool{}}
}

// Allow says what the viewer may do: control the mouse and keyboard, and
// paste into the computer. Losing control lets go of everything.
func (in *Input) Allow(control, paste bool) {
	in.mu.Lock()
	was := in.control
	in.control, in.paste = control, paste
	in.mu.Unlock()
	if was && !control {
		in.ReleaseAll()
	}
}

// SetDisplay is the index of the display shown, in the host's list.
func (in *Input) SetDisplay(i int) {
	in.mu.Lock()
	in.display = uint8(i)
	in.mu.Unlock()
}

func scale(v float64) uint16 {
	return uint16(math.Round(math.Min(1, math.Max(0, v)) * 65535))
}

// Move: the pointer is at x, y (0..1 over the picture).
func (in *Input) Move(x, y float64, inside bool) {
	in.mu.Lock()
	if !in.control || !inside {
		in.mu.Unlock()
		return
	}
	in.seq++
	p := inputproto.Packet{Type: inputproto.TypeAbs, Seq: in.seq, Display: in.display, X: scale(x), Y: scale(y)}
	in.mu.Unlock()
	in.sink.SendPointer(inputproto.Encode(p))
}

// Button: a press over the picture, or the release of one (anywhere).
func (in *Input) Button(button int, down bool, x, y float64, inside bool) {
	in.mu.Lock()
	if !in.control || button < 0 || button > inputproto.ButtonForward {
		in.mu.Unlock()
		return
	}
	if down && !inside {
		in.mu.Unlock()
		return
	}
	if !down && !in.buttons[button] {
		in.mu.Unlock()
		return // a release this view never saw pressed
	}
	if down {
		in.buttons[button] = true
	} else {
		delete(in.buttons, button)
	}
	p := inputproto.Packet{Type: inputproto.TypeButton, Button: uint8(button), Down: down, Display: in.display, X: scale(x), Y: scale(y)}
	if !inside {
		p.Display, p.X, p.Y = 0xFF, 0, 0 // act where the pointer is
	}
	in.mu.Unlock()
	in.sink.SendInput(inputproto.Encode(p))
}

// Wheel scrolls: the browser's sign, in points or in lines. Fractions
// add up until they make a whole step.
func (in *Input) Wheel(dx, dy float64, lines bool) {
	in.mu.Lock()
	if !in.control {
		in.mu.Unlock()
		return
	}
	in.wheelX += dx
	in.wheelY += dy
	sx, sy := math.Trunc(in.wheelX), math.Trunc(in.wheelY)
	if sx == 0 && sy == 0 {
		in.mu.Unlock()
		return
	}
	in.wheelX -= sx
	in.wheelY -= sy
	mode := uint8(inputproto.WheelPixels)
	if lines {
		mode = inputproto.WheelLines
		in.wheelX, in.wheelY = 0, 0
	}
	p := inputproto.Packet{Type: inputproto.TypeWheel, DX: clamp16(sx), DY: clamp16(sy), Mode: mode}
	in.mu.Unlock()
	in.sink.SendInput(inputproto.Encode(p))
}

func clamp16(v float64) int16 {
	return int16(math.Max(math.MinInt16, math.Min(math.MaxInt16, v)))
}

var (
	keyV      = keymap.Index("KeyV")
	metaLeft  = keymap.Index("MetaLeft")
	metaRight = keymap.Index("MetaRight")
)

// Key: a macOS virtual key code pressed or released, with the modifiers
// held.
func (in *Input) Key(macCode int, down, repeat bool, mods int) {
	code := keymap.FromMac(macCode)
	if code == 0 {
		return
	}
	in.mu.Lock()
	if !in.control && !in.paste {
		in.mu.Unlock()
		return
	}
	meta := mods&inputproto.ModMeta != 0
	// Cmd+V: this computer's clipboard goes there, and is pasted.
	if down && code == keyV && meta && mods&(inputproto.ModAlt|inputproto.ModShift) == 0 && in.paste && in.Clipboard != nil {
		in.swallowed[code] = true
		control := in.control
		in.mu.Unlock()
		if text, ok := in.Clipboard(); ok && text != "" {
			in.sink.SendClipboard(text, control)
		}
		return
	}
	if !down && in.swallowed[code] {
		delete(in.swallowed, code)
		in.mu.Unlock()
		return
	}
	if !in.control {
		in.mu.Unlock()
		return
	}
	if !down && !in.keys[code] {
		in.mu.Unlock()
		return // a release this view never saw pressed
	}
	var out [][]byte
	if down {
		in.keys[code] = true
		if meta && code != metaLeft && code != metaRight {
			in.withMeta[code] = true
		}
	} else {
		delete(in.keys, code)
		delete(in.withMeta, code)
	}
	out = append(out, inputproto.Encode(inputproto.Packet{Type: inputproto.TypeKey, Keycode: code, Down: down, Repeat: down && repeat, Mods: uint8(mods)}))
	// Cmd goes up: let go of the keys pressed with it, whose release macOS
	// may not report.
	if !down && (code == metaLeft || code == metaRight) {
		for k := range in.withMeta {
			if in.keys[k] {
				delete(in.keys, k)
				out = append(out, inputproto.Encode(inputproto.Packet{Type: inputproto.TypeKey, Keycode: k, Mods: uint8(mods)}))
			}
		}
		clear(in.withMeta)
	}
	in.mu.Unlock()
	for _, b := range out {
		in.sink.SendInput(b)
	}
}

// ReleaseAll lets go of everything pressed there (the window lost the
// focus, the session lost control).
func (in *Input) ReleaseAll() {
	in.mu.Lock()
	had := len(in.keys) > 0 || len(in.buttons) > 0
	clear(in.keys)
	clear(in.withMeta)
	clear(in.swallowed)
	clear(in.buttons)
	in.wheelX, in.wheelY = 0, 0
	control := in.control
	in.mu.Unlock()
	if had || control {
		in.sink.SendInput(inputproto.Encode(inputproto.Packet{Type: inputproto.TypeReleaseAll}))
	}
}

// TypeText types text there, character by character, whatever its
// keyboard layout ("Type the clipboard").
func (in *Input) TypeText(text string) {
	in.mu.Lock()
	control := in.control
	in.mu.Unlock()
	if !control {
		return
	}
	for len(text) > 0 {
		b := inputproto.Encode(inputproto.Packet{Type: inputproto.TypeText, Text: text})
		in.sink.SendInput(b)
		text = text[len(b)-1:]
	}
}

// Combo presses keys in order (by their codes) and lets go of them in
// reverse: shortcuts from the toolbar.
func (in *Input) Combo(codes ...string) {
	in.mu.Lock()
	control := in.control
	in.mu.Unlock()
	if !control {
		return
	}
	bits := map[string]uint8{"ShiftLeft": inputproto.ModShift, "ControlLeft": inputproto.ModCtrl, "AltLeft": inputproto.ModAlt, "MetaLeft": inputproto.ModMeta}
	var mods uint8
	for _, c := range codes {
		in.sink.SendInput(inputproto.Encode(inputproto.Packet{Type: inputproto.TypeKey, Keycode: keymap.Index(c), Down: true, Mods: mods}))
		mods |= bits[c]
	}
	for i := len(codes) - 1; i >= 0; i-- {
		mods &^= bits[codes[i]]
		in.sink.SendInput(inputproto.Encode(inputproto.Packet{Type: inputproto.TypeKey, Keycode: keymap.Index(codes[i]), Mods: mods}))
	}
}
