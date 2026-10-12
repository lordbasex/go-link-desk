// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package keymap

// Native is the key as this system's injector takes it: the Windows scan
// code, 0xE0xx for extended keys (-1: none).
func (k Key) Native() int { return k.Win }

var macOfWin = func() map[int]int {
	m := map[int]int{}
	for _, k := range Keys {
		if k.Win >= 0 {
			if _, dup := m[k.Win]; !dup {
				m[k.Win] = k.Mac
			}
		}
	}
	return m
}()

// MacOf is a native key as macOS's virtual key code (the test logs speak it
// on every system); -1 when macOS has no such key.
func MacOf(native int) int {
	if v, ok := macOfWin[native]; ok {
		return v
	}
	return -1
}
