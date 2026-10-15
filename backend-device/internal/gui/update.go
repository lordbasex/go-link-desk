// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
	"github.com/lordbasex/go-link-desk/backend-device/internal/updater"
)

// updates looks for a newer go-link Desk now and then (and when asked),
// says so in a window of its own, and installs it: the download is
// checked against the release's signed list, the new version takes this
// one's place and opens.
type updates struct {
	u     *ui
	check *services.UpdateService
	up    *updater.Updater

	mu      sync.Mutex
	known   models.UpdateInfo // the newest release found (Latest "" when none)
	shown   string            // the release whose window opened on its own
	pending bool              // found during a session: shown when it ends
	busy    bool              // downloading or installing
	cancel  context.CancelFunc

	win         fyne.Window
	title, text *widget.Label
	bar         *widget.ProgressBar
	spin        *widget.ProgressBarInfinite
	buttons     *fyne.Container
	trayItem    *fyne.MenuItem
}

// checkTimeout bounds "Check for Updates…".
const checkTimeout = 20 * time.Second

func (u *ui) newUpdates() *updates {
	p := &updates{u: u, up: updater.New()}
	p.check = services.NewUpdateService(services.UpdateConfig{
		Current:  u.opts.Version,
		Logger:   u.opts.Logger,
		OnUpdate: p.found,
		Enabled: func() bool {
			return u.opts.Settings == nil || !u.opts.Settings.Get().NoUpdateCheck
		},
	})
	return p
}

// start checks in the background until the app quits.
func (p *updates) start() { go p.check.Run(p.u.opts.Ctx) }

// found: the periodic check saw a newer release. Its window opens once,
// not during a session (then when it ends).
func (p *updates) found(info models.UpdateInfo) {
	p.mu.Lock()
	p.known = info
	show := info.Latest != p.shown && !p.busy
	if show {
		p.shown = info.Latest
	}
	inSession := activeOf(p.u.opts.Status.Snapshot()) != nil
	if show && inSession {
		p.pending, show = true, false
	}
	p.mu.Unlock()
	fyne.Do(func() {
		p.renderTray()
		if show {
			p.showAvailable()
		}
	})
}

// sessionEnded shows an update found during the session.
func (p *updates) sessionEnded() {
	p.mu.Lock()
	show := p.pending && p.known.Latest != "" && !p.busy
	p.pending = false
	p.mu.Unlock()
	if show {
		p.showAvailable()
	}
}

// ask is "Check for Updates…": it says what it found either way.
func (p *updates) ask() {
	p.mu.Lock()
	busy, known := p.busy, p.known
	p.mu.Unlock()
	if busy {
		p.window().Show()
		p.win.RequestFocus()
		return
	}
	if known.Latest != "" {
		p.showAvailable()
		return
	}
	p.render(L("Checking for updates…"), "", true)
	p.setButtons(widget.NewButton(L("Close"), p.win.Hide))
	go func() {
		ctx, cancel := context.WithTimeout(p.u.opts.Ctx, checkTimeout)
		defer cancel()
		info, err := p.check.Check(ctx)
		fyne.Do(func() {
			switch {
			case err != nil:
				p.render(L("Could not check for updates"), L("There is no connection to GitHub, where the versions are. Try again later."), false)
				p.setButtons(widget.NewButton(L("Close"), p.win.Hide))
			case info.Latest == "":
				p.render(L("go-link Desk is up to date"), Lf("You have %s, the newest version.", bare(p.u.version())), false)
				p.setButtons(widget.NewButton(L("OK"), p.win.Hide))
			default:
				p.mu.Lock()
				p.known, p.shown = info, info.Latest
				p.mu.Unlock()
				p.renderTray()
				p.showAvailable()
			}
		})
	}()
}

// showAvailable shows the newest release with Update.
func (p *updates) showAvailable() {
	p.mu.Lock()
	info := p.known
	p.mu.Unlock()
	if info.Latest == "" {
		return
	}
	text := Lf("You have %s.", bare(p.u.version()))
	if !p.up.CanInstall() {
		text += " " + L("This copy cannot replace itself: download the new one from its page and install it as before.")
	} else if activeOf(p.u.opts.Status.Snapshot()) != nil {
		text += " " + L("Updating ends the session in progress.")
	}
	p.render(Lf("go-link Desk %s is available", bare(info.Latest)), text, false)
	news := widget.NewButton(L("What's new"), func() { p.openPage(info.URL) })
	later := widget.NewButton(L("Later"), p.win.Hide)
	if !p.up.CanInstall() {
		get := widget.NewButton(L("Download"), func() { p.openPage(info.URL); p.win.Hide() })
		get.Importance = widget.HighImportance
		p.setButtons(news, later, get)
		return
	}
	update := widget.NewButton(L("Update"), func() { p.install(info) })
	update.Importance = widget.HighImportance
	p.setButtons(news, later, update)
}

