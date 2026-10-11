// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"net/url"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/remoteview"
)

// aboutText says what go-link Desk is, under its name and version.
func (u *ui) aboutText() string {
	return L("Remote desktop over a direct connection, encrypted end to end.") + "\n" +
		L("Free and open source software (MIT license).") + "\n\n" +
		strings.TrimRight(u.opts.WebURL, "/")
}

// version is the app's version as shown ("v1.2.3").
func (u *ui) version() string {
	return "v" + strings.TrimPrefix(u.opts.Version, "v")
}

// showAbout shows the About panel: the system's on macOS, a small window
// elsewhere.
func (u *ui) showAbout() {
	if u.native {
		remoteview.ShowAbout(u.version(), u.aboutText())
		return
	}
	if u.about == nil {
		w := u.app.NewWindow(L("About go-link Desk"))
		choice := ""
		if u.opts.Settings != nil {
			choice = u.opts.Settings.Get().AppIcon
		}
		icon := canvas.NewImageFromResource(fyne.NewStaticResource("icon.png", appIcon(choice)))
		icon.FillMode = canvas.ImageFillContain
		icon.SetMinSize(fyne.NewSize(96, 96))
		name := widget.NewLabelWithStyle("go-link Desk", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		ver := widget.NewLabelWithStyle(u.version(), fyne.TextAlignCenter, fyne.TextStyle{})
		text := widget.NewLabelWithStyle(L("Remote desktop over a direct connection, encrypted end to end.")+"\n"+
			L("Free and open source software (MIT license)."), fyne.TextAlignCenter, fyne.TextStyle{})
		text.Wrapping = fyne.TextWrapWord
		items := []fyne.CanvasObject{icon, name, ver, text}
		if web, err := url.Parse(strings.TrimRight(u.opts.WebURL, "/")); err == nil && web.Host != "" {
			items = append(items, container.NewCenter(widget.NewHyperlink(web.Host, web)))
		}
		copyright := widget.NewLabelWithStyle("© 2026 Federico Pereira", fyne.TextAlignCenter, fyne.TextStyle{})
		copyright.Importance = widget.LowImportance
		items = append(items, copyright)
		w.SetContent(container.NewPadded(container.NewVBox(items...)))
		w.Resize(fyne.NewSize(340, 0))
		w.SetFixedSize(true)
		w.SetCloseIntercept(w.Hide)
		u.about = w
	}
	u.about.Show()
	u.about.RequestFocus()
}
