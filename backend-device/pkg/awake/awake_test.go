// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package awake

import (
	"runtime"
	"testing"
)

func TestHoldAndRelease(t *testing.T) {
	Hold()
	Hold()
	if got := Held(); got != (runtime.GOOS == "darwin") {
		t.Fatalf("held = %v on %s", got, runtime.GOOS)
	}
	Release()
	Release()
	if Held() {
		t.Fatal("still held")
	}
}
