// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless && !darwin

package gui

// floatWindow is macOS only for now: elsewhere the window opens where the
// system puts it (Windows and Linux come in a later phase).
func floatWindow(string, bool) {}
