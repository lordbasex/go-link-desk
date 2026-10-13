// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

// Package gui is the host's window and tray icon (Fyne).
package gui

import (
	"context"
	screencap "github.com/lordbasex/go-link-desk/backend-device/pkg/capture"
	"log/slog"
	"os"
	"strings"
	"sync"
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
	"github.com/lordbasex/go-link-desk/backend-device/internal/viewer"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/permission"
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
	// EndSession disconnects the viewer in session.
	EndSession func()
	// SetPermissions changes what the viewer in session may do.
	SetPermissions func(models.Permissions)
	// Chat is the session's chat, shown in the session panel (nil: none).
	Chat *services.ChatService
	// Files are the session's file transfers, shown in the session panel.
	Files *services.FilesService
	// Call is the session's video call: its window (nil: none).
	Call *services.CallService
	// AskCalls hands the window's "wants a video call" question to the
	// call (false: the call keeps its own, the tests' auto-accept).
	AskCalls bool
	// SetConsent hands the window's connection requests to the session
	// (nil: the session keeps its own, the tests' auto-accept).
	SetConsent func(services.Consent)
	// Settings and Links are what the Settings window changes (nil: no
	// Settings).
	Settings *services.SettingsService
	Links    *services.LinkService
	// Remotes are the computers this app connects to (nil: the window
	// has no Connect side), through SignalURL.
	Remotes   *services.RemotesService
	SignalURL string
	// Connect opens sessions to these session codes at start (separated
	// by commas).
	Connect string
	// Woken receives when the app is opened again while it runs (Windows):
	// the main window comes forward.
	Woken <-chan struct{}
	// Autostart replaces the system's login item (tests).
	Autostart *Autostart
	// Permissions replaces the system's checks (tests).
	Permissions *PermissionChecks
	WebURL      string // where viewers type the code
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
	trayEnd  *fyne.MenuItem
	trayMenu *fyne.Menu
	trayLive bool   // the tray shows the red icon
	trayDark bool   // the menu bar icon's dark variant (Settings)
	current  string // the code on screen, without spaces
	panel    *sessionPanel
	perms    *permissionsBox // nil where the system asks for nothing
	copied   *time.Timer     // puts "Copy" back on the button
	call     *callWindow     // nil without a call
	settings *settingsWindow // nil without Settings
	about    fyne.Window     // the About window, off macOS
	// native: the main window is the system's own (Home and the
	// sessions' tabs), not win.
	native   bool
	lastSt   models.Status
	homeMu   sync.Mutex
	homeLast string // the last Home sent, to send only changes
	prefs    prefsState

	remotesMu sync.Mutex
	remotes   []*remote // sessions to other computers
}

// Run shows the window and the tray icon and blocks until the host stops.
func Run(opts Options) {
	// On X11 the windows' class is what ties them to the app's desktop
	// file (StartupWMClass); GLFW takes it from RESOURCE_NAME, else each
	// window's title.
	if os.Getenv("RESOURCE_NAME") == "" {
		_ = os.Setenv("RESOURCE_NAME", appID)
	}
	a := app.NewWithID(appID)
	useAppTheme(a) // before anything measures text
	icon := trayicon.AppPNG()
	if opts.Settings != nil {
		cfg := opts.Settings.Get()
		icon = appIcon(cfg.AppIcon)
	}
	a.SetIcon(fyne.NewStaticResource("go-link-desk.png", icon))
	u := newUI(a, opts)
	if opts.Remotes != nil && hasMain {
		u.openMain()
	}
	u.setupTray()
	u.render(opts.Status.Snapshot())
	if opts.Woken != nil {
		go func() {
			for {
				select {
				case <-opts.Ctx.Done():
					return
				case <-opts.Woken:
					fyne.Do(u.showMain)
				}
			}
		}()
	}
	if opts.SetConsent != nil {
		opts.SetConsent(u) // the request window asks from now on
		defer opts.SetConsent(nil)
	}
	if opts.Call != nil {
		u.call = u.newCallWindow(opts.Call)
		if opts.AskCalls {
			opts.Call.SetConsent(u)
			defer opts.Call.SetConsent(nil)
		}
		opts.Call.OnStatus(func(st services.CallStatus) {
			fyne.Do(func() { u.call.render(st) })
		})
	}
	stopPerms := make(chan struct{})
	defer close(stopPerms)
	if u.perms != nil {
		go u.perms.watch(stopPerms)
	}
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
	if u.native {
		mainShow()
		for _, c := range strings.Split(opts.Connect, ",") {
			if code := digits(c); len(code) == 9 {
				go u.connect(viewer.Door{Kind: viewer.DoorJoin, Code: code})
			}
		}
	} else {
		u.win.Show()
	}
	a.Run()
	close(stopped)
	u.closeRemotes()
	if opts.Quit != nil {
		opts.Quit()
	}
}

