// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package autostart

import (
	"errors"

	"golang.org/x/sys/windows/registry"
)

// Windows starts the programs named under the user's Run key at login
// (Settings › Apps › Startup lists them, and can turn them off).

// Supported reports whether this system has an entry.
const Supported = true

// runKey is the user's Run key (tests use one of their own), and value
// the entry's name in it.
var runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

const value = "go-link Desk"

func enabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(value)
	return err == nil
}

// enable writes the command: the program's path in quotes, as a path with
// spaces needs.
func enable(exe string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(value, `"`+exe+`"`)
}

func disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(value); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}
