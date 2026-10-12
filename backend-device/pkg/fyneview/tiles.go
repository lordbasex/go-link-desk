// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package fyneview

import (
	"bytes"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/remoteview"
)

// The tiles' sizes, as the Mac app's (kTileW, kTileH, kRowH, kDetailH,
// kGap).
const (
	tileW   = 200
	tileH   = 150
	rowH    = 56
	detailH = 38
	gap     = 14
)

// renderTiles fills the computers' area for the filter and the view.
func (m *mainWin) renderTiles() {
	if m.tiles == nil {
		return
	}
	m.mu.Lock()
	filter, mode, h := m.filter, m.viewMode, m.home
	m.mu.Unlock()
	var items []remoteview.Tile
	linked := filter != "recents"
	if !linked {
		items = h.Recents
	} else {
		for _, d := range h.Devices {
			if filter == "favorites" && !d.Favorite {
				continue
			}
			items = append(items, d)
		}
	}
	var objs []fyne.CanvasObject
	switch {
	case len(items) == 0:
		note := m.L("empty")
		switch filter {
		case "recents":
			note = m.L("emptyRecents")
		case "favorites":
			note = m.L("emptyFavorites")
		}
		objs = append(objs, m.emptyNote(note))
	default:
		if mode == 2 {
			objs = append(objs, m.detailHeader())
		}
		for _, t := range items {
			objs = append(objs, m.tile(t, linked, mode))
		}
	}
	m.tiles.Layout = &gridLayout{mode: mode}
	m.tiles.Objects = objs
	m.tiles.Refresh()
	m.scroll.Refresh()
}

// gridLayout puts cards in rows that wrap, and list rows one under the
// other at the whole width.
type gridLayout struct{ mode int }

func (g *gridLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	if g.mode != 0 || len(objs) == 1 && !isTile(objs[0]) {
		y := float32(0)
		for _, o := range objs {
			h := o.MinSize().Height
			o.Move(fyne.NewPos(0, y))
			o.Resize(fyne.NewSize(size.Width, h))
			y += h
			if g.mode == 1 {
				y += 8
			}
		}
		return
	}
	perRow := int((size.Width + gap) / (tileW + gap))
	if perRow < 1 {
		perRow = 1
	}
	for i, o := range objs {
		col, line := i%perRow, i/perRow
		o.Move(fyne.NewPos(float32(col)*(tileW+gap), float32(line)*(tileH+gap)))
		o.Resize(fyne.NewSize(tileW, tileH))
	}
}

func (g *gridLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if g.mode != 0 || len(objs) == 1 && !isTile(objs[0]) {
		var h float32
		for _, o := range objs {
			h += o.MinSize().Height
			if g.mode == 1 {
				h += 8
			}
		}
		return fyne.NewSize(tileW, h)
	}
	// The height follows the width the scroll gives (set by Layout); the
	// rows of cards are counted at the window's usual width.
	perRow := 4
	lines := (len(objs) + perRow - 1) / perRow
	return fyne.NewSize(tileW, float32(lines)*(tileH+gap))
}

func isTile(o fyne.CanvasObject) bool {
	_, ok := o.(*tileView)
	return ok
}

// emptyNote is the dashed box that says what goes here.
func (m *mainWin) emptyNote(note string) fyne.CanvasObject {
	p := c()
	return container.NewStack(
		box(nil, 10, p.dashed, 1),
		pad(18, 18, 18, 18, wrapText(note, 13, p.muted, 560)),
	)
}

// detailHeader is the detailed list's column titles.
func (m *mainWin) detailHeader() fyne.CanvasObject {
	p := c()
	var cols []fyne.CanvasObject
	for _, k := range []string{"colName", "colCode", "colSystem", "colWhen"} {
		cols = append(cols, text(strings.ToUpper(m.L(k)), 10.5, semibold, p.faint))
	}
	line := canvas.NewRectangle(p.border)
	return container.New(&columnsLayout{height: 28, line: true}, append(cols, line)...)
}

// columnsLayout puts the detailed list's four columns at the Mac app's
// places (34, 36 %, 58 %, 74 % of the width), and a line at the bottom.
type columnsLayout struct {
	height float32
	line   bool
	lead   bool // an icon first, at 10
}

