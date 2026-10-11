// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !darwin

package permission

func screenRecording() bool        { return true }
func requestScreenRecording() bool { return true }
func accessibility() bool          { return true }
func requestAccessibility() bool   { return true }
func openSettings(string)          {}

const needed = false
