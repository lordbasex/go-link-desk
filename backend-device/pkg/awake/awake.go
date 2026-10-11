// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package awake keeps the computer's display from going to sleep while a
// session is on: a viewer looking at the screen is not the person at the
// computer, and the system only counts the latter as activity.
package awake

import "sync"

var (
	mu   sync.Mutex
	held bool
)

// Hold keeps the display awake until Release. Holding twice is holding
// once.
func Hold() {
	mu.Lock()
	defer mu.Unlock()
	if held {
		return
	}
	held = hold()
}

// Release lets the display sleep again.
func Release() {
	mu.Lock()
	defer mu.Unlock()
	if !held {
		return
	}
	release()
	held = false
}

// Held reports whether the display is kept awake.
func Held() bool {
	mu.Lock()
	defer mu.Unlock()
	return held
}
