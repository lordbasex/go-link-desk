// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package fyneview

import (
	"strings"
	"sync"
	"time"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/remoteview"
)

// Handler hears the main window's actions, as remoteview.MainHandler:
// "copy", "newcode", "settings", "allow", "connect" ("join:<code>" or
// "claim:<code>"), "device", "favorite", "forget", "rename", "recent",
// "clear", "setting" (Settings: "<id>\n<value>").
type Handler = remoteview.MainHandler

// mainWin is the one main window.
type mainWin struct {
	app    fyne.App
	win    fyne.Window
	labels map[string]string
	h      Handler

	mu       sync.Mutex
	home     remoteview.Home
	filter   string // all, favorites, recents
	chose    bool   // the person picked a filter
	viewMode int    // 0 cards, 1 list, 2 detailed list

	address   *widget.Entry
	doorLabel *canvas.Text
	doorIndex int // 0 a session, 1 a link
	tiles     *fyne.Container
	scroll    *container.Scroll

	// Home's parts that change with the state.
	code       *canvas.Text
	copied     *canvas.Text
	link       *canvas.Text
	linkRow    *fyne.Container
	status     *canvas.Text
	statusDot  *canvas.Circle
	copyBtn    *button
	renewBtn   *button
	permBtn    *button
	version    *canvas.Text
	filterTabs map[string]*filterTab
	clearBtn   *tap
	viewIcons  [3]*canvas.Image
	copiedOff  *time.Timer
}

var (
	gMu   sync.Mutex
	gMain *mainWin
)

// OpenMain builds the main window (not shown yet: ShowMain).
func OpenMain(labels map[string]string, h Handler) error {
	a := fyne.CurrentApp()
	if a == nil {
		return remoteview.ErrUnsupported
	}
	a.Settings().SetTheme(fontTheme{theme.DefaultTheme()})
	m := &mainWin{app: a, labels: labels, h: h, filter: "all"}
	m.viewMode = a.Preferences().IntWithFallback("org.go-link.desk.view", 0)
	m.win = a.NewWindow("go-link Desk")
	m.win.SetPadded(false) // the content reaches the edges, as the Mac's
	// The Mac's 1180 x 760 does not fit a 1366 x 768 laptop (or a
	// 1280 x 800 screen) with its taskbar: a little smaller.
	m.win.Resize(fyne.NewSize(1120, 680))
	m.win.SetCloseIntercept(m.win.Hide) // it lives in the tray
	m.build()
	m.win.CenterOnScreen()
	gMu.Lock()
	gMain = m
	gMu.Unlock()
	return nil
}

func current() *mainWin {
	gMu.Lock()
	defer gMu.Unlock()
	return gMain
}

// ShowMain brings the main window forward.
func ShowMain() {
	if m := current(); m != nil {
		m.win.Show()
		styleFrame(m.win)
		m.win.RequestFocus()
	}
}

// SetHome shows what Home has now.
func SetHome(h remoteview.Home) {
	if m := current(); m != nil {
		m.setState(h)
	}
}

// SetTheme switches the windows' colors (remoteview.ThemeDark, Light or
// System).
func SetTheme(mode int) {
	p := dark
	switch mode {
	case remoteview.ThemeLight:
		p = light
	case remoteview.ThemeSystem:
		if fyne.CurrentApp() != nil && fyne.CurrentApp().Settings().ThemeVariant() == 1 { // theme.VariantLight
			p = light
		}
	}
	setPalette(p)
	if m := current(); m != nil {
		m.build()
		styleFrame(m.win)
	}
	if pw := prefsWin(); pw != nil {
		pw.rebuild()
		styleFrame(pw.win)
	}
}

// SetIcon sets the windows' icon.
func SetIcon(png []byte) {
	if m := current(); m != nil && len(png) > 0 {
		m.win.SetIcon(fyne.NewStaticResource("go-link-desk.png", png))
	}
}

func (m *mainWin) L(key string) string {
	if v := m.labels[key]; v != "" {
		return v
	}
	return key
}

