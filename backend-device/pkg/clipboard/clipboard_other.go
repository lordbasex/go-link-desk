// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !darwin

package clipboard

// Supported reports whether this build shares the clipboard.
const Supported = false

type none struct{}

// System has no clipboard to share on this system yet.
func System() Board { return none{} }

func (none) ChangeCount() int     { return 0 }
func (none) Text() (string, bool) { return "", false }
func (none) SetText(s string) int { return 0 }
