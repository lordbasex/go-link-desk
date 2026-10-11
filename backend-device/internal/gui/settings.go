// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"errors"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/autostart"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/remoteview"
)

// Autostart starts the host at login (Settings). Tests replace it.
type Autostart struct {
	Supported bool
	Enabled   func() bool
	Set       func(on bool) error
}

// systemAutostart writes the system's login entry for this binary.
func systemAutostart() *Autostart {
	return &Autostart{
		Supported: autostart.Supported,
		Enabled:   autostart.Enabled,
		Set: func(on bool) error {
			if !on {
				return autostart.Disable()
			}
			exe, err := autostart.Executable()
			if err != nil {
				return err
			}
			return autostart.Enable(exe)
		},
	}
}

// idleChoices are the idle times Settings offers, in minutes.
var idleChoices = []int{0, 5, 15, 30, 60}

// settingsWindow is Settings: unattended access (off by default, away
// from the main window), the linked browsers, and what happens around a
// session.
type settingsWindow struct {
	u   *ui
	win fyne.Window

	unattended *widget.Check
	details    *fyne.Container // what only shows with unattended access on
	code       *widget.Label
	password   *widget.Label
	newPass    *widget.Entry
	passErr    *widget.Label
	clearPass  *widget.Button
	links      *fyne.Container
	linkIDs    []string // the links shown, to redraw only on change
	startup    *widget.Check
	idle       *widget.Select
	lock       *widget.Check
	ticks      *widget.Select // what a request starts with ticked
	language   *widget.Select
	viewerName *widget.Entry // nil without the Connect side
	tabs       *widget.Check
	setting    bool // render is updating the widgets, not the person
}

func idleLabel(m int) string {
	switch {
	case m == 0:
		return L("Never")
	case m%60 == 0:
		return Lf("After %d h", m/60)
	default:
		return Lf("After %d min", m)
	}
}

