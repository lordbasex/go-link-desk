// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux && headless && lordos

package clipboard

import (
	"sync"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/lordshare"
)

// Supported reports whether this build shares the clipboard.
const Supported = true

// System is the session's clipboard through the API, which only passes
// what is copied during the share and never what is marked sensitive.
func System() Board { return &board{} }

type board struct {
	mu  sync.Mutex
	own int // this side's writes, which also move ChangeCount on
}

func (b *board) ChangeCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	gen := 0
	if c := lordshare.Shared(); c != nil {
		_, gen = c.Clipboard()
	}
	return gen + b.own
}

func (b *board) Text() (string, bool) {
	c := lordshare.Shared()
	if c == nil {
		return "", false
	}
	s, gen := c.Clipboard()
	return s, gen > 0
}

func (b *board) SetText(s string) int {
	if c := lordshare.Shared(); c != nil {
		c.SetClipboard(s)
	}
	b.mu.Lock()
	b.own++
	b.mu.Unlock()
	return b.ChangeCount()
}
