// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build windows && !headless

package clipboard

import "testing"

// The clipboard keeps text in any script, and its count moves; what was
// there before comes back at the end.
func TestTheClipboardKeepsText(t *testing.T) {
	b := System()
	before, had := b.Text()
	defer func() {
		if had {
			b.SetText(before)
		}
	}()
	start := b.ChangeCount()
	want := "go-link Desk ñ á ¿? € 😀 日本\r\nline 2"
	n := b.SetText(want)
	if n == start {
		t.Fatalf("the count did not move: %d", n)
	}
	got, ok := b.Text()
	if !ok || got != want {
		t.Fatalf("got %q %v", got, ok)
	}
}