func (u *ui) newSettings() *settingsWindow {
	s := &settingsWindow{u: u}
	s.win = u.app.NewWindow(L("Settings"))
	set := u.opts.Settings

	heading := func(text string) *widget.Label {
		return widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	}
	wrap := func(text string) *widget.Label {
		l := widget.NewLabel(text)
		l.Wrapping = fyne.TextWrapWord
		return l
	}

	// Unattended access.
	s.unattended = widget.NewCheck(L("Allow unattended access"), func(on bool) {
		if s.setting {
			return
		}
		_ = set.Update(func(c *models.Config) { c.Unattended.Enabled = on })
	})
	warn := wrap(L("Linked browsers come in without you accepting, with the permissions you gave them. Link only your own browsers."))
	warn.Importance = widget.WarningImportance
	s.code = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	how := wrap(Lf("To link a browser, open %s in it and type this code. You will be asked to accept it.", strings.TrimRight(u.opts.WebURL, "/")+"/link"))
	s.password = wrap("")
	s.newPass = widget.NewPasswordEntry()
	s.newPass.SetPlaceHolder(Lf("New password (%d characters or more)", services.MinPassword))
	s.passErr = widget.NewLabel("")
	s.passErr.Importance = widget.DangerImportance
	s.passErr.Hide()
	setPass := widget.NewButton(L("Set password"), func() { s.setPassword(s.newPass.Text) })
	s.clearPass = widget.NewButton(L("Remove password"), func() {
		_ = set.Update(func(c *models.Config) { c.Unattended.PasswordHash = "" })
	})
	s.links = container.NewVBox()
	s.details = container.NewVBox(
		widget.NewLabel(L("Linking code")), s.code, how,
		widget.NewSeparator(),
		s.password, s.newPass, container.NewHBox(setPass, s.clearPass), s.passErr,
		widget.NewSeparator(),
		heading(L("Linked browsers")), s.links,
	)

	// Around a session.
	start := u.opts.Autostart
	s.startup = widget.NewCheck(L("Open go-link Desk when I log in"), func(on bool) {
		if s.setting {
			return
		}
		if err := start.Set(on); err != nil {
			u.opts.Logger.Warn("cannot change the login item", "err", err)
		}
		s.render()
	})
	if !start.Supported {
		s.startup.Disable()
	}
	labels := make([]string, len(idleChoices))
	for i, m := range idleChoices {
		labels[i] = idleLabel(m)
	}
	s.idle = widget.NewSelect(labels, func(v string) {
		if s.setting {
			return
		}
		if i := slices.Index(labels, v); i >= 0 {
			_ = set.Update(func(c *models.Config) { c.IdleMinutes = idleChoices[i] })
		}
	})
	s.lock = widget.NewCheck(L("Lock the screen when a session ends"), func(on bool) {
		if !s.setting {
			_ = set.Update(func(c *models.Config) { c.LockOnEnd = on })
		}
	})
	ticks := []string{L("Full control"), L("Only the screen")}
	s.ticks = widget.NewSelect(ticks, func(v string) {
		if !s.setting {
			_ = set.Update(func(c *models.Config) {
				c.RequestTicks = ""
				if v == ticks[1] {
					c.RequestTicks = "view"
				}
			})
		}
	})
	ticksNote := wrap(L("Only the screen: whoever asks has to be given each permission by hand."))
	ticksNote.Importance = widget.LowImportance
	langs := make([]string, len(languages))
	for i, l := range languages {
		langs[i] = l.label
		if l.id == "" {
			langs[i] = L("Automatic")
		}
	}
	s.language = widget.NewSelect(langs, func(v string) {
		if s.setting {
			return
		}
		if i := slices.Index(langs, v); i >= 0 {
			_ = set.Update(func(c *models.Config) { c.Language = languages[i].id })
		}
	})
	langNote := wrap(L("The new language shows the next time go-link Desk opens."))
	langNote.Importance = widget.LowImportance

	content := container.NewVBox(
		heading(L("Unattended access")), s.unattended, warn, s.details,
		widget.NewSeparator(),
		heading(L("Sessions")),
		container.NewBorder(nil, nil, widget.NewLabel(L("Disconnect when idle")), nil, s.idle),
		s.lock,
		container.NewBorder(nil, nil, widget.NewLabel(L("A request starts with")), nil, s.ticks), ticksNote,
		widget.NewSeparator(),
	)
	if u.opts.Remotes != nil && remoteview.Supported {
		// Connecting to other computers.
		s.viewerName = widget.NewEntry()
		s.viewerName.SetPlaceHolder(u.viewerName())
		s.viewerName.OnChanged = func(v string) {
			if !s.setting {
				name := strings.TrimSpace(v)
				if len([]rune(name)) > models.MaxKnockName {
					name = string([]rune(name)[:models.MaxKnockName])
				}
				_ = set.Update(func(c *models.Config) { c.ViewerName = name })
			}
		}
		nameNote := wrap(L("Shown to the computer you connect to. Empty: your user's name."))
		nameNote.Importance = widget.LowImportance
		s.tabs = widget.NewCheck(L("Open sessions in tabs"), func(on bool) {
			if !s.setting {
				_ = set.Update(func(c *models.Config) { c.SessionWindows = !on })
			}
		})
		content.Add(heading(L("Connecting to other computers")))
		content.Add(container.NewBorder(nil, nil, widget.NewLabel(L("Your name")), nil, s.viewerName))
		content.Add(nameNote)
		content.Add(s.tabs)
		content.Add(widget.NewSeparator())
	}
	content.Add(container.NewVBox(
		heading(L("This computer")), s.startup,
		container.NewBorder(nil, nil, widget.NewLabel(L("Language")), nil, s.language), langNote,
	))
	s.win.SetContent(container.NewVScroll(container.NewPadded(content)))
	s.win.Resize(fyne.NewSize(480, 620))
	s.win.SetCloseIntercept(s.win.Hide)
	set.OnChange(func(models.Config) { fyne.Do(s.render) })
	s.render()
	return s
}

// setPassword hashes and keeps a new unattended password.
func (s *settingsWindow) setPassword(pw string) {
	hash, err := services.HashPassword(pw)
	if errors.Is(err, services.ErrPasswordLength) {
		s.passErr.SetText(Lf("The password must have %d to %d characters.", services.MinPassword, services.MaxPassword))
		s.passErr.Show()
		return
	}
	if err != nil {
		s.u.opts.Logger.Warn("cannot set the password", "err", err)
		return
	}
	s.passErr.Hide()
	s.newPass.SetText("")
	_ = s.u.opts.Settings.Update(func(c *models.Config) { c.Unattended.PasswordHash = hash })
}

