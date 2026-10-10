// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

// Package gui is the host's window and tray icon (Fyne).
package gui

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/trayicon"
)

// appID is the bundle identifier, also the key of Fyne's preferences.
const appID = "org.go-link.desk"

// Options is what the window needs from the host.
type Options struct {
	Ctx     context.Context
	Quit    func() // stops the host
	Status  *services.StatusService
	NewCode func() // replaces the session code
	WebURL  string // where viewers type the code
	// Language is the window's language (en, es, pt or "" for the
	// computer's).
	Language string
	Version  string
	Logger   *slog.Logger
}

type ui struct {
	opts Options
	app  fyne.App
	win  fyne.Window

	code     *canvas.Text
	link     *widget.Label
	state    *widget.Label
	copyBtn  *widget.Button
	newBtn   *widget.Button
	trayCode *fyne.MenuItem
	trayMenu *fyne.Menu
	current  string // the code on screen, without spaces
}

// Run shows the window and the tray icon and blocks until the host stops.
func Run(opts Options) {
	a := app.NewWithID(appID)
	a.SetIcon(fyne.NewStaticResource("go-link-desk.png", trayicon.AppPNG()))
	u := newUI(a, opts)
	u.setupTray()
	u.render(opts.Status.Snapshot())
	opts.Status.OnChange(func(st models.Status) {
		fyne.Do(func() { u.render(st) })
	})
	// stopped closes when the app loop ends: after that any Fyne call
	// panics, so the goroutine below must not touch the app.
	stopped := make(chan struct{})
	go func() {
		defer ignoreAfterStop()
		select {
		case <-u.opts.Ctx.Done():
			fyne.Do(a.Quit)
		case <-stopped:
		}
	}()
	u.win.Show()
	a.Run()
	close(stopped)
	if opts.Quit != nil {
		opts.Quit()
	}
}

// ignoreAfterStop swallows the panic of a Fyne call that raced with the
// end of the app loop (the window is already gone, nothing is lost).
func ignoreAfterStop() {
	_ = recover()
}

// newUI builds the window without showing it (tests use a test app).
func newUI(a fyne.App, opts Options) *ui {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Ctx == nil {
		opts.Ctx = context.Background()
	}
	if opts.WebURL == "" {
		opts.WebURL = models.DefaultWebURL
	}
	setLanguage(opts.Language)
	u := &ui{opts: opts, app: a}
	u.win = a.NewWindow("go-link Desk")

	title := widget.NewLabelWithStyle(L("Your code"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	u.code = canvas.NewText("— — —", theme.Color(theme.ColorNameForeground))
	u.code.TextSize = 44
	u.code.TextStyle = fyne.TextStyle{Monospace: true}
	u.code.Alignment = fyne.TextAlignCenter
	u.link = widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{})
	u.link.Wrapping = fyne.TextWrapWord
	u.state = widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})
	u.copyBtn = widget.NewButtonWithIcon(L("Copy"), theme.ContentCopyIcon(), u.copyCode)
	u.newBtn = widget.NewButtonWithIcon(L("New code"), theme.ViewRefreshIcon(), func() {
		if u.opts.NewCode != nil {
			u.opts.NewCode()
		}
	})
	hint := widget.NewLabelWithStyle(L("Share this code only with someone you know. Nobody legitimate will call you asking for access to your computer."), fyne.TextAlignCenter, fyne.TextStyle{})
	hint.Wrapping = fyne.TextWrapWord
	version := widget.NewLabelWithStyle("v"+strings.TrimPrefix(opts.Version, "v"), fyne.TextAlignTrailing, fyne.TextStyle{})

	u.win.SetContent(container.NewPadded(container.NewVBox(
		title,
		u.code,
		u.link,
		container.NewCenter(container.NewHBox(u.copyBtn, u.newBtn)),
		widget.NewSeparator(),
		hint,
		u.state,
		version,
	)))
	u.win.Resize(fyne.NewSize(420, 360))
	return u
}

// render shows the session code and the link with signalhub.
func (u *ui) render(st models.Status) {
	code := ""
	if st.Session != nil {
		code = st.Session.Code
	}
	u.current = code
	switch {
	case code != "":
		u.code.Text = services.FormatCode(code)
		u.link.SetText(Lf("Or open %s", strings.TrimRight(u.opts.WebURL, "/")+"/c/"+st.Session.Invite))
		u.copyBtn.Enable()
	default:
		u.code.Text = "— — —"
		u.link.SetText("")
		u.copyBtn.Disable()
	}
	u.code.Refresh()
	if st.Session == nil {
		u.newBtn.Disable()
	} else {
		u.newBtn.Enable()
	}
	switch st.Signal.State {
	case models.SignalConnected:
		u.state.SetText(L("Connected"))
	case models.SignalDisconnected:
		u.state.SetText(L("Offline, reconnecting…"))
	default:
		u.state.SetText(L("Connecting…"))
	}
	if u.trayCode != nil {
		u.trayCode.Label = Lf("Code: %s", services.FormatCode(code))
		if code == "" {
			u.trayCode.Label = L("No code yet")
		}
		u.trayMenu.Refresh()
	}
}

func (u *ui) copyCode() {
	if u.current == "" {
		return
	}
	u.app.Clipboard().SetContent(services.FormatCode(u.current))
	u.copyBtn.SetText(L("Copied"))
	go func() {
		time.Sleep(1500 * time.Millisecond)
		fyne.Do(func() { u.copyBtn.SetText(L("Copy")) })
	}()
}

// setupTray adds the tray icon and its menu. Closing the window only
// hides it; the host keeps running until Quit.
func (u *ui) setupTray() {
	desk, ok := u.app.(desktop.App)
	if !ok {
		return
	}
	u.trayCode = fyne.NewMenuItem(L("No code yet"), nil)
	u.trayCode.Disabled = true
	show := fyne.NewMenuItem(L("Open go-link Desk"), func() {
		u.win.Show()
		u.win.RequestFocus()
	})
	u.trayMenu = fyne.NewMenu("go-link Desk", u.trayCode, fyne.NewMenuItemSeparator(), show)
	desk.SetSystemTrayMenu(u.trayMenu)
	desk.SetSystemTrayIcon(fyne.NewStaticResource("tray.png", trayicon.PNG()))
	u.win.SetCloseIntercept(u.win.Hide)
}
