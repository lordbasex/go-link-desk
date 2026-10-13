// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package fyneview

import (
	"image/color"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/remoteview"
)

// prefsWindow is Settings, as the Mac's (RVPrefs): a page per button of
// the bar at the top, each a list of boxed sections whose rows are a label
// on the left and a control on the right. Changes go to the main window's
// handler as "setting" ("<id>\n<value>").
type prefsWindow struct {
	win  fyne.Window
	spec remoteview.Prefs
	page string
	// A change came while a field was being typed in: shown when it ends.
	typing bool
	stale  bool
}

var (
	pwMu sync.Mutex
	gPW  *prefsWindow
)

func prefsWin() *prefsWindow {
	pwMu.Lock()
	defer pwMu.Unlock()
	return gPW
}

// SetPrefs shows Settings' pages (and the window when show).
func SetPrefs(spec remoteview.Prefs, show bool) {
	pw := prefsWin()
	if pw == nil {
		if !show {
			return
		}
		a := fyne.CurrentApp()
		if a == nil {
			return
		}
		pw = &prefsWindow{win: a.NewWindow("")}
		pw.win.SetPadded(false)
		pw.win.Resize(fyne.NewSize(640, 640))
		pw.win.SetCloseIntercept(pw.win.Hide)
		pwMu.Lock()
		gPW = pw
		pwMu.Unlock()
		pw.win.CenterOnScreen()
	}
	pw.spec = spec
	if pw.typing && !show {
		pw.stale = true
	} else {
		pw.rebuild()
	}
	if show {
		pw.win.Show()
		styleFrame(pw.win)
		pw.win.RequestFocus()
	}
}

func (pw *prefsWindow) send(id, value string) {
	if m := current(); m != nil {
		m.h.MainAction("setting", id+"\n"+value)
	}
}

// rebuild draws the chosen page.
func (pw *prefsWindow) rebuild() {
	p := c()
	pages := pw.spec.Pages
	if len(pages) == 0 {
		return
	}
	var page remoteview.PrefPage
	found := false
	for _, pg := range pages {
		if pg.ID == pw.page {
			page, found = pg, true
		}
	}
	if !found {
		page = pages[0]
		pw.page = page.ID
	}
	pw.win.SetTitle(page.Title)

	// The bar: a button per page, its symbol over its title, as the Mac's
	// preferences toolbar (the chosen one in the text's color on a grey
	// fill, the others muted).
	var buttons []fyne.CanvasObject
	for _, pg := range pages {
		pg := pg
		on := pg.ID == pw.page
		col := p.muted
		bg := box(nil, 7, nil, 0)
		if on {
			col = p.text
			bg.FillColor = p.selected
		}
		item := fyneColumn(3, container.NewCenter(icon(pg.Symbol, 22, col)), container.NewCenter(text(pg.Title, 11, regular, col)))
		buttons = append(buttons, newTap(container.NewStack(bg, pad(6, 10, 5, 10, item)), func() {
			pw.page = pg.ID
			pw.rebuild()
		}))
	}
	line := canvas.NewRectangle(p.border)
	line.SetMinSize(fyne.NewSize(1, 1))
	bar := container.NewStack(canvas.NewRectangle(p.surface),
		container.NewBorder(nil, line, nil, nil, pad(8, 0, 8, 0, container.NewCenter(row(4, buttons...)))))

	// The page: titled boxes of rows.
	var items []fyne.CanvasObject
	gaps := map[int]float32{}
	for _, sec := range page.Sections {
		if sec.Title != "" {
			if len(items) > 0 {
				gaps[len(items)-1] = 22
			}
			items = append(items, text(sec.Title, 13, bold, p.text))
		}
		var rows []fyne.CanvasObject
		for i, r := range sec.Rows {
			rows = append(rows, pw.row(r))
			if i+1 < len(sec.Rows) {
				l := canvas.NewRectangle(p.border)
				l.SetMinSize(fyne.NewSize(1, 1))
				rows = append(rows, pad(0, 14, 0, 14, l))
			}
		}
		items = append(items, container.NewStack(box(p.surface, 8, p.border, 1), container.New(columnLayoutFill{}, rows...)))
		if sec.Footer != "" {
			items = append(items, wrapText(sec.Footer, 11, p.faint, 560))
		}
	}
	body := container.New(&sectionsLayout{gap: 8, gaps: gaps}, items...)
	scroll := container.NewVScroll(pad(20, 20, 24, 20, body))
	pw.win.SetContent(container.NewThemeOverride(container.NewStack(canvas.NewRectangle(p.bg), container.NewBorder(bar, nil, nil, nil, scroll)), appTheme{}))
}