// build makes the window's insides in the palette's colors; built again
// when the theme changes.
func (m *mainWin) build() {
	p := c()

	// The top bar: the address field, Connect, Settings; then the tabs.
	m.address = widget.NewEntry()
	// The placeholder is drawn here, as the Mac's (13, regular): Fyne's
	// own measures it in the app's font, not Inter, and cuts its end.
	holder := text(m.L("address"), 13, regular, p.holder)
	m.address.OnSubmitted = func(string) { m.connect() }
	m.address.OnChanged = func(s string) {
		if f := formatCode(s); f != s {
			m.address.SetText(f)
			m.address.CursorColumn = len([]rune(f))
		}
		holder.Hidden = m.address.Text != ""
		holder.Refresh()
	}
	// Regular, not the Mac's medium: Fyne measures a field's text in the
	// regular face, and wider letters would be cut.
	fieldTheme := &sizedTheme{text: 14, inputBorder: 0, fontW: regular}
	glyph := icon("display", 15, p.faint)
	m.doorLabel = text(m.doorTitle(), 11, medium, p.text)
	var door *tap
	door = newTap(row(3, m.doorLabel, icon("chevrons-up-down", 10, p.muted)), func() {
		items := []*fyne.MenuItem{}
		for i, k := range []string{"doorSession", "doorLink"} {
			i := i
			it := fyne.NewMenuItem(m.L(k), func() {
				m.doorIndex = i
				m.doorLabel.Text = m.doorTitle()
				m.doorLabel.Refresh()
			})
			it.Checked = i == m.doorIndex
			items = append(items, it)
		}
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(door)
		widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", items...), m.win.Canvas(), pos.Add(fyne.NewPos(0, door.Size().Height+4)))
	})
	field := container.NewStack(
		box(p.bg, 7, p.fieldBorder, 1),
		container.NewBorder(nil, nil,
			pad(0, 0, 0, 10, container.NewCenter(glyph)),
			pad(0, 8, 0, 6, container.NewCenter(door)),
			container.NewStack(
				container.NewThemeOverride(m.address, fieldTheme),
				pad(0, 0, 0, 4, container.NewVBox(layout.NewSpacer(), holder, layout.NewSpacer())))),
	)
	connect := newButton(m.L("connect"), "arrow.right", true, m.connect).trailingSymbol()
	settings := newButton("", "gearshape", false, func() { m.h.MainAction("settings", "") })
	settings.fill = transparent
	settings.restyle()
	bar := container.New(&topBarLayout{}, field, connect, settings)

	homeTab := m.tab("go-link Desk", "house.fill", true)
	tabs := pad(0, 10, 0, 10, row(2, homeTab))
	line := canvas.NewRectangle(p.border)
	line.SetMinSize(fyne.NewSize(1, 1))
	top := container.NewStack(
		canvas.NewRectangle(p.surface),
		container.New(columnLayoutFill{gaps: []float32{8, 0}}, pad(8, 12, 0, 12, bar), tabs, line),
	)

	content := container.NewBorder(top, nil, nil, nil, m.buildHome())
	// Only these windows take the Mac app's theme: the app's other Fyne
	// windows keep Fyne's, as on macOS.
	m.win.SetContent(container.NewThemeOverride(container.NewStack(canvas.NewRectangle(p.bg), content), appTheme{}))
	m.setState(m.home)
}

var transparent = canvasTransparent()

// tab is one tab of the strip (Home has no close button).
func (m *mainWin) tab(title, symbol string, selected bool) fyne.CanvasObject {
	p := c()
	bg := box(nil, 8, nil, 0)
	t := text(title, 12.5, medium, p.muted)
	if selected {
		bg.FillColor = p.selected
		t.Color = p.text
	}
	ic := icon(symbol, 14, p.accent)
	return container.New(&heightLayout{30}, container.NewStack(bg, pad(0, 14, 0, 12, container.NewCenter(row(7, ic, t)))))
}