func (l *columnsLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	i := 0
	if l.lead {
		objs[0].Move(fyne.NewPos(10, (l.height-16)/2))
		objs[0].Resize(fyne.NewSize(16, 16))
		i = 1
	}
	at := []float32{34, size.Width * 0.36, size.Width * 0.58, size.Width * 0.74}
	end := size.Width - 70
	for k := 0; k < 4 && i < len(objs); k, i = k+1, i+1 {
		stop := end
		if k < 3 {
			stop = at[k+1] - 10
		}
		m := objs[i].MinSize()
		objs[i].Move(fyne.NewPos(at[k], (l.height-m.Height)/2))
		objs[i].Resize(fyne.NewSize(max(20, stop-at[k]), m.Height))
	}
	for ; i < len(objs); i++ {
		o := objs[i]
		if l.line && i == len(objs)-1 {
			o.Move(fyne.NewPos(0, l.height-1))
			o.Resize(fyne.NewSize(size.Width, 1))
			continue
		}
		// The star and the menu, at the right end.
		m := o.MinSize()
		x := size.Width - 34
		if i == len(objs)-3 || (i == len(objs)-2 && !l.line) {
			x = size.Width - 62
		}
		o.Move(fyne.NewPos(x, (l.height-m.Height)/2))
		o.Resize(m)
	}
}

func (l *columnsLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(400, l.height) }

// tileView is a computer: a card, a list row or a detailed row, with a
// star (linked ones), a "more" menu, a hover and a right click.
type tileView struct {
	*tap
	mode int
}

func (t *tileView) MinSize() fyne.Size {
	switch t.mode {
	case 0:
		return fyne.NewSize(tileW, tileH)
	case 1:
		return fyne.NewSize(tileW, rowH)
	}
	return fyne.NewSize(tileW, detailH)
}

func (m *mainWin) tile(t remoteview.Tile, linked bool, mode int) fyne.CanvasObject {
	p := c()
	sym := "display"
	if linked {
		sym = "desktopcomputer"
	}
	iconTint := p.faint
	if mode == 0 {
		iconTint = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	}
	tv := &tileView{mode: mode}
	var thumb fyne.CanvasObject
	if len(t.Thumb) > 0 {
		img := canvas.NewImageFromReader(bytes.NewReader(t.Thumb), t.ID+".jpg")
		img.FillMode = canvas.ImageFillContain
		img.FillMode = canvas.ImageFillStretch
		img.ScaleMode = canvas.ImageScaleSmooth
		thumb = img
	}
	starSym, starTint := "star", iconTint
	if t.Favorite {
		starSym, starTint = "star.fill", p.accent
	}
	star, _ := iconButton(starSym, 15, starTint, func() { m.h.MainAction("favorite", t.ID) })
	more, _ := iconButton("ellipsis", 15, iconTint, nil)
	more.onTap = func() { m.menu(t, linked, more) }

	var content fyne.CanvasObject
	var ring *canvas.Rectangle
	var hoverBG func(bool)
	switch mode {
	case 0:
		bg := box(p.surface2, 10, nil, 0)
		var picture fyne.CanvasObject = container.NewCenter(icon(sym, 34, p.faint))
		if thumb != nil {
			picture = thumb
		}
		band := canvas.NewRectangle(color.NRGBA{A: 184})
		band.BottomLeftCornerRadius, band.BottomRightCornerRadius = 10, 10
		title := text(t.Title, 12.5, semibold, color.White)
		mon := icon(sym, 15, p.text)
		ring = box(nil, 10, p.accent, 2)
		ring.Hide()
		var top []fyne.CanvasObject
		if linked {
			top = append(top, star)
		}
		content = container.New(&cardLayout{}, bg, picture, band, mon, title, more, ring, container.NewHBox(top...))
	case 1:
		bg := box(p.surface, 8, nil, 0)
		shot := container.NewStack(box(p.video, 5, nil, 0))
		if thumb != nil {
			shot.Add(thumb)
		}
		title := text(t.Title, 13, semibold, p.text)
		sub := text(t.Subtitle, 11.5, regular, p.faint)
		ends := []fyne.CanvasObject{more}
		if linked {
			ends = []fyne.CanvasObject{star, more}
		}
		content = container.New(&listRowLayout{}, bg, shot, title, sub, container.NewHBox(ends...))
		hoverBG = hoverFill(bg, p.surface2, p.surface)
	default:
		bg := box(color.Transparent, 8, nil, 0)
		cells := []fyne.CanvasObject{icon(sym, 15, iconTint)}
		for i, v := range []string{t.Name, t.Code, t.System, t.When} {
			w, col := regular, p.muted
			if i == 0 {
				w, col = medium, p.text
			}
			cells = append(cells, text(v, 12.5, w, col))
		}
		if linked {
			cells = append(cells, star)
		}
		cells = append(cells, more, canvas.NewRectangle(p.border))
		content = container.NewStack(bg, container.New(&columnsLayout{height: detailH, line: true, lead: true}, cells...))
		hoverBG = hoverFill(bg, p.surface2, color.Transparent)
	}
	tv.tap = newTap(content, func() {
		action := "recent"
		if linked {
			action = "device"
		}
		m.h.MainAction(action, t.ID)
	})
	tv.tap.onSecond = func(*fyne.PointEvent) { m.menu(t, linked, tv.tap) }
	tv.tap.onHover = func(in bool) {
		if ring != nil {
			ring.Hidden = !in
			ring.Refresh()
		}
		if hoverBG != nil {
			hoverBG(in)
		}
	}
	tv.ExtendBaseWidget(tv)
	if t.Subtitle != "" && mode == 0 {
		tv.tooltip = t.Subtitle
	}
	return tv
}

