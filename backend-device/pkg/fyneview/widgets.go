// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package fyneview

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// text is a label in Inter at a size and weight, as the Mac's label().
func text(s string, size float32, w weight, col color.Color) *canvas.Text {
	t := canvas.NewText(s, col)
	t.TextSize = size
	t.FontSource = fontRes[w]
	return t
}

// icon is a symbol at a size, in a color.
func icon(symbol string, size float32, col color.NRGBA) *canvas.Image {
	name := symbols[symbol]
	if name == "" {
		name = symbol
	}
	img := canvas.NewImageFromResource(iconRes(name, col))
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(size, size))
	return img
}

// box is a rounded rectangle: a fill, and a border when stroke is set.
func box(fill color.Color, radius float32, stroke color.Color, width float32) *canvas.Rectangle {
	r := canvas.NewRectangle(fill)
	r.CornerRadius = radius
	if stroke != nil {
		r.StrokeColor = stroke
		r.StrokeWidth = width
	}
	return r
}

// fixed keeps an object at a size (it may still grow in a layout that
// stretches it).
type fixed struct {
	size fyne.Size
}

func (f fixed) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Resize(size)
		o.Move(fyne.NewPos(0, 0))
	}
}

func (f fixed) MinSize([]fyne.CanvasObject) fyne.Size { return f.size }

func sized(w, h float32, objs ...fyne.CanvasObject) *fyne.Container {
	return container.New(fixed{fyne.NewSize(w, h)}, objs...)
}

// pad puts space around an object.
type padding struct{ top, right, bottom, left float32 }

func (p padding) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(p.left, p.top))
		o.Resize(fyne.NewSize(size.Width-p.left-p.right, size.Height-p.top-p.bottom))
	}
}

func (p padding) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var m fyne.Size
	for _, o := range objs {
		m = m.Max(o.MinSize())
	}
	return m.Add(fyne.NewSize(p.left+p.right, p.top+p.bottom))
}

func pad(top, right, bottom, left float32, objs ...fyne.CanvasObject) *fyne.Container {
	return container.New(padding{top, right, bottom, left}, objs...)
}

// row lays objects out left to right with a gap, centered vertically,
// each at its own width (a spacer takes what is left).
type rowLayout struct{ gap float32 }

func (r rowLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	var fixedW float32
	spacers := 0
	visible := 0
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		visible++
		if _, ok := o.(layout.SpacerObject); ok {
			spacers++
			continue
		}
		fixedW += o.MinSize().Width
	}
	if visible > 1 {
		fixedW += r.gap * float32(visible-1)
	}
	extra := float32(0)
	if spacers > 0 && size.Width > fixedW {
		extra = (size.Width - fixedW) / float32(spacers)
	}
	x := float32(0)
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		if _, ok := o.(layout.SpacerObject); ok {
			x += extra + r.gap
			continue
		}
		m := o.MinSize()
		o.Resize(m)
		o.Move(fyne.NewPos(x, (size.Height-m.Height)/2))
		x += m.Width + r.gap
	}
}

func (r rowLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var w, h float32
	n := 0
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		n++
		m := o.MinSize()
		w += m.Width
		if m.Height > h {
			h = m.Height
		}
	}
	if n > 1 {
		w += r.gap * float32(n-1)
	}
	return fyne.NewSize(w, h)
}

func row(gap float32, objs ...fyne.CanvasObject) *fyne.Container {
	return container.New(rowLayout{gap}, objs...)
}

// column stacks objects top to bottom, each at its own height, aligned
// left (or right), with a gap after each (gaps[i] after object i, else
// gap).
type columnLayout struct {
	gap   float32
	gaps  map[int]float32
	right bool
}

func (c columnLayout) after(i int) float32 {
	if g, ok := c.gaps[i]; ok {
		return g
	}
	return c.gap
}

func (c columnLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	y := float32(0)
	for i, o := range objs {
		if !o.Visible() {
			continue
		}
		m := o.MinSize()
		w := m.Width
		if w > size.Width {
			w = size.Width
		}
		x := float32(0)
		if c.right {
			x = size.Width - w
		}
		o.Resize(fyne.NewSize(w, m.Height))
		o.Move(fyne.NewPos(x, y))
		y += m.Height + c.after(i)
	}
}

func (c columnLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var w, h float32
	last := -1
	for i, o := range objs {
		if !o.Visible() {
			continue
		}
		m := o.MinSize()
		if m.Width > w {
			w = m.Width
		}
		h += m.Height
		if last >= 0 {
			h += c.after(last)
		}
		last = i
	}
	return fyne.NewSize(w, h)
}