// buildHome makes the Home tab: this computer's panel, the filters, the
// computers, the version.
func (m *mainWin) buildHome() fyne.CanvasObject {
	p := c()
	thisTitle := text(strings.ToUpper(m.L("thisComputer")), 11, semibold, p.tintText)
	m.code = text("— — —", 40, bold, p.accent)
	m.copied = text(m.L("copied"), 12, semibold, p.ok)
	m.copied.Hide()
	codeTap := newTap(m.code, m.copyCode)
	codeLine := row(12, codeTap, m.copied)
	m.link = text("", 12, regular, p.muted)
	linkTap := newTap(m.link, m.copyLink)
	m.linkRow = row(6, icon("link", 13, p.faint), linkTap)
	hint := wrapText(m.L("hint"), 11.5, p.faint, 560)
	left := container.New(columnLayout{gap: 2, gaps: map[int]float32{1: 6, 2: 8}}, thisTitle, codeLine, m.linkRow, hint)

	m.copyBtn = newButton("", "doc.on.doc", false, m.copyCode)
	m.renewBtn = newButton("", "arrow.clockwise", false, func() { m.h.MainAction("newcode", "") })
	m.permBtn = newButton("", "checkmark.shield", false, func() { m.h.MainAction("allow", "") })
	tools := row(8, m.copyBtn, m.renewBtn, m.permBtn)
	m.statusDot = canvas.NewCircle(p.danger)
	m.status = text("", 12, regular, p.muted)
	statusRow := row(7, sized(8, 8, m.statusDot), m.status)
	right := container.New(columnLayout{gap: 12, right: true}, tools, statusRow)

	panel := container.NewStack(
		box(p.tintBG, 14, p.tintBorder, 1),
		pad(18, 24, 18, 24, container.NewBorder(nil, nil, nil, right, left)),
	)

	// The filters and the view.
	m.filterTabs = map[string]*filterTab{}
	var filters []fyne.CanvasObject
	for _, f := range [][2]string{{"all", m.L("myComputers")}, {"favorites", m.L("favorites")}, {"recents", m.L("recents")}} {
		ft := m.newFilterTab(f[0], f[1])
		m.filterTabs[f[0]] = ft
		filters = append(filters, ft.tap)
	}
	clear := text(m.L("clear"), 12, regular, p.faint)
	m.clearBtn = newTap(clear, func() { m.h.MainAction("clear", "") })
	var viewBtns []fyne.CanvasObject
	for i, sym := range []string{"square.grid.2x2", "rectangle.grid.1x2", "list.bullet"} {
		mode := i
		t, img := iconButton(sym, 15, p.faint, func() { m.pickView(mode) })
		m.viewIcons[i] = img
		viewBtns = append(viewBtns, t)
	}
	views := container.NewStack(box(p.surface, 7, nil, 0), pad(3, 6, 3, 6, row(2, viewBtns...)))
	filterRow := row(18, append(filters, layout.NewSpacer(), m.clearBtn, views)...)

	m.tiles = container.NewWithoutLayout()
	// A pixel above the tiles: Fyne draws a border on its edge, half
	// outside, and the scroll would clip the first row's top line (the
	// Mac draws it inside). The filters give that pixel back.
	m.scroll = container.NewVScroll(pad(1, 28, 12, 28, m.tiles))
	m.version = text("", 11, regular, p.faint)

	header := pad(24, 28, 0, 28, panel)
	filtersPad := pad(26, 28, 13, 28, filterRow)
	versionRow := pad(6, 14, 8, 0, container.NewHBox(layout.NewSpacer(), m.version))
	return container.NewBorder(container.NewVBox(header, filtersPad), versionRow, nil, nil, m.scroll)
}

// filterTab is one of My computers, Favorites, Recent sessions: the
// chosen one bold, underlined in the brand's color.
type filterTab struct {
	tap   *tap
	label *canvas.Text
	line  *canvas.Rectangle
}

func (m *mainWin) newFilterTab(id, title string) *filterTab {
	ft := &filterTab{label: text(title, 14, regular, c().faint)}
	ft.line = canvas.NewRectangle(c().text)
	ft.line.SetMinSize(fyne.NewSize(1, 2))
	ft.tap = newTap(container.New(&underlineLayout{}, ft.label, ft.line), func() {
		m.mu.Lock()
		m.filter, m.chose = id, true
		m.mu.Unlock()
		m.renderFilters()
		m.renderTiles()
	})
	return ft
}

func (m *mainWin) pickView(mode int) {
	m.mu.Lock()
	m.viewMode = mode
	m.mu.Unlock()
	m.app.Preferences().SetInt("org.go-link.desk.view", mode)
	m.renderFilters()
	m.renderTiles()
}

