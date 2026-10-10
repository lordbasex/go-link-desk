// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
)

// sessionPanel is the small window that stays above every other one for
// as long as a session lasts: who is connected, what they can do, and
// Disconnect. It cannot be closed, only ended.
type sessionPanel struct {
	win        fyne.Window
	who        *widget.Label
	browser    *widget.Label
	since      *widget.Label
	perms      *widget.Label
	disconnect *widget.Button
	shown      bool
}

func (u *ui) newSessionPanel() *sessionPanel {
	p := &sessionPanel{}
	p.win = u.app.NewWindow(L("go-link Desk session"))
	p.who = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	p.who.Wrapping = fyne.TextWrapWord
	p.browser = widget.NewLabel("")
	p.since = widget.NewLabel("")
	p.since.Importance = widget.LowImportance
	p.perms = widget.NewLabel("")
	p.disconnect = widget.NewButtonWithIcon(L("Disconnect"), theme.LogoutIcon(), func() {
		p.disconnect.Disable()
		if u.opts.EndSession != nil {
			go u.opts.EndSession() // it waits for the last message to go out
		}
	})
	p.disconnect.Importance = widget.DangerImportance
	p.win.SetContent(container.NewPadded(container.NewVBox(p.who, p.browser, p.since, p.perms, p.disconnect)))
	p.win.Resize(fyne.NewSize(320, 0))
	p.win.SetFixedSize(true)
	// The panel goes away with the session, not before.
	p.win.SetCloseIntercept(func() {})
	return p
}

// render shows the session, or hides the panel when there is none.
func (p *sessionPanel) render(a *models.ActiveSession) {
	if a == nil {
		if p.shown {
			p.win.Hide()
			p.shown = false
		}
		return
	}
	name := a.Name
	if name == "" {
		name = L("Someone")
	}
	p.who.SetText(Lf("%s is viewing your screen", name))
	p.browser.SetText(a.Browser)
	p.since.SetText(Lf("Since %s", a.Since.Local().Format("15:04")))
	text := L("Can:") + " " + L("See your screen")
	p.perms.SetText(text)
	if p.shown {
		return
	}
	p.disconnect.Enable()
	p.win.Show()
	floatWindow(p.win.Title(), false)
	p.shown = true
}