// tap makes anything clickable: a click, a right click, the hover, and
// the hand pointer.
type tap struct {
	widget.BaseWidget
	content   fyne.CanvasObject
	onTap     func()
	onSecond  func(*fyne.PointEvent)
	onHover   func(bool)
	tooltip   string
	noPointer bool
}

func newTap(content fyne.CanvasObject, onTap func()) *tap {
	t := &tap{content: content, onTap: onTap}
	t.ExtendBaseWidget(t)
	return t
}

func (t *tap) CreateRenderer() fyne.WidgetRenderer { return widget.NewSimpleRenderer(t.content) }

func (t *tap) Tapped(*fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
}

func (t *tap) TappedSecondary(e *fyne.PointEvent) {
	if t.onSecond != nil {
		t.onSecond(e)
	}
}

func (t *tap) MouseIn(*desktop.MouseEvent) {
	if t.onHover != nil {
		t.onHover(true)
	}
}
func (t *tap) MouseMoved(*desktop.MouseEvent) {}
func (t *tap) MouseOut() {
	if t.onHover != nil {
		t.onHover(false)
	}
}

func (t *tap) Cursor() desktop.Cursor {
	if t.noPointer {
		return desktop.DefaultCursor
	}
	return desktop.PointerCursor
}

// button is the Mac app's flat button (RVButton): a rounded fill, a symbol
// and a title, 30 high; primary is the brand's orange.
type button struct {
	*tap
	bg      *canvas.Rectangle
	label   *canvas.Text
	glyph   *canvas.Image
	symbol  string
	primary bool
	fill    color.Color // in place of the default fill
	enabled bool
	inner   *fyne.Container // the symbol and the title
}

func newButton(title, symbol string, primary bool, onTap func()) *button {
	b := &button{symbol: symbol, primary: primary, enabled: true}
	b.bg = box(nil, 7, nil, 0)
	var parts []fyne.CanvasObject
	if title != "" {
		b.label = text(title, 13, regular, nil)
		if primary {
			b.label.FontSource = fontRes[semibold]
		}
	}
	if symbol != "" {
		b.glyph = icon(symbol, 15, c().text)
	}
	b.tap = newTap(nil, func() {
		if b.enabled && onTap != nil {
			onTap()
		}
	})
	b.restyle()
	padX := float32(11)
	if title == "" {
		padX = 7.5
	}
	if b.glyph != nil {
		parts = append(parts, b.glyph)
	}
	if b.label != nil {
		parts = append(parts, b.label)
	}
	b.inner = row(6, parts...)
	b.tap.content = container.NewStack(b.bg, pad(0, padX, 0, padX, container.NewCenter(b.inner)))
	return b
}

// trailingSymbol puts the symbol after the title (Connect →).
func (b *button) trailingSymbol() *button {
	if b.glyph != nil && b.label != nil {
		b.inner.Objects = []fyne.CanvasObject{b.label, b.glyph}
	}
	return b
}

func (b *button) restyle() {
	p := c()
	fg := p.text
	if b.primary {
		fg = p.onAccent
	}
	if !b.enabled {
		fg = p.faint
	}
	var fill color.Color = p.surface2
	switch {
	case b.fill != nil:
		fill = b.fill
	case b.primary && b.enabled:
		fill = p.accent
	}
	b.bg.FillColor = fill
	b.bg.Refresh()
	if b.label != nil {
		b.label.Color = fg
		b.label.Refresh()
	}
	if b.glyph != nil {
		b.glyph.Resource = iconRes(symbolName(b.symbol), fg)
		b.glyph.Refresh()
	}
}

func symbolName(s string) string {
	if n := symbols[s]; n != "" {
		return n
	}
	return s
}

func (b *button) setEnabled(on bool) {
	b.enabled = on
	b.restyle()
}

func (b *button) MinSize() fyne.Size {
	m := b.tap.MinSize()
	if m.Height < 30 {
		m.Height = 30
	}
	if b.label == nil && m.Width < 30 {
		m.Width = 30
	}
	return m
}

// tint recolors a symbol button's glyph (the permissions' shield).
func (b *button) tint(col color.NRGBA) {
	if b.glyph != nil {
		b.glyph.Resource = iconRes(symbolName(b.symbol), col)
		b.glyph.Refresh()
	}
}

// iconButton is a bare symbol that can be clicked (the views, the star,
// the "more" menu).
func iconButton(symbol string, size float32, col color.NRGBA, onTap func()) (*tap, *canvas.Image) {
	img := icon(symbol, size, col)
	return newTap(sized(size+6, size+6, container.NewCenter(img)), onTap), img
}
