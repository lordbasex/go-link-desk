// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

// Package fyneview is the main window and Settings of go-link Desk drawn
// with Fyne, as macOS draws them natively (pkg/remoteview): the same
// layout, sizes and colors, for systems without that native window
// (Windows). It takes the same data (remoteview.Home, remoteview.Prefs)
// and answers with the same actions.
package fyneview

import (
	"embed"
	"image/color"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// The brand's colors, as the Mac app's (remoteview_darwin.m, from
// frontend/packages/shared/src/tokens.css): dark and light.
type palette struct {
	bg, video, surface, surface2, selected, border    color.NRGBA
	text, muted, faint, accent, onAccent, ok, danger  color.NRGBA
	tintBG, tintBorder, tintText, fieldBorder, holder color.NRGBA
	dashed, dangerFill                                color.NRGBA
}

func hex(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}
}

var dark = palette{
	bg: hex(0x0e1016), video: hex(0x05060a), surface: hex(0x161a23), surface2: hex(0x1b2030),
	selected: hex(0x1e2330), border: hex(0x262c3a), text: hex(0xe9ecf2), muted: hex(0xa3abbd),
	faint: hex(0x8c95a8), accent: hex(0xf2a33a), onAccent: hex(0x1a1206), ok: hex(0x7ee2a8),
	danger: hex(0xf4a9b6), tintBG: hex(0x2a1d0c), tintBorder: hex(0x6b4a1c), tintText: hex(0xd9a35a),
	fieldBorder: hex(0x3a4256), holder: hex(0x7f889c), dashed: hex(0x4a5368), dangerFill: hex(0x5a2a33),
}

var light = palette{
	bg: hex(0xf3f4f7), video: hex(0x05060a), surface: hex(0xffffff), surface2: hex(0xeceef3),
	selected: hex(0xe2e6ee), border: hex(0xd8dce5), text: hex(0x151821), muted: hex(0x4b5366),
	faint: hex(0x6b7386), accent: hex(0xd97e12), onAccent: hex(0x1a1206), ok: hex(0x1d8f55),
	danger: hex(0xc23a4c), tintBG: hex(0xfdf2e1), tintBorder: hex(0xf1c88a), tintText: hex(0xb06a12),
	fieldBorder: hex(0xc9ced9), holder: hex(0x8a91a1), dashed: hex(0xc0c6d2), dangerFill: hex(0xf7d7dc),
}

var (
	palMu sync.Mutex
	pal   = dark
)

// c is the current palette.
func c() palette {
	palMu.Lock()
	defer palMu.Unlock()
	return pal
}

func setPalette(p palette) {
	palMu.Lock()
	pal = p
	palMu.Unlock()
}

// Inter (SIL Open Font License 1.1), the closest to the Mac's system font
// that may be shipped: its Latin letters only, with tabular figures by
// default, as the Mac app's monospaced digits.
//
//go:embed assets/Inter-Regular.ttf assets/Inter-Medium.ttf assets/Inter-SemiBold.ttf assets/Inter-Bold.ttf
var fonts embed.FS

type weight int

const (
	regular weight = iota
	medium
	semibold
	bold
)

var fontRes = func() map[weight]fyne.Resource {
	m := map[weight]fyne.Resource{}
	for w, name := range map[weight]string{regular: "Inter-Regular.ttf", medium: "Inter-Medium.ttf", semibold: "Inter-SemiBold.ttf", bold: "Inter-Bold.ttf"} {
		b, err := fonts.ReadFile("assets/" + name)
		if err != nil {
			panic(err)
		}
		m[w] = fyne.NewStaticResource(name, b)
	}
	return m
}()

// Lucide's icons (ISC license), drawn in thin lines like the Mac's
// symbols.
//
//go:embed assets/icons/*.svg
var icons embed.FS

var (
	iconMu    sync.Mutex
	iconCache = map[string]fyne.Resource{}
)