// hoverFill lightens a row's background under the pointer.
func hoverFill(bg *canvas.Rectangle, in, out color.Color) func(bool) {
	return func(on bool) {
		bg.FillColor = out
		if on {
			bg.FillColor = in
		}
		bg.Refresh()
	}
}

// cardLayout places a card's parts as the Mac app does.
type cardLayout struct{}

func (cardLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	bg, picture, band, mon, title, more, ring, top := objs[0], objs[1], objs[2], objs[3], objs[4], objs[5], objs[6], objs[7]
	full := fyne.NewSize(tileW, tileH)
	bg.Resize(full)
	ring.Resize(full)
	picture.Move(fyne.NewPos(0, 0))
	picture.Resize(fyne.NewSize(tileW, tileH-34))
	if _, ok := picture.(*canvas.Image); ok {
		picture.Resize(full)
	}
	band.Move(fyne.NewPos(0, tileH-36))
	band.Resize(fyne.NewSize(tileW, 36))
	mon.Move(fyne.NewPos(9, tileH-27))
	mon.Resize(fyne.NewSize(16, 16))
	tm := title.MinSize()
	title.Move(fyne.NewPos(30, tileH-18-tm.Height/2))
	title.Resize(fyne.NewSize(tileW-60, tm.Height))
	more.Move(fyne.NewPos(tileW-30, tileH-29))
	more.Resize(more.MinSize())
	top.Move(fyne.NewPos(tileW-28, 6))
	top.Resize(top.MinSize())
}

func (cardLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(tileW, tileH) }

// listRowLayout places a list row's parts: the picture, the title over
// the subtitle, the star and the menu at the right.
type listRowLayout struct{}

func (listRowLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	bg, shot, title, sub, ends := objs[0], objs[1], objs[2], objs[3], objs[4]
	bg.Resize(size)
	shot.Move(fyne.NewPos(8, 7))
	shot.Resize(fyne.NewSize(70, 42))
	title.Move(fyne.NewPos(90, 9))
	title.Resize(fyne.NewSize(size.Width-160, 18))
	sub.Move(fyne.NewPos(90, 30))
	sub.Resize(fyne.NewSize(size.Width-160, 16))
	em := ends.MinSize()
	ends.Move(fyne.NewPos(size.Width-em.Width-8, (size.Height-em.Height)/2))
	ends.Resize(em)
}

func (listRowLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(tileW, rowH) }

// menu is a computer's menu: Connect, the star, Rename and Forget for a
// linked one; Connect, Copy the code and Remove for a recent one.
func (m *mainWin) menu(t remoteview.Tile, linked bool, at fyne.CanvasObject) {
	connect := m.L("connect")
	var items []*fyne.MenuItem
	items = append(items, fyne.NewMenuItem(connect, func() {
		if linked {
			m.h.MainAction("device", t.ID)
		} else {
			m.h.MainAction("recent", t.ID)
		}
	}))
	if linked {
		star := m.L("favorite")
		if t.Favorite {
			star = m.L("unfavorite")
		}
		items = append(items,
			fyne.NewMenuItem(star, func() { m.h.MainAction("favorite", t.ID) }),
			fyne.NewMenuItem(m.L("rename"), func() { m.rename(t) }),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem(m.L("forget"), func() { m.h.MainAction("forget", t.ID) }),
		)
	} else {
		items = append(items,
			fyne.NewMenuItem(m.L("copyCode"), func() { m.app.Clipboard().SetContent(t.Code) }),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem(m.L("removeRecent"), func() { m.h.MainAction("remove-recent", t.ID) }),
		)
	}
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(at)
	widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", items...), m.win.Canvas(), pos.Add(fyne.NewPos(0, at.Size().Height+2)))
}

// rename asks for a linked computer's name here, in a small sheet over
// the window.
func (m *mainWin) rename(t remoteview.Tile) {
	p := c()
	e := widget.NewEntry()
	e.SetText(t.Name)
	var pop *widget.PopUp
	save := func() {
		pop.Hide()
		m.h.MainAction("rename", t.ID+"\n"+e.Text)
	}
	e.OnSubmitted = func(string) { save() }
	cancel := newButton(m.L("cancel"), "", false, func() { pop.Hide() })
	ok := newButton(m.L("save"), "", true, save)
	body := pad(16, 16, 16, 16, fyneColumn(12,
		text(m.L("renameTitle"), 13, semibold, p.text),
		sized(288, 34, e),
		row(8, cancel, ok),
	))
	pop = widget.NewModalPopUp(container.NewStack(box(p.surface, 12, p.border, 1), body), m.win.Canvas())
	pop.Show()
	m.win.Canvas().Focus(e)
}