// sectionsLayout stacks Settings' parts at the whole width (titles and
// notes at their own), gaps[i] after part i, else gap.
type sectionsLayout struct {
	gap  float32
	gaps map[int]float32
}

func (l *sectionsLayout) after(i int) float32 {
	if g, ok := l.gaps[i]; ok {
		return g
	}
	return l.gap
}

func (l *sectionsLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	y := float32(0)
	for i, o := range objs {
		m := o.MinSize()
		w := size.Width
		if _, ok := o.(*canvas.Text); ok {
			w = m.Width
		}
		o.Move(fyne.NewPos(0, y))
		o.Resize(fyne.NewSize(w, m.Height))
		y += m.Height + l.after(i)
	}
}

func (l *sectionsLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var w, h float32
	for i, o := range objs {
		m := o.MinSize()
		if m.Width > w {
			w = m.Width
		}
		h += m.Height
		if i < len(objs)-1 {
			h += l.after(i)
		}
	}
	return fyne.NewSize(w, h)
}

func tintOf(t string) color.NRGBA {
	p := c()
	switch t {
	case "warn":
		return p.accent
	case "ok":
		return p.ok
	case "danger":
		return p.danger
	}
	return p.muted
}

// row is one setting: its label (and detail) on the left, its control
// on the right.
func (pw *prefsWindow) row(r remoteview.PrefRow) fyne.CanvasObject {
	p := c()
	enabled := r.Enabled == nil || *r.Enabled
	if r.Kind == "note" {
		return pad(10, 14, 10, 14, wrapText(r.Label, 12, tintOf(r.Tint), 540))
	}
	nameCol := p.text
	if !enabled {
		nameCol = p.faint
	}
	var left fyne.CanvasObject = wrapText(r.Label, 13, nameCol, 300)
	if r.Detail != "" {
		left = fyneColumn(2, left, wrapText(r.Detail, 11, p.faint, 330))
	}
	var right fyne.CanvasObject
	switch r.Kind {
	case "toggle":
		right = newSwitch(r.On, enabled, func(on bool) {
			v := "0"
			if on {
				v = "1"
			}
			pw.send(r.ID, v)
		})
	case "select":
		right = pw.selectBox(r, enabled)
	case "text":
		e := widget.NewEntry()
		e.SetText(r.Value)
		e.SetPlaceHolder(r.Placeholder)
		e.OnSubmitted = func(v string) { pw.send(r.ID, v); pw.typing = false; pw.renderIfStale() }
		e.OnChanged = func(string) { pw.typing = true }
		right = sized(220, 22, fieldStyle(e))
	case "password":
		f := widget.NewPasswordEntry()
		f.SetPlaceHolder(r.Placeholder)
		var f2 *widget.Entry
		fields := []fyne.CanvasObject{sized(170, 22, fieldStyle(f))}
		if r.Repeat != "" {
			f2 = widget.NewPasswordEntry()
			f2.SetPlaceHolder(r.Repeat)
			fields = append(fields, sized(170, 22, fieldStyle(f2)))
		}
		set := func() {
			v := f.Text
			if f2 != nil {
				v += "\n" + f2.Text
			}
			f.SetText("")
			if f2 != nil {
				f2.SetText("")
			}
			pw.typing = false
			pw.send(r.ID, v)
			pw.renderIfStale()
		}
		f.OnChanged = func(string) { pw.typing = true }
		if f2 != nil {
			f.OnSubmitted = func(string) { pw.win.Canvas().Focus(f2) }
			f2.OnSubmitted = func(string) { set() }
		} else {
			f.OnSubmitted = func(string) { set() }
		}
		b := smallButton(r.Button, false, set)
		right = row(6, fyneColumn(4, fields...), b)
	case "button":
		b := smallButton(r.Button, r.Tint == "danger", func() {
			if r.Confirm == nil {
				pw.send(r.ID, "")
				return
			}
			pw.confirm(*r.Confirm, func() { pw.send(r.ID, "") })
		})
		b.setEnabled(enabled)
		right = b
	case "code":
		right = text(r.Value, 17, semibold, p.accent)
	default:
		v := text(r.Value, 13, regular, tintOf(r.Tint))
		if r.Tint == "warn" || r.Tint == "danger" {
			right = row(5, icon("shield-alert", 13, tintOf(r.Tint)), v)
		} else {
			right = v
		}
	}
	return container.New(&settingRowLayout{}, left, right)
}