// iconRes is an icon in a color; a name ending in "-fill" is the icon
// filled too (the favorite's star).
func iconRes(name string, col color.NRGBA) fyne.Resource {
	key := name + colorKey(col)
	filled := strings.HasSuffix(name, "-fill")
	name = strings.TrimSuffix(name, "-fill")
	iconMu.Lock()
	defer iconMu.Unlock()
	if r, ok := iconCache[key]; ok {
		return r
	}
	b, err := icons.ReadFile("assets/icons/" + name + ".svg")
	if err != nil {
		b, _ = icons.ReadFile("assets/icons/monitor.svg")
	}
	s := strings.ReplaceAll(string(b), "currentColor", colorKey(col))
	if filled {
		s = strings.ReplaceAll(s, `fill="none"`, `fill="`+colorKey(col)+`"`)
	}
	s = strings.ReplaceAll(s, `stroke-width="2"`, `stroke-width="1.75"`)
	r := fyne.NewStaticResource(key+".svg", []byte(s))
	iconCache[key] = r
	return r
}

func colorKey(col color.NRGBA) string {
	const digits = "0123456789abcdef"
	b := []byte{'#', 0, 0, 0, 0, 0, 0}
	for i, v := range []uint8{col.R, col.G, col.B} {
		b[1+2*i], b[2+2*i] = digits[v>>4], digits[v&15]
	}
	return string(b)
}

// Symbols maps the Mac's SF Symbols names to these icons.
var symbols = map[string]string{
	"gearshape": "gear", "display": "monitor", "desktopcomputer": "monitor", "link": "link",
	"doc.on.doc": "copy", "arrow.clockwise": "rotate-cw", "checkmark.shield": "shield-check",
	"exclamationmark.shield": "shield-alert", "house.fill": "house-fill", "square.grid.2x2": "layout-grid",
	"rectangle.grid.1x2": "rows-2", "list.bullet": "list", "star": "star", "star.fill": "star-fill",
	"ellipsis": "ellipsis", "arrow.right": "arrow-right", "display.2": "monitor-smartphone",
	"lock.shield": "lock-keyhole", "hand.raised": "hand", "xmark": "x", "info.circle": "info",
	"folder": "folder", "bubble.left.and.bubble.right": "message-square", "slider.horizontal.3": "sliders-horizontal",
	"speaker.wave.2": "volume-2", "command": "command", "arrow.up.left.and.arrow.down.right": "maximize-2",
	"xmark.circle.fill": "circle-x",
}

// appTheme gives Fyne's own widgets (fields, lists, menus, dialogs) the
// Mac app's colors and font.
type appTheme struct{}

func (appTheme) Color(n fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	p := c()
	switch n {
	case theme.ColorNameBackground:
		return p.bg
	case theme.ColorNameForeground:
		return p.text
	case theme.ColorNamePlaceHolder:
		return p.holder
	case theme.ColorNamePrimary, theme.ColorNameFocus, theme.ColorNameHyperlink:
		return p.accent
	case theme.ColorNameForegroundOnPrimary:
		return p.onAccent
	case theme.ColorNameButton:
		return p.surface2
	case theme.ColorNameDisabledButton:
		return p.surface2
	case theme.ColorNameDisabled:
		return p.faint
	case theme.ColorNameInputBackground:
		return p.bg
	case theme.ColorNameInputBorder:
		return p.fieldBorder
	case theme.ColorNameHover:
		return p.selected
	case theme.ColorNamePressed:
		return p.selected
	case theme.ColorNameSelection:
		return withAlpha(p.accent, 0x55)
	case theme.ColorNameSeparator:
		return p.border
	case theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground, theme.ColorNameHeaderBackground:
		return p.surface
	case theme.ColorNameScrollBar:
		return withAlpha(p.faint, 0x99)
	case theme.ColorNameScrollBarBackground:
		return color.Transparent
	case theme.ColorNameShadow:
		return color.NRGBA{A: 0x66}
	case theme.ColorNameSuccess:
		return p.ok
	case theme.ColorNameError:
		return p.danger
	case theme.ColorNameWarning:
		return p.accent
	}
	return theme.DefaultTheme().Color(n, theme.VariantDark)
}

func (appTheme) Font(s fyne.TextStyle) fyne.Resource {
	if s.Bold {
		return fontRes[semibold]
	}
	if s.Monospace || s.Symbol {
		return theme.DefaultTheme().Font(s)
	}
	return fontRes[regular]
}

func (appTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(n) }

func (appTheme) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case theme.SizeNameText:
		return 13
	case theme.SizeNameInputRadius, theme.SizeNameSelectionRadius:
		return 7
	case theme.SizeNameScrollBarSmall:
		return 3
	}
	return theme.DefaultTheme().Size(n)
}

func withAlpha(col color.NRGBA, a uint8) color.NRGBA {
	col.A = a
	return col
}
