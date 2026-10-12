// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package instancelock

import (
	"path/filepath"
	"testing"
	"time"
)

// A second launch wakes the first, which is woken once per call.
func TestWakeReachesTheRunningOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desk.lock")
	if Wake(path) {
		t.Fatal("woke a process that is not there")
	}
	l, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	if !Wake(path) {
		t.Fatal("the running one did not answer")
	}
	select {
	case <-l.Woken():
	case <-time.After(2 * time.Second):
		t.Fatal("not woken")
	}
}
