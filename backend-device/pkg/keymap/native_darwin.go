// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package keymap

// Native is the key as this system's injector takes it: macOS's virtual
// key code (-1: none).
func (k Key) Native() int { return k.Mac }

// MacOf is a native key as macOS's virtual key code (the test logs speak it
// on every system).
func MacOf(native int) int { return native }
