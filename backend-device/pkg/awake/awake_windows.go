// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package awake

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows keeps the display and the computer awake with a power request
// (PowerCreateRequest), which belongs to a handle rather than to a thread:
// releasing it is clearing it and closing the handle. powercfg /requests
// lists it with its reason while a session is on.

var (
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	procCreateReq    = kernel32.NewProc("PowerCreateRequest")
	procSetReq       = kernel32.NewProc("PowerSetRequest")
	procClearReq     = kernel32.NewProc("PowerClearRequest")
	request          windows.Handle
	reason, _        = windows.UTF16PtrFromString("A go-link Desk session is on")
	requestTypes     = []uintptr{0, 1} // PowerRequestDisplayRequired, PowerRequestSystemRequired
	simpleReasonFlag = uint32(1)       // POWER_REQUEST_CONTEXT_SIMPLE_STRING
)

// reasonContext is REASON_CONTEXT with its simple string (the union is as
// large as its detailed form).
type reasonContext struct {
	version uint32
	flags   uint32
	reason  *uint16
	_       [2]uintptr
}

func hold() bool {
	ctx := reasonContext{flags: simpleReasonFlag, reason: reason}
	h, _, _ := procCreateReq.Call(uintptr(unsafe.Pointer(&ctx)))
	if h == 0 || windows.Handle(h) == windows.InvalidHandle {
		return false
	}
	for _, t := range requestTypes {
		if ok, _, _ := procSetReq.Call(h, t); ok == 0 {
			for _, t := range requestTypes {
				procClearReq.Call(h, t)
			}
			windows.CloseHandle(windows.Handle(h))
			return false
		}
	}
	request = windows.Handle(h)
	return true
}

func release() {
	for _, t := range requestTypes {
		procClearReq.Call(uintptr(request), t)
	}
	windows.CloseHandle(request)
	request = 0
}
