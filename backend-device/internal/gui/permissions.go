// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/permission"
)

// permRow is one permission in the window: its state and a button to ask
// for it.
type permRow struct {
	kind   permission.Kind
	state  *widget.Label
	button *widget.Button
}

// permissionsBox lists what macOS needs the person at the computer to
// allow, and checks again every two seconds until everything is allowed.
type permissionsBox struct {
	box     *fyne.Container
	rows    []*permRow
	restart *widget.Label
	granted func(permission.Kind) bool
	asked   bool
}

func (u *ui) newPermissionsBox(granted func(permission.Kind) bool, request func(permission.Kind) bool) *permissionsBox {
	p := &permissionsBox{granted: granted}
	title := widget.NewLabelWithStyle(L("Permissions"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	items := []fyne.CanvasObject{title}
	for _, k := range []struct {
		kind permission.Kind
		name string
	}{
		{permission.ScreenRecording, L("Screen Recording: to show your screen")},
		{permission.Accessibility, L("Accessibility: to let the viewer use the mouse and keyboard")},
	} {
		r := &permRow{kind: k.kind, state: widget.NewLabel("")}
		r.button = widget.NewButton(L("Allow…"), func() {
			p.asked = true
			request(r.kind)
			p.refresh()
		})
		name := widget.NewLabel(k.name)
		name.Wrapping = fyne.TextWrapWord
		items = append(items, container.NewBorder(nil, nil, nil, container.NewHBox(r.state, r.button), name))
		p.rows = append(p.rows, r)
	}
	p.restart = widget.NewLabel(L("After allowing it in System Settings, quit go-link Desk and open it again."))
	p.restart.Wrapping = fyne.TextWrapWord
	p.restart.Importance = widget.WarningImportance
	items = append(items, p.restart, layout.NewSpacer())
	p.box = container.NewVBox(items...)
	p.refresh()
	return p
}

// refresh shows each permission's state.
func (p *permissionsBox) refresh() {
	all := true
	for _, r := range p.rows {
		if p.granted(r.kind) {
			r.state.SetText(L("Allowed"))
			r.state.Importance = widget.SuccessImportance
			r.button.Hide()
		} else {
			all = false
			r.state.SetText(L("Not allowed"))
			r.state.Importance = widget.WarningImportance
			r.button.Show()
		}
		r.state.Refresh()
	}
	if p.asked && !all {
		p.restart.Show()
	} else {
		p.restart.Hide()
	}
}

// watch checks the permissions again every two seconds.
func (p *permissionsBox) watch(stop <-chan struct{}) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			fyne.Do(p.refresh)
		}
	}
}
