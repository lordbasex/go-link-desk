// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !windows && !linux

package instancelock

// macOS brings the running app forward by itself when it is opened again.
func listen(string, chan<- struct{}) func() { return func() {} }

// Wake is Windows' and Linux's only: false.
func Wake(string) bool { return false }
