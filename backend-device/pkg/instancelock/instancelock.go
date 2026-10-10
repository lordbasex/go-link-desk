// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package instancelock makes sure only one device runs per user. It locks
// a file instead of listening on a port, so the device opens no server.
// The operating system releases the lock when the process ends, even if
// it crashes.
package instancelock

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrLocked means another process holds the lock.
var ErrLocked = errors.New("another device is already running")

// Lock is a held lock. Keep it for the life of the process.
type Lock struct{ f *os.File }

// Acquire locks path, creating it when needed.
func Acquire(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		f.Close()
		return nil, err
	}
	return &Lock{f: f}, nil
}

// Release frees the lock.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	err := unlockFile(l.f)
	if cerr := l.f.Close(); err == nil {
		err = cerr
	}
	l.f = nil
	return err
}
