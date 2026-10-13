// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build (windows || linux) && !headless

package fyneview

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// The windows' frame in the app's colors, as the Mac's (paintFrame, one
// per system): the title bar in the top bar's color, its title in the
// text's, the border in the lines'.

// styleFrame colors a shown window's frame with the palette.
func styleFrame(w fyne.Window) {
	p := c()
	paintFrame(w, p == dark, p.surface, p.text, p.border)
}

// StyleWindow colors the frame of one of the app's other windows (the
// connection request, the session) like its insides: Fyne's background
// and text in the windows' variant.
func StyleWindow(w fyne.Window) {
	v := windowVariant()
	th := theme.DefaultTheme()
	nrgba := func(n fyne.ThemeColorName) color.NRGBA {
		return color.NRGBAModel.Convert(th.Color(n, v)).(color.NRGBA)
	}
	paintFrame(w, v == theme.VariantDark, nrgba(theme.ColorNameBackground), nrgba(theme.ColorNameForeground), nrgba(theme.ColorNameSeparator))
}
