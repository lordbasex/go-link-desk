// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
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
	checks     *permChecks
	transfer   *widget.Label // the file moving now
	chat       *chatBox      // nil without a chat
	peer       string        // the viewer shown, to start its chat empty
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
	p.perms = widget.NewLabel("")
	p.checks = newPermChecks(func(perm models.Permissions) {
		if u.opts.SetPermissions != nil {
			u.opts.SetPermissions(perm)
		}
	})
	p.disconnect = widget.NewButtonWithIcon(L("Disconnect"), theme.LogoutIcon(), func() {
		p.disconnect.Disable()
		if u.opts.EndSession != nil {
			go u.opts.EndSession() // it waits for the last message to go out
		}
	})
	p.disconnect.Importance = widget.DangerImportance
	p.transfer = widget.NewLabel("")
	p.transfer.Wrapping = fyne.TextWrapWord
	p.transfer.Hide()
	if u.opts.Files != nil {
		u.opts.Files.OnChange(func(t services.Transfer) { fyne.Do(func() { p.showTransfer(t) }) })
	}
	items := append([]fyne.CanvasObject{p.who, p.browser, p.since, p.perms}, p.checks.objects()...)
	items = append(items, p.transfer)
	top := container.NewVBox(items...)
	if u.opts.Chat != nil {
		p.chat = newChatBox(u.opts.Chat)
		u.opts.Chat.OnMessage(func(m services.ChatMessage) {
			// Never take the focus: the viewer may be typing into another app.
			fyne.Do(func() { p.chat.add(m) })
		})
		p.win.SetContent(container.NewPadded(container.NewBorder(top, p.disconnect, nil, nil, p.chat.box)))
	} else {
		p.win.SetContent(container.NewPadded(container.NewVBox(top, p.disconnect)))
	}
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
	if a.Unattended {
		p.who.SetText(Lf("%s is viewing your screen (unattended access)", name))
	}
	p.browser.SetText(a.Browser)
	p.since.SetText(Lf("Since %s", a.Since.Local().Format("15:04")))
	p.perms.SetText(L("Can see your screen, and:"))
	p.checks.set(a.Permissions)
	if p.chat != nil && p.peer != a.PeerID {
		p.chat.clear()
	}
	p.peer = a.PeerID
	if p.shown {
		return
	}
	p.disconnect.Enable()
	p.win.Show()
	floatWindow(p.win.Title(), false)
	p.shown = true
}

// showTransfer shows the file moving now, and how it ended.
func (p *sessionPanel) showTransfer(t services.Transfer) {
	pct := 0
	if t.Size > 0 {
		pct = int(t.Done * 100 / t.Size)
	}
	switch {
	case t.Over && t.Err != nil:
		p.transfer.SetText(Lf("%s: stopped", t.Name))
	case t.Over && t.Upload:
		p.transfer.SetText(Lf("Received %s, in Downloads › go-link Desk", t.Name))
	case t.Over:
		p.transfer.SetText(Lf("Sent %s", t.Name))
	case t.Upload:
		p.transfer.SetText(Lf("Receiving %s… %d%%", t.Name, pct))
	default:
		p.transfer.SetText(Lf("Sending %s… %d%%", t.Name, pct))
	}
	p.transfer.Show()
}
