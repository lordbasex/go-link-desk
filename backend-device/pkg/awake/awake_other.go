// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !darwin && !linux && !windows

package awake

// Elsewhere there is nothing to hold yet (Windows comes later).
func hold() bool { return false }

func release() {}