func (m *mainWin) renderFilters() {
	p := c()
	m.mu.Lock()
	filter, mode, recents := m.filter, m.viewMode, len(m.home.Recents)
	m.mu.Unlock()
	for id, ft := range m.filterTabs {
		on := id == filter
		ft.label.Color = p.faint
		ft.label.FontSource = fontRes[regular]
		ft.line.Hidden = !on
		if on {
			ft.label.Color = p.text
			ft.label.FontSource = fontRes[semibold]
		}
		ft.label.Refresh()
		ft.line.Refresh()
		ft.tap.Refresh()
	}
	if filter == "recents" && recents > 0 {
		m.clearBtn.Show()
	} else {
		m.clearBtn.Hide()
	}
	for i, img := range m.viewIcons {
		col := p.faint
		if i == mode {
			col = p.accent
		}
		img.Resource = iconRes(symbolName([]string{"square.grid.2x2", "rectangle.grid.1x2", "list.bullet"}[i]), col)
		img.Refresh()
	}
}

// setState shows Home's state.
func (m *mainWin) setState(h remoteview.Home) {
	p := c()
	m.mu.Lock()
	m.home = h
	if !m.chose && len(h.Devices) == 0 && len(h.Recents) > 0 {
		m.filter = "recents"
	}
	m.mu.Unlock()
	if m.code == nil {
		return
	}
	m.code.Text = h.Code
	if h.Code == "" {
		m.code.Text = "— — —"
	}
	m.code.Refresh()
	m.link.Text = middleTruncate(h.Link, 64)
	m.link.Refresh()
	if h.Link == "" {
		m.linkRow.Hide()
	} else {
		m.linkRow.Show()
	}
	m.status.Text = h.Status
	m.status.Refresh()
	switch {
	case h.InSession:
		m.statusDot.FillColor = p.accent
	case h.StatusOK:
		m.statusDot.FillColor = p.ok
	default:
		m.statusDot.FillColor = p.danger
	}
	m.statusDot.Refresh()
	m.copyBtn.setEnabled(h.Code != "")
	m.renewBtn.setEnabled(h.CanNew)
	if len(h.Perms) == 0 {
		m.permBtn.Hide()
	} else {
		m.permBtn.Show()
		all := true
		for _, pm := range h.Perms {
			all = all && pm.Granted
		}
		if all {
			m.permBtn.symbol, m.permBtn.fill = "checkmark.shield", nil
			m.permBtn.restyle()
			m.permBtn.tint(p.ok)
		} else {
			m.permBtn.symbol, m.permBtn.fill = "exclamationmark.shield", p.tintBG
			m.permBtn.restyle()
			m.permBtn.tint(p.accent)
		}
	}
	m.version.Text = h.Version
	m.version.Refresh()
	m.renderFilters()
	m.renderTiles()
}

func (m *mainWin) copyCode() {
	m.mu.Lock()
	code := m.home.Code
	m.mu.Unlock()
	if code == "" {
		return
	}
	m.h.MainAction("copy", "")
	m.flashCopied()
}

func (m *mainWin) copyLink() {
	m.mu.Lock()
	link := m.home.Link
	m.mu.Unlock()
	if link == "" {
		return
	}
	m.app.Clipboard().SetContent(link)
	m.flashCopied()
}

func (m *mainWin) flashCopied() {
	m.copied.Show()
	if m.copiedOff != nil {
		m.copiedOff.Stop()
	}
	m.copiedOff = time.AfterFunc(1500*time.Millisecond, func() { fyne.Do(m.copied.Hide) })
}

// connect opens a session to the code typed (or a link, with Link).
func (m *mainWin) connect() {
	code := digitsOf(m.address.Text)
	if len(code) != 9 {
		return
	}
	kind := "join"
	if m.doorIndex == 1 {
		kind = "claim"
	}
	m.address.SetText("")
	m.h.MainAction("connect", kind+":"+code)
}

func (m *mainWin) doorTitle() string {
	if m.doorIndex == 1 {
		return m.L("doorLink")
	}
	return m.L("doorSession")
}

func digitsOf(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// formatCode shows what is typed as the code is written, "123 456 789",
// up to 9 digits.
func formatCode(s string) string {
	d := digitsOf(s)
	if len(d) > 9 {
		d = d[:9]
	}
	var b strings.Builder
	for i, r := range d {
		if i > 0 && i%3 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func middleTruncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	half := (max - 1) / 2
	return string(r[:half]) + "…" + string(r[len(r)-half:])
}
