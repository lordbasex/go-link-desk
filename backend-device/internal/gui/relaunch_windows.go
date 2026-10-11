// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless && windows

package gui

// relaunch is not offered on Windows yet: the person opens it again.
func (u *ui) relaunch() {}
