// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build windows && !headless

package fyneview

import (
	"image/color"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	"golang.org/x/sys/windows"
)

// The windows' frame in the app's colors, as the Mac's: the title bar in
// the top bar's color, its title in the text's, the border in the
// lines'. Windows draws them (DWM) and keeps its own buttons. Windows 11
// takes the colors; Windows 10 only a dark or light frame.

var (
	dwmapi                 = windows.NewLazySystemDLL("dwmapi.dll")
	procSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
)

const (
	dwmwaDarkModeOld  = 19 // Windows 10 before 20H1
	dwmwaDarkMode     = 20
	dwmwaBorderColor  = 34
	dwmwaCaptionColor = 35
	dwmwaTextColor    = 36
)

func setAttribute(hwnd uintptr, attr uint32, v uint32) {
	if procSetWindowAttribute.Find() != nil {
		return
	}
	procSetWindowAttribute.Call(hwnd, uintptr(attr), uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v))
}

// colorRef is a Win32 COLORREF: 0x00BBGGRR.
func colorRef(c color.NRGBA) uint32 { return uint32(c.R) | uint32(c.G)<<8 | uint32(c.B)<<16 }

// styleFrame colors a shown window's frame with the palette.
func styleFrame(w fyne.Window) {
	nw, ok := w.(driver.NativeWindow)
	if !ok {
		return
	}
	p := c()
	nw.RunNative(func(ctx any) {
		wc, ok := ctx.(driver.WindowsWindowContext)
		if !ok || wc.HWND == 0 {
			return
		}
		night := uint32(0)
		if p == dark {
			night = 1
		}
		setAttribute(wc.HWND, dwmwaDarkModeOld, night)
		setAttribute(wc.HWND, dwmwaDarkMode, night)
		setAttribute(wc.HWND, dwmwaCaptionColor, colorRef(p.surface))
		setAttribute(wc.HWND, dwmwaTextColor, colorRef(p.text))
		setAttribute(wc.HWND, dwmwaBorderColor, colorRef(p.border))
	})
}