func (pw *prefsWindow) renderIfStale() {
	if pw.stale {
		pw.stale = false
		pw.rebuild()
	}
}

// settingRowLayout: the label on the left, the control on the right, both
// centered, at least 40 high, 14 in from the edges.
type settingRowLayout struct{}

func (settingRowLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	left, right := objs[0], objs[1]
	rm := right.MinSize()
	right.Resize(rm)
	right.Move(fyne.NewPos(size.Width-14-rm.Width, (size.Height-rm.Height)/2))
	lm := left.MinSize()
	lw := size.Width - 28 - rm.Width - 16
	if lw > lm.Width {
		lw = lm.Width
	}
	left.Resize(fyne.NewSize(lw, lm.Height))
	left.Move(fyne.NewPos(14, (size.Height-lm.Height)/2))
}

func (settingRowLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	lm, rm := objs[0].MinSize(), objs[1].MinSize()
	h := lm.Height + 20
	if rm.Height+12 > h {
		h = rm.Height + 12
	}
	if h < 40 {
		h = 40
	}
	return fyne.NewSize(lm.Width+rm.Width+44, h)
}

// selectBox is the Mac's small popup button: the chosen title and
// chevrons in a rounded box; a click shows the choices.
func (pw *prefsWindow) selectBox(r remoteview.PrefRow, enabled bool) fyne.CanvasObject {
	p := c()
	title := ""
	for _, o := range r.Options {
		if o.ID == r.Value {
			title = o.Title
		}
	}
	if title == "" && len(r.Options) > 0 {
		title = r.Options[0].Title
	}
	col := p.text
	if !enabled {
		col = p.faint
	}
	label := text(title, 12, regular, col)
	// As wide as its longest choice, as the Mac's pop-up button: the
	// title on the left, the arrows on the right, a plain fill.
	var widest float32
	for _, o := range r.Options {
		widest = max(widest, measureInter(o.Title, 12, regular))
	}
	var t *tap
	t = newTap(sized(widest+40, 22, container.NewStack(box(p.surface2, 5, nil, 0),
		pad(0, 6, 0, 9, row(6, label, layout.NewSpacer(), icon("chevrons-up-down", 11, p.text))))), func() {
		if !enabled {
			return
		}
		var items []*fyne.MenuItem
		for _, o := range r.Options {
			o := o
			it := fyne.NewMenuItem(o.Title, func() {
				label.Text = o.Title
				label.Refresh()
				pw.send(r.ID, o.ID)
			})
			it.Checked = o.ID == r.Value
			items = append(items, it)
		}
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(t)
		widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", items...), pw.win.Canvas(), pos.Add(fyne.NewPos(0, t.Size().Height+2)))
	})
	return t
}