// PermissionChecks stand in for the system's permission checks (tests).
type PermissionChecks struct {
	Granted func(permission.Kind) bool
	Request func(permission.Kind) bool
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
	if opts.Autostart == nil {
		opts.Autostart = systemAutostart()
	}
	setLanguage(opts.Language)
	u := &ui{opts: opts, app: a}
	if opts.Settings != nil {
		u.trayDark = opts.Settings.Get().TrayIcon == "dark"
	}
	u.win = a.NewWindow("go-link Desk")

	title := widget.NewLabelWithStyle(L("Your code"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	u.code = canvas.NewText("— — —", theme.Color(theme.ColorNameForeground))
	u.code.TextSize = 44
	u.code.TextStyle = fyne.TextStyle{Monospace: true}
	// A light or dark system theme changes the code's color with it.
	u.app.Settings().AddListener(func(fyne.Settings) {
		fyne.Do(func() {
			u.code.Color = theme.Color(theme.ColorNameForeground)
			u.code.Refresh()
		})
	})
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

	buttons := container.NewHBox(u.copyBtn, u.newBtn)
	if opts.Settings != nil {
		buttons.Add(widget.NewButtonWithIcon(L("Settings"), theme.SettingsIcon(), u.showSettings))
	}
	items := []fyne.CanvasObject{
		title,
		u.code,
		u.link,
		container.NewCenter(buttons),
		widget.NewSeparator(),
		hint,
		u.state,
	}
	// A Linux Wayland desktop without the remote desktop portal (or
	// PipeWire): its screen cannot be shared; say how to make it work.
	if screencap.Problem() == screencap.ProblemWayland {
		w := widget.NewLabelWithStyle(L("This Wayland desktop cannot share its screen (it lacks the remote desktop portal or PipeWire): sessions show a test card. Log out and log in again with an Xorg session (the gear on the login screen), or use GNOME or KDE Plasma."), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		w.Wrapping = fyne.TextWrapWord
		w.Importance = widget.WarningImportance
		items = append(items, widget.NewSeparator(), w)
	}
	if permission.Needed || opts.Permissions != nil {
		granted, request := permission.Granted, permission.Request
		if opts.Permissions != nil {
			granted, request = opts.Permissions.Granted, opts.Permissions.Request
		}
		u.perms = u.newPermissionsBox(granted, request)
		items = append(items, widget.NewSeparator(), u.perms.box)
	}
	items = append(items, version)
	u.win.SetContent(container.NewPadded(container.NewVBox(items...)))
	u.win.Resize(fyne.NewSize(460, 360))
	u.panel = u.newSessionPanel()
	if opts.Settings != nil {
		u.settings = u.newSettings()
	}
	return u
}

// showSettings opens Settings.
func (u *ui) showSettings() {
	if u.native && u.opts.Settings != nil {
		u.showPrefs()
		return
	}
	if u.settings != nil {
		u.settings.show()
	}
}

// render shows the session code and the link with signalhub.
func (u *ui) render(st models.Status) {
	u.lastSt = st
	if u.native {
		u.pushHome()
	}
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
	// The theme's text color now (light or dark, as the system is).
	u.code.Color = theme.Color(theme.ColorNameForeground)
	u.code.Refresh()
	if st.Session == nil {
		u.newBtn.Disable()
	} else {
		u.newBtn.Enable()
	}
	var active *models.ActiveSession
	if st.Session != nil {
		active = st.Session.Active
	}
	u.panel.render(active)
	if u.settings != nil {
		u.settings.renderCode(st)
	}
	switch {
	case active != nil:
		name := active.Name
		if name == "" {
			name = L("Someone")
		}
		u.state.SetText(Lf("In session with %s", name))
	case st.Signal.State == models.SignalConnected:
		u.state.SetText(L("Connected"))
	case st.Signal.State == models.SignalDisconnected:
		u.state.SetText(L("Offline, reconnecting…"))
	default:
		u.state.SetText(L("Connecting…"))
	}
	if u.trayCode != nil {
		u.trayCode.Label = Lf("Code: %s", services.FormatCode(code))
		if code == "" {
			u.trayCode.Label = L("No code yet")
		}
		u.renderTray(active)
	}
}

// renderTray turns the tray icon red while a session is on, with a
// Disconnect item.
func (u *ui) renderTray(active *models.ActiveSession) {
	live := active != nil
	u.trayEnd.Disabled = !live
	u.trayEnd.Label = L("Disconnect")
	if live && active.Name != "" {
		u.trayEnd.Label = Lf("Disconnect %s", active.Name)
	}
	u.trayMenu.Refresh()
	if live == u.trayLive {
		return
	}
	u.trayLive = live
	if desk, ok := u.app.(desktop.App); ok {
		icon := u.trayPNG()
		if live {
			icon = trayicon.LivePNG()
		}
		desk.SetSystemTrayIcon(fyne.NewStaticResource("tray.png", icon))
	}
}

func (u *ui) copyCode() {
	if u.current == "" {
		return
	}
	u.app.Clipboard().SetContent(services.FormatCode(u.current))
	u.copyBtn.SetText(L("Copied"))
	if u.copied != nil {
		u.copied.Stop()
	}
	u.copied = time.AfterFunc(1500*time.Millisecond, func() {
		fyne.Do(func() { u.copyBtn.SetText(L("Copy")) })
	})
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
	show := fyne.NewMenuItem(L("Open go-link Desk"), u.showMain)
	u.trayEnd = fyne.NewMenuItem(L("Disconnect"), func() {
		if u.opts.EndSession != nil {
			go u.opts.EndSession()
		}
	})
	u.trayEnd.Disabled = true
	u.trayMenu = fyne.NewMenu("go-link Desk", u.trayCode, u.trayEnd, fyne.NewMenuItemSeparator(), show)
	if u.settings != nil {
		u.trayMenu.Items = append(u.trayMenu.Items, fyne.NewMenuItem(L("Settings…"), u.showSettings))
	}
	u.trayMenu.Items = append(u.trayMenu.Items, fyne.NewMenuItem(L("About go-link Desk"), u.showAbout))
	desk.SetSystemTrayMenu(u.trayMenu)
	desk.SetSystemTrayIcon(fyne.NewStaticResource("tray.png", u.trayPNG()))
	u.win.SetCloseIntercept(u.win.Hide)
}

// showMain brings the main window to the front.
func (u *ui) showMain() {
	if u.native {
		mainShow()
		return
	}
	u.win.Show()
	u.win.RequestFocus()
}

// trayPNG is the menu bar icon chosen in Settings (when nobody is
// connected).
func (u *ui) trayPNG() []byte {
	if u.trayDark {
		return trayicon.DarkPNG()
	}
	return trayicon.PNG()
}

// activeOf is the session someone has with this computer, if any.
func activeOf(st models.Status) *models.ActiveSession {
	if st.Session == nil {
		return nil
	}
	return st.Session.Active
}
