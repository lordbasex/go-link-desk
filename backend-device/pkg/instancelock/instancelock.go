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
type Lock struct {
	f     *os.File
	woken chan struct{}
	stop  func()
}

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
	l := &Lock{f: f, woken: make(chan struct{}, 1)}
	l.stop = listen(path, l.woken)
	return l, nil
}

// Woken receives a value each time another process calls Wake for this
// lock's path: someone opened the app again, and the running one should
// show itself. It never receives where Wake is not supported.
func (l *Lock) Woken() <-chan struct{} { return l.woken }

// Release frees the lock.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	if l.stop != nil {
		l.stop()
		l.stop = nil
	}
	err := unlockFile(l.f)
	if cerr := l.f.Close(); err == nil {
		err = cerr
	}
	l.f = nil
	return err
}
