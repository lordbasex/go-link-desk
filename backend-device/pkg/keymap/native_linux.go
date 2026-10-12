// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package keymap

// Native is the key as this system's injector takes it: the X11 keysym
// (-1: none).
func (k Key) Native() int { return k.X11 }

var macOfX11 = func() map[int]int {
	m := map[int]int{}
	for _, k := range Keys {
		if k.X11 >= 0 {
			if _, dup := m[k.X11]; !dup {
				m[k.X11] = k.Mac
			}
		}
	}
	return m
}()

// MacOf is a native key as macOS's virtual key code (the test logs speak it
// on every system); -1 when macOS has no such key.
func MacOf(native int) int {
	if v, ok := macOfX11[native]; ok {
		return v
	}
	return -1
}
