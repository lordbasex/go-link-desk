// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package awake

import (
	"runtime"
	"testing"
)

func TestHoldAndRelease(t *testing.T) {
	Hold()
	Hold()
	// Linux holds only with a desktop session to ask (not in CI).
	if got := Held(); runtime.GOOS != "linux" && got != (runtime.GOOS == "darwin") {
		t.Fatalf("held = %v on %s", got, runtime.GOOS)
	}
	Release()
	Release()
	if Held() {
		t.Fatal("still held")
	}
}
