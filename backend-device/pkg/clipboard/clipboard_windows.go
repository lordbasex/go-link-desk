// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build windows && !headless

package clipboard

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows' clipboard as Unicode text (CF_UNICODETEXT). Its sequence number
// moves on every change. Another program may hold it open for a moment:
// opening it is tried a few times.

var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	procOpen         = user32.NewProc("OpenClipboard")
	procClose        = user32.NewProc("CloseClipboard")
	procEmpty        = user32.NewProc("EmptyClipboard")
	procGetData      = user32.NewProc("GetClipboardData")
	procSetData      = user32.NewProc("SetClipboardData")
	procIsAvailable  = user32.NewProc("IsClipboardFormatAvailable")
	procSequence     = user32.NewProc("GetClipboardSequenceNumber")
	procGlobalAlloc  = kernel32.NewProc("GlobalAlloc")
	procGlobalFree   = kernel32.NewProc("GlobalFree")
	procGlobalLock   = kernel32.NewProc("GlobalLock")
	procGlobalUnlock = kernel32.NewProc("GlobalUnlock")
	procGlobalSize   = kernel32.NewProc("GlobalSize")
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

// Supported reports whether this build shares the clipboard.
const Supported = true

type win struct{}

// System is the computer's clipboard.
func System() Board { return win{} }

func open() bool {
	for i := 0; i < 10; i++ {
		if r, _, _ := procOpen.Call(0); r != 0 {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func (win) ChangeCount() int {
	r, _, _ := procSequence.Call()
	return int(uint32(r))
}

func (win) Text() (string, bool) {
	if r, _, _ := procIsAvailable.Call(cfUnicodeText); r == 0 {
		return "", false
	}
	if !open() {
		return "", false
	}
	defer procClose.Call()
	h, _, _ := procGetData.Call(cfUnicodeText)
	if h == 0 {
		return "", false
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		return "", false
	}
	defer procGlobalUnlock.Call(h)
	size, _, _ := procGlobalSize.Call(h)
	// Up to its terminating zero, never past the memory it has.
	u := unsafe.Slice((*uint16)(global(p)), size/2)
	for i, c := range u {
		if c == 0 {
			u = u[:i]
			break
		}
	}
	return windows.UTF16ToString(u), true
}

func (b win) SetText(s string) int {
	u, err := windows.UTF16FromString(s)
	if err != nil { // a zero inside: up to it
		u = windows.StringToUTF16(s[:indexZero(s)])
	}
	if !open() {
		return b.ChangeCount()
	}
	defer procClose.Call()
	procEmpty.Call()
	h, _, _ := procGlobalAlloc.Call(gmemMoveable, uintptr(len(u)*2))
	if h == 0 {
		return b.ChangeCount()
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		procGlobalFree.Call(h)
		return b.ChangeCount()
	}
	copy(unsafe.Slice((*uint16)(global(p)), len(u)), u)
	procGlobalUnlock.Call(h)
	// The clipboard owns the memory once it takes it.
	if r, _, _ := procSetData.Call(cfUnicodeText, h); r == 0 {
		procGlobalFree.Call(h)
	}
	return b.ChangeCount()
}

// global is memory GlobalLock gave: Windows', not Go's.
func global(p uintptr) unsafe.Pointer { return unsafe.Add(nil, p) }

func indexZero(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			return i
		}
	}
	return len(s)
}
