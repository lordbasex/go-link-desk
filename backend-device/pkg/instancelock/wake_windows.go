// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package instancelock

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// Windows: a named event of this user's session, named after the lock's
// path. The running process waits on it; a second one sets it, and lets the
// first take the foreground (the person just opened the app, so the new
// process may hand that on).

var procAllowSetForeground = windows.NewLazySystemDLL("user32.dll").NewProc("AllowSetForegroundWindow")

func eventName(path string) *uint16 {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	sum := sha256.Sum256([]byte(strings.ToLower(abs)))
	name, _ := windows.UTF16PtrFromString(`Local\go-link-desk-wake-` + hex.EncodeToString(sum[:8]))
	return name
}

func listen(path string, woken chan<- struct{}) func() {
	ev, err := windows.CreateEvent(nil, 0, 0, eventName(path)) // auto-reset
	if err != nil {
		return func() {}
	}
	quit, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		windows.CloseHandle(ev)
		return func() {}
	}
	go func() {
		defer windows.CloseHandle(ev)
		defer windows.CloseHandle(quit)
		for {
			r, err := windows.WaitForMultipleObjects([]windows.Handle{ev, quit}, false, windows.INFINITE)
			if err != nil || r != windows.WAIT_OBJECT_0 {
				return
			}
			select {
			case woken <- struct{}{}:
			default: // one is waiting already
			}
		}
	}()
	return func() { windows.SetEvent(quit) }
}

// Wake asks the process holding the lock at path to show itself; false
// when none answers.
func Wake(path string) bool {
	ev, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, eventName(path))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(ev)
	procAllowSetForeground.Call(uintptr(0xFFFFFFFF)) // ASFW_ANY
	return windows.SetEvent(ev) == nil
}
