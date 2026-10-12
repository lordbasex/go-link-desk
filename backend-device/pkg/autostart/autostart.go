// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package autostart starts the host when the person logs in. It only
// writes one entry pointing at the binary, where the system looks for
// them: a LaunchAgent on macOS, an XDG autostart file on Linux, a value
// under the user's Run key on Windows. Nothing is installed and no
// administrator is needed; turning it off deletes the entry.
package autostart

import (
	"errors"
	"os"
	"path/filepath"
)

// Label names the entry (the app's bundle identifier).
const Label = "org.go-link.desk"

// ErrUnsupported: this system has no entry.
var ErrUnsupported = errors.New("autostart: not supported on this system")

// home is the user's home folder; tests replace it.
var home = os.UserHomeDir

// Enabled reports whether the entry exists.
func Enabled() bool { return enabled() }

// Enable writes the entry so exe starts at login.
func Enable(exe string) error {
	if !filepath.IsAbs(exe) {
		return errors.New("autostart: the program path must be absolute")
	}
	return enable(exe)
}

// Disable deletes the entry; no entry is not an error.
func Disable() error { return disable() }

// Executable is the running binary, its links resolved: what the entry
// starts.
func Executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}
