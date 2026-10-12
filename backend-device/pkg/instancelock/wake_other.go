// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !windows

package instancelock

// macOS brings the running app forward by itself when it is opened again;
// elsewhere there is no wake yet.
func listen(string, chan<- struct{}) func() { return func() {} }

// Wake is Windows' only: false.
func Wake(string) bool { return false }