// install downloads, checks and installs the release, and quits for the
// new version to open.
func (p *updates) install(info models.UpdateInfo) {
	ctx, cancel := context.WithCancel(p.u.opts.Ctx)
	p.mu.Lock()
	p.busy, p.cancel = true, cancel
	p.mu.Unlock()
	p.render(Lf("Downloading go-link Desk %s…", bare(info.Latest)), "", false)
	p.bar.Show()
	p.bar.SetValue(0)
	p.setButtons(widget.NewButton(L("Cancel"), cancel))
	done := func() {
		p.mu.Lock()
		p.busy, p.cancel = false, nil
		p.mu.Unlock()
		cancel()
	}
	go func() {
		path, err := p.up.Download(ctx, info.Latest, func(n, total int64) {
			if total > 0 {
				fyne.Do(func() { p.bar.SetValue(float64(n) / float64(total)) })
			}
		})
		if err != nil {
			done()
			fyne.Do(func() { p.failed(info, err) })
			return
		}
		fyne.Do(func() {
			p.render(L("Installing…"), L("go-link Desk opens again by itself."), true)
			p.setButtons()
		})
		restart, err := p.up.Install(ctx, path)
		if err != nil {
			done()
			fyne.Do(func() { p.failed(info, err) })
			return
		}
		p.u.opts.Logger.Info("update installed", "from", p.u.opts.Version, "to", info.Latest)
		if restart != nil {
			if err := restart(); err != nil {
				p.u.opts.Logger.Warn("cannot open the new version", "err", err)
			}
		}
		if p.u.opts.Quit != nil {
			p.u.opts.Quit()
		} else {
			fyne.Do(p.u.app.Quit)
		}
	}()
}

// failed says why the update did not go in, with the release's page.
func (p *updates) failed(info models.UpdateInfo, err error) {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, updater.ErrCancelled):
		p.showAvailable()
		return
	case errors.Is(err, updater.ErrSignature):
		p.render(L("The update was not installed"), L("The download is not the one go-link Desk published (its signature or its contents differ), so it was thrown away."), false)
	case errors.Is(err, updater.ErrManual):
		p.render(L("The update was not installed"), L("This copy cannot replace itself: download the new one from its page and install it as before."), false)
	default:
		p.render(L("The update was not installed"), L("Something went wrong while downloading or installing it. Try again, or download it from its page."), false)
	}
	p.u.opts.Logger.Warn("update failed", "to", info.Latest, "err", err)
	page := widget.NewButton(L("Open its page"), func() { p.openPage(info.URL); p.win.Hide() })
	// Close where Update was: a second click only closes.
	p.setButtons(page, widget.NewButton(L("Close"), p.win.Hide))
}

// window builds the update window once.
func (p *updates) window() fyne.Window {
	if p.win != nil {
		return p.win
	}
	w := p.u.app.NewWindow(L("Software update"))
	p.title = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	p.text = widget.NewLabel("")
	p.text.Wrapping = fyne.TextWrapWord
	p.bar = widget.NewProgressBar()
	p.spin = widget.NewProgressBarInfinite()
	p.buttons = container.NewHBox()
	w.SetContent(container.NewPadded(container.NewVBox(p.title, p.text, p.bar, p.spin, container.NewHBox(layout.NewSpacer(), p.buttons))))
	w.Resize(fyne.NewSize(440, 0))
	w.SetFixedSize(true)
	w.SetCloseIntercept(func() {
		p.mu.Lock()
		busy := p.busy
		p.mu.Unlock()
		if !busy {
			w.Hide()
		}
	})
	p.win = w
	return w
}

// render puts a state in the window and shows it.
func (p *updates) render(title, text string, spinning bool) {
	w := p.window()
	p.title.SetText(title)
	p.text.SetText(text)
	if text == "" {
		p.text.Hide()
	} else {
		p.text.Show()
	}
	p.bar.Hide()
	if spinning {
		p.spin.Show()
		p.spin.Start()
	} else {
		p.spin.Stop()
		p.spin.Hide()
	}
	w.Show()
	styleWindow(w)
	w.CenterOnScreen()
	w.RequestFocus()
}

func (p *updates) setButtons(b ...fyne.CanvasObject) {
	p.buttons.Objects = b
	p.buttons.Refresh()
}

func (p *updates) openPage(link string) {
	if u, err := url.Parse(link); err == nil && u.Scheme == "https" {
		_ = p.u.app.OpenURL(u)
	}
}

// trayLabel is the tray's item: "Update to 1.3.1…" once one is known.
func (p *updates) trayLabel() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.known.Latest != "" {
		return Lf("Update to %s…", bare(p.known.Latest))
	}
	return L("Check for Updates…")
}

func (p *updates) renderTray() {
	if p.trayItem == nil {
		return
	}
	p.trayItem.Label = p.trayLabel()
	p.u.trayMenu.Refresh()
}

// bare is a version without its v: 1.3.1.
func bare(v string) string { return strings.TrimPrefix(v, "v") }
