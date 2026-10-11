// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/inject"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/keymap"
)

// IdleWatch ends a session after a while without input from its viewer
// (Settings › Disconnect when idle). Seeing the screen is not activity: a
// session left open and forgotten is what this closes.
type IdleWatch struct {
	now   func() time.Time
	every time.Duration
	end   func(reason string)

	mu    sync.Mutex
	limit time.Duration
	peer  string
	last  time.Time
}

// NewIdleWatch builds the watch; end stops the session. now may be nil.
func NewIdleWatch(end func(reason string), now func() time.Time) *IdleWatch {
	if now == nil {
		now = time.Now
	}
	return &IdleWatch{now: now, every: 15 * time.Second, end: end}
}

// SetMinutes sets the time without input that ends a session (0: never).
func (w *IdleWatch) SetMinutes(m int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.limit = time.Duration(m) * time.Minute
}

// Session tells whose session is on ("" when none). A new viewer starts
// with a fresh clock.
func (w *IdleWatch) Session(peer string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if peer != w.peer {
		w.peer, w.last = peer, w.now()
	}
}

// Touch notes input from peer.
func (w *IdleWatch) Touch(peer string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if peer == w.peer {
		w.last = w.now()
	}
}

// check ends the session if it has been idle too long.
func (w *IdleWatch) check() {
	w.mu.Lock()
	idle := w.peer != "" && w.limit > 0 && w.now().Sub(w.last) >= w.limit
	if idle {
		w.peer = ""
	}
	w.mu.Unlock()
	if idle {
		w.end(models.EndIdle)
	}
}

// Run checks until ctx ends.
func (w *IdleWatch) Run(ctx context.Context) {
	t := time.NewTicker(w.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.check()
		}
	}
}

// LockScreen locks the computer's screen as its person would: on macOS,
// Control-Command-Q (it needs the Accessibility permission, like the
// viewer's input). False where there is no way yet.
func LockScreen(inj inject.Injector) bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	k, ok := keymap.Lookup(keymap.Index("KeyQ"))
	if !ok || k.Mac < 0 {
		return false
	}
	mods := uint8(inject.ModCtrl | inject.ModMeta)
	inj.Key(k.Mac, true, false, mods)
	inj.Key(k.Mac, false, false, mods)
	return true
}
