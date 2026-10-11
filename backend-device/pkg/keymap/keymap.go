// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package keymap is the keyboard table the host and the website share
// (protocol/keycodes.json): a key's index is the keycode the input
// channel carries, and each entry gives that key on every system.
package keymap

//go:generate go run ./gen

// Key is one physical key, by its position (the browser's
// KeyboardEvent.code), on each system. -1: the system has no such key.
type Key struct {
	Code string
	Mac  int // the macOS virtual key code (kVK_*)
	Win  int // the Windows scan code, 0xE0xx for extended keys
	X11  int // the X11 keysym
}

// Lookup returns the key with a keycode.
func Lookup(keycode uint16) (Key, bool) {
	if int(keycode) == 0 || int(keycode) >= len(Keys) {
		return Key{}, false
	}
	return Keys[keycode], true
}

// Index returns a code's keycode (0: unknown).
func Index(code string) uint16 {
	for i, k := range Keys {
		if k.Code == code {
			return uint16(i)
		}
	}
	return 0
}

// fromMac maps a macOS virtual key code to its keycode.
var fromMac = func() map[int]uint16 {
	m := map[int]uint16{}
	for i, k := range Keys {
		if i > 0 && k.Mac >= 0 {
			if _, dup := m[k.Mac]; !dup {
				m[k.Mac] = uint16(i)
			}
		}
	}
	return m
}()

// FromMac returns the keycode of a macOS virtual key code (0: none).
func FromMac(code int) uint16 { return fromMac[code] }
