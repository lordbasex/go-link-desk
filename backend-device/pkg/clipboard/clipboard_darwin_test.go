// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

package clipboard

import (
	"os"
	"testing"
)

// The real clipboard: only when asked for, since it replaces what the
// person running the tests copied.
func TestSystemClipboardRoundTrip(t *testing.T) {
	if os.Getenv("DESK_TEST_CLIPBOARD") == "" {
		t.Skip("set DESK_TEST_CLIPBOARD=1 to use the real clipboard")
	}
	b := System()
	before := b.ChangeCount()
	n := b.SetText("hola ñandú 日本")
	if n == before || b.ChangeCount() != n {
		t.Fatalf("change count %d → %d (now %d)", before, n, b.ChangeCount())
	}
	if s, ok := b.Text(); !ok || s != "hola ñandú 日本" {
		t.Fatalf("read %q %v", s, ok)
	}
}
