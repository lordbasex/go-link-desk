// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !windows

package autostart

import (
	"errors"
	"os"
	"path/filepath"
)

// macOS and Linux keep the entry in a file (entryPath, entry).

// enabled: the entry file exists.
func enabled() bool {
	p, err := entryPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// enable writes the entry file.
func enable(exe string) error {
	p, err := entryPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, entry(exe), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// disable deletes the entry file.
func disable() error {
	p, err := entryPath()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
