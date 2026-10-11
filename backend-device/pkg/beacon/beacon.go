// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package beacon shows the latency beacon on the host's screen: a small
// square in the top-left corner of the display being captured, that the
// viewer turns white and black and times until the change reaches its own
// screen. That is the whole path: capture, encoder, network, decoder and
// display. It goes away a few seconds after the last flip.
package beacon

import (
	"sync"
	"time"
)

// Side is the square's side, in points.
const Side = 32

// hideAfter is how long the beacon stays after the last flip.
const hideAfter = 4 * time.Second

var (
	mu    sync.Mutex
	timer *time.Timer
)

// Set shows the beacon on (white) or off (black) at the top-left corner
// x, y of a display, in the desktop's points.
func Set(on bool, x, y int) {
	mu.Lock()
	defer mu.Unlock()
	show(on, float64(x), float64(y), Side)
	if timer != nil {
		timer.Stop()
	}
	timer = time.AfterFunc(hideAfter, hide)
}
