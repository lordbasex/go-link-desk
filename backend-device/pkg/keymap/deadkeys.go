// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package keymap

import "golang.org/x/text/unicode/norm"

// Keysym is the X11 keysym that types r: Latin-1 as it is, the rest by
// X11's Unicode keysyms, newline and tab as their keys.
func Keysym(r rune) int {
	switch {
	case r == '\n':
		return 0xff0d // XK_Return
	case r == '\t':
		return 0xff09 // XK_Tab
	case r >= 0x100:
		return 0x01000000 | int(r)
	}
	return int(r)
}

// deadOf is the dead key (its X11 keysym) of each combining accent.
var deadOf = map[rune]int{
	0x0300: 0xfe50, // grave
	0x0301: 0xfe51, // acute
	0x0302: 0xfe52, // circumflex
	0x0303: 0xfe53, // tilde
	0x0304: 0xfe54, // macron
	0x0306: 0xfe55, // breve
	0x0307: 0xfe56, // dot above
	0x0308: 0xfe57, // diaeresis
	0x030a: 0xfe58, // ring above
	0x030b: 0xfe59, // double acute
	0x030c: 0xfe5a, // caron
	0x0327: 0xfe5b, // cedilla
	0x0328: 0xfe5c, // ogonek
}

// DeadKey splits an accented letter into the dead key and the letter a
// keyboard types it with (á: dead acute, then a); ok is false for other
// characters.
func DeadKey(r rune) (dead int, base rune, ok bool) {
	d := []rune(norm.NFD.String(string(r)))
	if len(d) != 2 {
		return 0, 0, false
	}
	dead, ok = deadOf[d[1]]
	return dead, d[0], ok
}
