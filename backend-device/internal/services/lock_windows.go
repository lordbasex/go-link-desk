// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import "golang.org/x/sys/windows"

var procLockWorkStation = windows.NewLazySystemDLL("user32.dll").NewProc("LockWorkStation")

// lockWorkstation locks the screen (Windows+L).
func lockWorkstation() bool {
	r, _, _ := procLockWorkStation.Call()
	return r != 0
}
