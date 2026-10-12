// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !windows

package services

// lockWorkstation is Windows' only.
func lockWorkstation() bool { return false }
