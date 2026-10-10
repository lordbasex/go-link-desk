// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package instancelock

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestOnlyOneHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "device.lock")
	first, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	// flock locks belong to the open file, so a second open in the same
	// process conflicts just like another process would.
	if _, err := Acquire(path); !errors.Is(err, ErrLocked) {
		t.Fatalf("second acquire: %v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	again, err := Acquire(path)
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	again.Release()
}
