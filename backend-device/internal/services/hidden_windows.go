// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import "golang.org/x/sys/windows"

// hiddenOnDisk reports whether Windows marks a file or folder hidden, as it
// does AppData, NTUSER.DAT and the old links into AppData. Not the system
// attribute: Windows also sets it on folders it customizes, Documents may
// be one.
func hiddenOnDisk(path string) bool {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return true
	}
	attrs, err := windows.GetFileAttributes(p)
	if err != nil {
		return false // gone or unreadable: opening it fails on its own
	}
	return attrs&windows.FILE_ATTRIBUTE_HIDDEN != 0
}
