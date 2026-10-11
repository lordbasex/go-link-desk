// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
)

// permChecks are the boxes of what a viewer may do besides seeing the
// screen, in the connection request and in the session panel. In the
// request they start ticked (fullControl): whoever asks for support should
// not have to tick four boxes; the person at the computer unticks what
// they do not want, there or during the session.
type permChecks struct {
	control   *widget.Check
	audio     *widget.Check
	clipboard *widget.Check
	files     *widget.Check
	setting   bool // set is updating the boxes, not the person
}

// newPermChecks builds the boxes; changed (may be nil) is told when the
// person ticks or unticks one.
func newPermChecks(changed func(models.Permissions)) *permChecks {
	c := &permChecks{}
	on := func(bool) {
		if !c.setting && changed != nil {
			changed(c.value())
		}
	}
	c.control = widget.NewCheck(L("Use your mouse and keyboard"), on)
	c.audio = widget.NewCheck(L("Hear your computer's sound"), on)
	c.clipboard = widget.NewCheck(L("Share the clipboard (copy and paste)"), on)
	c.files = widget.NewCheck(L("Send and receive files"), on)
	return c
}

func (c *permChecks) objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{c.control, c.audio, c.clipboard, c.files}
}

// value is what the boxes allow, seeing the screen always included.
func (c *permChecks) value() models.Permissions {
	return models.Permissions{View: true, Control: c.control.Checked, Audio: c.audio.Checked, Clipboard: c.clipboard.Checked, Files: c.files.Checked}
}

// set shows p without telling anyone.
func (c *permChecks) set(p models.Permissions) {
	c.setting = true
	defer func() { c.setting = false }()
	c.control.SetChecked(p.Control)
	c.audio.SetChecked(p.Audio)
	c.clipboard.SetChecked(p.Clipboard)
	c.files.SetChecked(p.Files)
}

// fullControl is everything a viewer may be given.
var fullControl = models.Permissions{View: true, Control: true, Audio: true, Clipboard: true, Files: true}
