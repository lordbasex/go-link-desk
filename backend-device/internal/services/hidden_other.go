// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !windows

package services

// hiddenOnDisk is Windows' only: elsewhere a leading dot hides a file.
func hiddenOnDisk(string) bool { return false }
