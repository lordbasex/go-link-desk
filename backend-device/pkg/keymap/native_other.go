// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !darwin && !linux && !windows

package keymap

// Native is the key as this system's injector takes it (Windows comes in
// phase 7): macOS's code until then.
func (k Key) Native() int { return k.Mac }

// MacOf is a native key as macOS's virtual key code.
func MacOf(native int) int { return native }
