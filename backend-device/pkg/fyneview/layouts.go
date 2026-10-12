// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package fyneview

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
)

func canvasTransparent() color.Color { return color.Transparent }

// sizedTheme is the app's theme for one field: its text size, weight and
// background, without a border (the field draws its own).
type sizedTheme struct {
	text        float32
	inputBorder float32
	fontW       weight
}

func (t *sizedTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameInputBackground, theme.ColorNameButton:
		return color.Transparent
	case theme.ColorNameInputBorder:
		return color.Transparent
	}
	return appTheme{}.Color(n, v)
}

func (t *sizedTheme) Font(s fyne.TextStyle) fyne.Resource {
	if s.Bold || s.Monospace || s.Symbol {
		return appTheme{}.Font(s)
	}
	return fontRes[t.fontW]
}

func (t *sizedTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return appTheme{}.Icon(n) }

func (t *sizedTheme) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case theme.SizeNameText:
		return t.text
	case theme.SizeNameInputBorder:
		return t.inputBorder
	case theme.SizeNameInnerPadding:
		return 4
	}
	return appTheme{}.Size(n)
}

// topBarLayout is the top bar's row: the field up to 520 wide, Connect
// next to it, Settings at the right end; 30 high.
type topBarLayout struct{}

func (topBarLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	field, connect, settings := objs[0], objs[1], objs[2]
	cw := connect.MinSize().Width
	sw := settings.MinSize().Width
	fw := size.Width - cw - sw - 16
	if fw > 520 {
		fw = 520
	}
	if fw < 160 {
		fw = 160
	}
	field.Move(fyne.NewPos(0, 0))
	field.Resize(fyne.NewSize(fw, 30))
	connect.Move(fyne.NewPos(fw+8, 0))
	connect.Resize(fyne.NewSize(cw, 30))
	settings.Move(fyne.NewPos(size.Width-sw, 0))
	settings.Resize(fyne.NewSize(sw, 30))
}

func (topBarLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(160+objs[1].MinSize().Width+objs[2].MinSize().Width+16, 30)
}

// columnLayoutFill stacks objects top to bottom at the whole width, each
// at its own height, gaps[i] after object i.
type columnLayoutFill struct{ gaps []float32 }

func (c columnLayoutFill) gap(i int) float32 {
	if i < len(c.gaps) {
		return c.gaps[i]
	}
	return 0
}

func (c columnLayoutFill) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	y := float32(0)
	for i, o := range objs {
		h := o.MinSize().Height
		o.Move(fyne.NewPos(0, y))
		o.Resize(fyne.NewSize(size.Width, h))
		y += h + c.gap(i)
	}
}

func (c columnLayoutFill) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var w, h float32
	for i, o := range objs {
		m := o.MinSize()
		if m.Width > w {
			w = m.Width
		}
		h += m.Height
		if i < len(objs)-1 {
			h += c.gap(i)
		}
	}
	return fyne.NewSize(w, h)
}

// underlineLayout puts a label with a thick line just under it (the
// chosen filter).
type underlineLayout struct{}

func (underlineLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	label, line := objs[0], objs[1]
	m := label.MinSize()
	label.Move(fyne.NewPos(0, 0))
	label.Resize(m)
	line.Move(fyne.NewPos(0, m.Height-2))
	line.Resize(fyne.NewSize(m.Width, 1.5))
}

func (underlineLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return objs[0].MinSize()
}

// heightLayout keeps its content at a height, as wide as it needs.
type heightLayout struct{ h float32 }

func (l *heightLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(size)
	}
}

func (l *heightLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var w float32
	for _, o := range objs {
		if m := o.MinSize(); m.Width > w {
			w = m.Width
		}
	}
	return fyne.NewSize(w, l.h)
}

// wrapText is text that wraps at width, as the Mac's wrapping labels: a
// column of canvas texts, line by line.
func wrapText(s string, size float32, col color.Color, width float32) fyne.CanvasObject {
	var lines []fyne.CanvasObject
	for _, l := range wrapLines(s, size, regular, width) {
		lines = append(lines, text(l, size, regular, col))
	}
	return fyneColumn(1, lines...)
}

func fyneColumn(gap float32, objs ...fyne.CanvasObject) *fyne.Container {
	return container.New(columnLayout{gap: gap}, objs...)
}

// wrapLines breaks s into lines no wider than width at a size and weight.
func wrapLines(s string, size float32, w weight, width float32) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		line := ""
		for _, word := range words {
			try := word
			if line != "" {
				try = line + " " + word
			}
			if line != "" && measureInter(try, size, w) > width {
				out = append(out, line)
				line = word
				continue
			}
			line = try
		}
		out = append(out, line)
	}
	return out
}

// measureInter is the width of t in Inter at a size and weight.
func measureInter(t string, size float32, w weight) float32 {
	ct := canvas.NewText(t, nil)
	ct.TextSize = size
	ct.FontSource = fontRes[w]
	return ct.MinSize().Width
}
