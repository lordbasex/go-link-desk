// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !darwin

package awake

// Elsewhere there is nothing to hold yet (Windows and Linux come later).
func hold() bool { return false }

func release() {}