// confirm asks before a button acts (the Mac's alert sheet).
func (pw *prefsWindow) confirm(cf remoteview.PrefConfirm, ok func()) {
	p := c()
	var pop *widget.PopUp
	cancel := smallButton(cf.Cancel, false, func() { pop.Hide() })
	yes := smallButton(cf.OK, true, func() { pop.Hide(); ok() })
	body := pad(18, 18, 18, 18, fyneColumn(10,
		text(cf.Title, 13, bold, p.text),
		wrapText(cf.Text, 12, p.muted, 300),
		container.NewHBox(layout.NewSpacer(), cancel, yes),
	))
	pop = widget.NewModalPopUp(container.NewStack(box(p.surface, 12, p.border, 1), body), pw.win.Canvas())
	pop.Show()
}

// smallButton is the Mac's small push button; danger ones in red.
func smallButton(title string, danger bool, onTap func()) *button {
	b := newButton(title, "", false, onTap)
	b.label.TextSize = 12
	if danger {
		b.fill = c().dangerFill
		b.restyle()
		b.label.Color = c().danger
	}
	return b
}

// fieldStyle gives a Fyne field the Mac look: the field's color, a thin
// border, no focus ring.
// fieldStyle is the Mac's small text field: square corners, a thin
// border, 12 points.
func fieldStyle(e *widget.Entry) fyne.CanvasObject {
	p := c()
	return container.NewStack(box(p.bg, 2, p.fieldBorder, 1),
		container.NewThemeOverride(e, &sizedTheme{text: 12, inputBorder: 0, fontW: regular}))
}

// switchView is the Mac's small switch.
type switchView struct {
	*tap
	on      bool
	track   *canvas.Rectangle
	knob    *canvas.Circle
	enabled bool
}

func newSwitch(on, enabled bool, changed func(bool)) *switchView {
	s := &switchView{on: on, enabled: enabled}
	s.track = box(nil, 9, nil, 0)
	s.knob = canvas.NewCircle(color.White)
	s.tap = newTap(container.New(&switchLayout{s}, s.track, s.knob), func() {
		if !s.enabled {
			return
		}
		s.on = !s.on
		s.paint()
		changed(s.on)
	})
	s.paint()
	return s
}

func (s *switchView) paint() {
	p := c()
	s.track.FillColor = p.surface2
	if s.on {
		s.track.FillColor = p.accent
	}
	if !s.enabled {
		s.track.FillColor = withAlpha(s.track.FillColor.(color.NRGBA), 0x77)
	}
	s.track.Refresh()
	s.tap.Refresh()
}

func (s *switchView) MinSize() fyne.Size { return fyne.NewSize(32, 18) }

type switchLayout struct{ s *switchView }

func (l *switchLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	track, knob := objs[0], objs[1]
	track.Move(fyne.NewPos(0, 0))
	track.Resize(fyne.NewSize(32, 18))
	x := float32(2)
	if l.s.on {
		x = 16
	}
	knob.Move(fyne.NewPos(x, 2))
	knob.Resize(fyne.NewSize(14, 14))
}

func (l *switchLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(32, 18) }

// ShowAbout shows the app's name, version and notice, as the Mac's About
// panel.
func ShowAbout(version, notice string) {
	a := fyne.CurrentApp()
	if a == nil {
		return
	}
	p := c()
	w := a.NewWindow("go-link Desk")
	var logo fyne.CanvasObject = layout.NewSpacer()
	if res := a.Icon(); res != nil {
		img := canvas.NewImageFromResource(res)
		img.FillMode = canvas.ImageFillContain
		img.SetMinSize(fyne.NewSize(72, 72))
		logo = img
	}
	var lines []fyne.CanvasObject
	for _, l := range strings.Split(notice, "\n") {
		lines = append(lines, container.NewCenter(wrapText(l, 11, p.muted, 300)))
	}
	body := fyneColumn(6, append([]fyne.CanvasObject{
		container.NewCenter(logo),
		container.NewCenter(text("go-link Desk", 15, bold, p.text)),
		container.NewCenter(text(version, 11, regular, p.faint)),
	}, lines...)...)
	w.SetContent(container.NewThemeOverride(container.NewStack(canvas.NewRectangle(p.bg), pad(22, 24, 22, 24, body)), appTheme{}))
	w.SetPadded(false)
	w.SetFixedSize(true)
	w.CenterOnScreen()
	w.Show()
	styleFrame(w)
}
