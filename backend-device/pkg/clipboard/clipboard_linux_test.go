// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux && !headless

package clipboard

import (
	"os"
	"testing"
	"time"
)

// The desktop's clipboard, by hand on a desktop (DESK_CLIPBOARD_TEST=1):
// it copies a text, reads it back, and waits for a copy made elsewhere.
func TestDesktopClipboard(t *testing.T) {
	if os.Getenv("DESK_CLIPBOARD_TEST") == "" {
		t.Skip("set DESK_CLIPBOARD_TEST=1 on a desktop")
	}
	b := System()
	t.Logf("board %T, changes %d", b, b.ChangeCount())
	n := b.SetText("go-link-desk ñ")
	if got, ok := b.Text(); !ok || got != "go-link-desk ñ" {
		t.Fatalf("read back %q %v", got, ok)
	}
	for i := 0; i < 5; i++ {
		time.Sleep(300 * time.Millisecond)
		got, ok := b.Text()
		t.Logf("after %d ms: changes %d, text %q %v", (i+1)*300, b.ChangeCount(), got, ok)
	}
	n = b.ChangeCount()
	t.Logf("set: changes %d; copy something else within 20 s", n)
	for end := time.Now().Add(20 * time.Second); time.Now().Before(end); time.Sleep(200 * time.Millisecond) {
		if b.ChangeCount() != n {
			got, ok := b.Text()
			t.Logf("copied elsewhere: %q %v", got, ok)
			return
		}
	}
	t.Fatal("no copy seen")
}