// render shows the settings as they are now.
func (s *settingsWindow) render() {
	cfg := s.u.opts.Settings.Get()
	s.setting = true
	defer func() { s.setting = false }()
	s.unattended.SetChecked(cfg.Unattended.Enabled)
	if cfg.Unattended.Enabled {
		s.details.Show()
	} else {
		s.details.Hide()
	}
	if cfg.Unattended.PasswordHash != "" {
		s.password.SetText(L("Password: set. Linked browsers type it after their link."))
		s.clearPass.Enable()
	} else {
		s.password.SetText(L("No password: linked browsers come in with their link alone."))
		s.clearPass.Disable()
	}
	s.renderCode(s.u.opts.Status.Snapshot())
	s.renderLinks(cfg.Links)
	s.startup.SetChecked(s.u.opts.Autostart.Enabled())
	for i, m := range idleChoices {
		if m == cfg.IdleMinutes {
			s.idle.SetSelectedIndex(i)
		}
	}
	if !slices.Contains(idleChoices, cfg.IdleMinutes) {
		s.idle.ClearSelected()
	}
	s.lock.SetChecked(cfg.LockOnEnd)
	s.ticks.SetSelectedIndex(0)
	if cfg.RequestTicks == "view" {
		s.ticks.SetSelectedIndex(1)
	}
	if s.viewerName != nil {
		// Not while it is being typed in: the change came from it.
		if f, ok := s.win.Canvas().Focused().(*widget.Entry); !ok || f != s.viewerName {
			s.viewerName.SetText(cfg.ViewerName)
		}
		s.tabs.SetChecked(!cfg.SessionWindows)
	}
	for i, l := range languages {
		if l.id == cfg.Language {
			s.language.SetSelectedIndex(i)
		}
	}
}

// renderCode shows the pairing code, which links a browser.
func (s *settingsWindow) renderCode(st models.Status) {
	code := strings.ReplaceAll(st.Pairing.Code, " ", "")
	if code == "" {
		s.code.SetText("— — —")
		return
	}
	s.code.SetText(services.FormatCode(code))
}

// renderLinks lists the linked browsers: name, when linked and last
// used, what each may do (changed here), and Revoke.
func (s *settingsWindow) renderLinks(links []models.Link) {
	s.links.RemoveAll()
	s.linkIDs = s.linkIDs[:0]
	if len(links) == 0 {
		none := widget.NewLabel(L("No browser is linked."))
		none.Importance = widget.LowImportance
		s.links.Add(none)
		return
	}
	for _, l := range links {
		s.linkIDs = append(s.linkIDs, l.ID)
		name := l.Name
		if name == "" {
			name = L("A browser")
		}
		title := widget.NewLabelWithStyle(name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		when := widget.NewLabel(Lf("Linked %s · last used %s", l.CreatedAt.Local().Format("2006-01-02"), l.LastSeen.Local().Format("2006-01-02 15:04")))
		when.Importance = widget.LowImportance
		id := l.ID
		checks := newPermChecks(func(p models.Permissions) {
			if s.u.opts.Links != nil {
				s.u.opts.Links.SetLink(id, func(l *models.Link) { l.Permissions = p })
			}
		})
		checks.set(l.Permissions)
		revoke := widget.NewButtonWithIcon(L("Revoke"), theme.DeleteIcon(), func() {
			s.confirmRevoke(id, name)
		})
		revoke.Importance = widget.DangerImportance
		box := container.NewVBox(container.NewBorder(nil, nil, nil, revoke, title), when)
		for _, o := range checks.objects() {
			box.Add(o)
		}
		s.links.Add(box)
		s.links.Add(widget.NewSeparator())
	}
}

// confirmRevoke asks before revoking a browser. (A window of its own:
// Fyne's dialog package would bring in another dependency.)
func (s *settingsWindow) confirmRevoke(id, name string) {
	w := s.u.app.NewWindow(L("Revoke this browser?"))
	text := widget.NewLabel(Lf("%s will not be able to come in again until it is linked with a new code.", name))
	text.Wrapping = fyne.TextWrapWord
	cancel := widget.NewButton(L("Cancel"), w.Close)
	revoke := widget.NewButtonWithIcon(L("Revoke"), theme.DeleteIcon(), func() {
		w.Close()
		if s.u.opts.Links != nil {
			go s.u.opts.Links.Revoke(id)
		}
	})
	revoke.Importance = widget.DangerImportance
	w.SetContent(container.NewPadded(container.NewVBox(text, container.NewHBox(layout.NewSpacer(), cancel, revoke))))
	w.Resize(fyne.NewSize(360, 0))
	w.Show()
}

// show brings Settings up.
func (s *settingsWindow) show() {
	s.render()
	s.win.Show()
	s.win.RequestFocus()
}
