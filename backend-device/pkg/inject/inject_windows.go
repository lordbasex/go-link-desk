// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build windows && !headless

package inject

import (
	"math"
	"sync"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows drives the mouse and keyboard with SendInput. Positions are the
// virtual screen's pixels (the process is per-monitor DPI aware, see
// pkg/capture), the primary monitor's top-left at 0, 0. Keys come as scan
// codes (keymap.Key.Native, 0xE0xx for extended keys), so the computer's
// own layout makes the characters; typed text goes as Unicode characters,
// whatever the layout. Windows lets no ordinary program drive a window
// running as administrator, nor the secure desktop (the lock screen, a UAC
// prompt): input there is dropped.

var (
	user32            = windows.NewLazySystemDLL("user32.dll")
	procSendInput     = user32.NewProc("SendInput")
	procGetMetrics    = user32.NewProc("GetSystemMetrics")
	procMapVirtualKey = user32.NewProc("MapVirtualKeyW")
)

// INPUT, as SendInput takes it: a type and a union of the mouse's and the
// keyboard's records (the mouse's is the larger), which starts at the
// alignment of a pointer.
type mouseInput struct {
	typ  uint32
	_    [unionPad]byte
	dx   int32
	dy   int32
	data uint32
	flag uint32
	time uint32
	info uintptr
}

type keyInput struct {
	typ  uint32
	_    [unionPad]byte
	vk   uint16
	scan uint16
	flag uint32
	time uint32
	info uintptr
	_    [8]byte // the rest of the union
}

const unionPad = unsafe.Sizeof(uintptr(0)) - 4

// Both records are one INPUT in size.
var (
	_ [unsafe.Sizeof(mouseInput{}) - unsafe.Sizeof(keyInput{})]struct{}
	_ [unsafe.Sizeof(keyInput{}) - unsafe.Sizeof(mouseInput{})]struct{}
)

const (
	inputMouse    = 0
	inputKeyboard = 1

	mouseMove       = 0x0001
	mouseLeftDown   = 0x0002
	mouseLeftUp     = 0x0004
	mouseRightDown  = 0x0008
	mouseRightUp    = 0x0010
	mouseMiddleDown = 0x0020
	mouseMiddleUp   = 0x0040
	mouseXDown      = 0x0080
	mouseXUp        = 0x0100
	mouseWheel      = 0x0800
	mouseHWheel     = 0x1000
	mouseVirtual    = 0x4000
	mouseAbsolute   = 0x8000

	keyExtended = 0x0001
	keyUp       = 0x0002
	keyUnicode  = 0x0004
	keyScanCode = 0x0008

	wheelDelta = 120

	smXVirtual  = 76
	smYVirtual  = 77
	smCXVirtual = 78
	smCYVirtual = 79

	vkPause    = 0x13
	vkSnapshot = 0x2C
)

func sendMouse(in ...mouseInput) {
	for i := range in {
		in[i].typ = inputMouse
	}
	procSendInput.Call(uintptr(len(in)), uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
}

func sendKeys(in ...keyInput) {
	if len(in) == 0 {
		return
	}
	for i := range in {
		in[i].typ = inputKeyboard
	}
	procSendInput.Call(uintptr(len(in)), uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
}

func metric(i int) int {
	r, _, _ := procGetMetrics.Call(uintptr(i))
	return int(int32(r))
}

// absolute is x, y as SendInput's absolute coordinates: 0 to 65535 across
// the whole virtual screen, rounded up so they land on the pixel meant.
func absolute(x, y float64) (int32, int32) {
	vx, vy := float64(metric(smXVirtual)), float64(metric(smYVirtual))
	vw, vh := float64(metric(smCXVirtual)), float64(metric(smCYVirtual))
	if vw <= 0 || vh <= 0 {
		return 0, 0
	}
	return normalize(x, vx, vw), normalize(y, vy, vh)
}

// normalize is a pixel p on an axis of size pixels from origin as 0 to
// 65535; Windows takes it back to the pixel as n * size / 65536, rounded
// down.
func normalize(p, origin, size float64) int32 {
	v := math.Ceil((math.Floor(p) - origin) * 65536 / size)
	return int32(math.Max(0, math.Min(65535, v)))
}

// key is a scan code (0xE0xx: extended) as a key record. Pause and Print
// Screen are no single scan code: they go by their virtual key.
func key(scan int, up bool) keyInput {
	k := keyInput{}
	if up {
		k.flag |= keyUp
	}
	switch scan {
	case 0xE11D, 0xE045: // Pause
		k.vk = vkPause
	case 0xE037: // Print Screen
		k.vk = vkSnapshot
		k.flag |= keyExtended
	default:
		k.scan = uint16(scan & 0xFF)
		k.flag |= keyScanCode
		if scan&0xFF00 == 0xE000 {
			k.flag |= keyExtended
		}
		return k
	}
	r, _, _ := procMapVirtualKey.Call(uintptr(k.vk), 0) // MAPVK_VK_TO_VSC
	k.scan = uint16(r)
	return k
}

type win struct {
	mu      sync.Mutex
	keys    map[int]bool // scan codes held
	buttons map[int]bool // the protocol's buttons held
	mods    []int        // modifier scan codes pressed for a key's mods, let go after it
}

// New returns the injector of the desktop.
func New() Injector { return &win{keys: map[int]bool{}, buttons: map[int]bool{}} }

// Supported reports whether this build injects input.
const Supported = true

func (w *win) Move(x, y float64) {
	nx, ny := absolute(x, y)
	w.mu.Lock()
	defer w.mu.Unlock()
	sendMouse(mouseInput{dx: nx, dy: ny, flag: mouseMove | mouseAbsolute | mouseVirtual})
}

func (w *win) MoveBy(dx, dy int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	sendMouse(mouseInput{dx: int32(dx), dy: int32(dy), flag: mouseMove})
}

// buttonFlags are a button's down and up flags and its mouse data (which
// X button).
func buttonFlags(b int) (down, up, data uint32) {
	switch b {
	case Left:
		return mouseLeftDown, mouseLeftUp, 0
	case Middle:
		return mouseMiddleDown, mouseMiddleUp, 0
	case Right:
		return mouseRightDown, mouseRightUp, 0
	case Back:
		return mouseXDown, mouseXUp, 1 // XBUTTON1
	case Forward:
		return mouseXDown, mouseXUp, 2 // XBUTTON2
	}
	return 0, 0, 0
}

func (w *win) Button(b int, down bool, x, y float64, at bool) {
	d, u, data := buttonFlags(b)
	if d == 0 {
		return
	}
	in := mouseInput{data: data, flag: u}
	if down {
		in.flag = d
	}
	if at {
		in.dx, in.dy = absolute(x, y)
		in.flag |= mouseMove | mouseAbsolute | mouseVirtual
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if down {
		w.buttons[b] = true
	} else {
		delete(w.buttons, b)
	}
	sendMouse(in)
}

// wheelStep is how many of the browser's pixels make one notch of a
// wheel; smaller steps go as fractions of a notch, as precision touchpads
// send them.
const wheelStep = 50

func (w *win) Scroll(dx, dy int, lines bool) {
	scale := func(d int) int32 {
		if lines {
			return int32(d * wheelDelta)
		}
		return int32(d * wheelDelta / wheelStep)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	// The wheel's positive turns scroll the content up; the browser's
	// positive dy scrolls it down. Horizontal turns follow the browser.
	if v := scale(dy); v != 0 {
		sendMouse(mouseInput{data: uint32(-v), flag: mouseWheel})
	}
	if v := scale(dx); v != 0 {
		sendMouse(mouseInput{data: uint32(v), flag: mouseHWheel})
	}
}

var modKeys = []struct {
	bit  uint8
	scan int
}{
	{ModShift, 0x2A}, {ModCtrl, 0x1D}, {ModAlt, 0x38}, {ModMeta, 0xE05B},
}

func (w *win) Key(scan int, down, repeat bool, mods uint8) {
	if scan <= 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if down {
		// The modifiers the key comes with, pressed if nobody holds them.
		var in []keyInput
		for _, m := range modKeys {
			if mods&m.bit == 0 || m.scan == scan || w.keys[m.scan] {
				continue
			}
			in = append(in, key(m.scan, false))
			w.mods = append(w.mods, m.scan)
		}
		w.keys[scan] = true
		sendKeys(append(in, key(scan, false))...)
		return
	}
	var in []keyInput
	if w.keys[scan] {
		delete(w.keys, scan)
		in = append(in, key(scan, true))
	}
	for _, m := range w.mods {
		in = append(in, key(m, true))
	}
	w.mods = nil
	sendKeys(in...)
}

func (w *win) Text(s string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, r := range s {
		switch r {
		case '\n':
			sendKeys(key(0x1C, false), key(0x1C, true)) // Enter
			continue
		case '\t':
			sendKeys(key(0x0F, false), key(0x0F, true)) // Tab
			continue
		case '\r':
			continue
		}
		// Characters outside the basic plane go as their two halves.
		var in []keyInput
		for _, u := range utf16.Encode([]rune{r}) {
			in = append(in, keyInput{scan: u, flag: keyUnicode}, keyInput{scan: u, flag: keyUnicode | keyUp})
		}
		sendKeys(in...)
	}
}

func (w *win) ReleaseAll() {
	w.mu.Lock()
	defer w.mu.Unlock()
	var keys []keyInput
	for scan := range w.keys {
		keys = append(keys, key(scan, true))
	}
	for _, m := range w.mods {
		keys = append(keys, key(m, true))
	}
	sendKeys(keys...)
	for b := range w.buttons {
		_, u, data := buttonFlags(b)
		sendMouse(mouseInput{data: data, flag: u})
	}
	w.keys, w.buttons, w.mods = map[int]bool{}, map[int]bool{}, nil
}

// Trusted: Windows lets any program of the person at the computer drive
// their mouse and keyboard.
func (w *win) Trusted() bool { return true }
