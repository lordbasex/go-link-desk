// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !darwin && !linux

package autostart

// Supported reports whether this system has an entry.
const Supported = false

func entryPath() (string, error) { return "", ErrUnsupported }

func entry(string) []byte { return nil }
