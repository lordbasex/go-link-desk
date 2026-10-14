// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/netprobe"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/permission"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/remoteview"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/signalclient"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/trayicon"
)

// Settings in the system's own window (macOS): the same settings as the
// Fyne window, as pages of rows. prefs builds the pages from the settings
// as they are; setting takes a change.

// prefsState is what the native Settings keeps between renders.
type prefsState struct {
	open      bool   // shown at least once: keep it up to date
	passErr   string // the last password refused
	passOK    string // the last password set
	serverErr string // the last server address refused
	netErr    string // the last port or address refused
	// start is the configuration this run started with: what a change of
	// server or port needs a new start for is told against it.
	start    models.Config
	startSet bool
	last     string // the last pages sent, to send only changes
}

// showPrefs brings the native Settings up.
func (u *ui) showPrefs() {
	u.prefs.open = true
	spec := u.prefsSpec()
	b, _ := json.Marshal(spec)
	u.prefs.last = string(b)
	mainSetPrefs(spec, true)
}

// refreshPrefs redraws the native Settings if it was opened and something
// changed (on Fyne's goroutine).
func (u *ui) refreshPrefs() {
	if !u.native || !u.prefs.open {
		return
	}
	spec := u.prefsSpec()
	b, _ := json.Marshal(spec)
	if string(b) == u.prefs.last {
		return
	}
	u.prefs.last = string(b)
	mainSetPrefs(spec, false)
}

func off() *bool { f := false; return &f }

func (u *ui) prefsSpec() remoteview.Prefs {
	cfg := u.opts.Settings.Get()
	st := u.lastSt
	var pages []remoteview.PrefPage

	// General.
	server := L("Connecting…")
	tint := "warn"
	switch st.Signal.State {
	case models.SignalConnected:
		server, tint = L("Connected"), "ok"
	case models.SignalDisconnected:
		server = L("Offline, reconnecting…")
	}
	host, _ := os.Hostname()
	langs := []remoteview.PrefOption{}
	for _, l := range languages {
		title := l.label
		if l.id == "" {
			title = L("Automatic")
		}
		langs = append(langs, remoteview.PrefOption{ID: l.id, Title: title})
	}
	general := remoteview.PrefPage{ID: "general", Title: L("General"), Symbol: "gearshape", Sections: []remoteview.PrefSection{
		{Title: L("State"), Rows: []remoteview.PrefRow{
			{Kind: "info", Label: L("Version"), Value: "v" + strings.TrimPrefix(u.opts.Version, "v")},
			{Kind: "info", Label: L("This computer"), Value: host},
			{Kind: "info", Label: L("go-link server"), Value: server, Tint: tint},
		}},
		{Title: L("Appearance"), Rows: []remoteview.PrefRow{
			{ID: "theme", Kind: "select", Label: L("Theme"), Value: cfg.Theme,
				Options: []remoteview.PrefOption{{ID: "", Title: L("Dark")}, {ID: "light", Title: L("Light")}, {ID: "system", Title: L(themeSystemLabel())}}},
			{ID: "app_icon", Kind: "select", Label: L("App icon"), Detail: L(appIconDetail()), Value: cfg.AppIcon,
				Options: []remoteview.PrefOption{{ID: "", Title: L("Color")}, {ID: "dark", Title: L("Dark")}}},
			{ID: "tray_icon", Kind: "select", Label: L(trayIconLabel()), Detail: L(trayIconDetail()), Value: cfg.TrayIcon,
				Options: []remoteview.PrefOption{{ID: "", Title: L("Color")}, {ID: "dark", Title: L("Dark")}}},
		}},
		u.serverSection(cfg),
		u.directSection(cfg),
		{Title: L("Interface"), Footer: L("The new language shows the next time go-link Desk opens."), Rows: []remoteview.PrefRow{
			{ID: "language", Kind: "select", Label: L("Language"), Value: cfg.Language, Options: langs},
		}},
	}}
	if a := u.opts.Autostart; a != nil {
		row := remoteview.PrefRow{ID: "autostart", Kind: "toggle", Label: L("Open go-link Desk when I log in"), On: a.Enabled()}
		if !a.Supported {
			row.Enabled = off()
		}
		general.Sections = append(general.Sections, remoteview.PrefSection{Title: L("Start"), Rows: []remoteview.PrefRow{row}})
	}
	pages = append(pages, general)

	// Sessions.
	idle := []remoteview.PrefOption{}
	for _, m := range idleChoices {
		idle = append(idle, remoteview.PrefOption{ID: strconv.Itoa(m), Title: idleLabel(m)})
	}
	pages = append(pages, remoteview.PrefPage{ID: "sessions", Title: L("Sessions"), Symbol: "display.2", Sections: []remoteview.PrefSection{
		{Title: L("When someone connects to this computer"), Rows: []remoteview.PrefRow{
			{ID: "idle", Kind: "select", Label: L("Disconnect when idle"), Value: strconv.Itoa(cfg.IdleMinutes), Options: idle},
			{ID: "lock", Kind: "toggle", Label: L("Lock the screen when a session ends"), On: cfg.LockOnEnd},
			{ID: "request_ticks", Kind: "select", Label: L("A request starts with"), Detail: L("Only the screen: whoever asks has to be given each permission by hand."), Value: cfg.RequestTicks,
				Options: []remoteview.PrefOption{{ID: "", Title: L("Full control")}, {ID: "view", Title: L("Only the screen")}}},
		}},
		{Title: L("Connecting to other computers"), Rows: []remoteview.PrefRow{
			{ID: "viewer_name", Kind: "text", Label: L("Your name"), Detail: L("Shown to the computer you connect to. Empty: your user's name."), Value: cfg.ViewerName, Placeholder: u.viewerName()},
			{ID: "tabs", Kind: "toggle", Label: L("Open sessions in tabs"), Detail: L("Off: each session in a window of its own."), On: !cfg.SessionWindows},
		}},
	}})

	// Unattended access: always with a password, so it is set first and
	// the switch waits for it.
	hasPass := cfg.Unattended.PasswordHash != ""
	toggle := remoteview.PrefRow{ID: "unattended", Kind: "toggle", Label: L("Allow unattended access"), On: cfg.Unattended.Enabled && hasPass}
	if !hasPass {
		toggle.Enabled = off()
		toggle.Detail = L("Set a password below first.")
	}
	un := remoteview.PrefPage{ID: "unattended", Title: L("Unattended access"), Symbol: "lock.shield", Sections: []remoteview.PrefSection{
		{Rows: []remoteview.PrefRow{
			toggle,
			{Kind: "note", Label: L("Linked devices come in without you accepting, with the permissions you gave them and the password. Link only your own devices."), Tint: "warn"},
		}},
	}}
	pass := []remoteview.PrefRow{}
	if hasPass {
		pass = append(pass, remoteview.PrefRow{Kind: "info", Label: L("Password"), Value: L("Set"), Tint: "ok", Detail: L("Linked devices type it after their link; the iPhone and iPad app keeps it behind Face ID.")})
	} else {
		pass = append(pass, remoteview.PrefRow{Kind: "info", Label: L("Password"), Value: L("None"), Tint: "warn", Detail: L("Unattended access needs one.")})
	}
	pass = append(pass, remoteview.PrefRow{ID: "password", Kind: "password", Label: L("New password"),
		Placeholder: Lf("%d characters or more", services.MinPassword), Repeat: L("The same again"), Button: L("Set password")})
	if u.prefs.passErr != "" {
		pass = append(pass, remoteview.PrefRow{Kind: "note", Label: u.prefs.passErr, Tint: "danger"})
	} else if u.prefs.passOK != "" {
		pass = append(pass, remoteview.PrefRow{Kind: "note", Label: u.prefs.passOK, Tint: "ok"})
	}
	if hasPass {
		pass = append(pass, remoteview.PrefRow{ID: "password_clear", Kind: "button", Label: L("Remove the password"), Button: L("Remove password"), Tint: "danger",
			Confirm: &remoteview.PrefConfirm{Title: L("Remove the password?"), Text: L("Unattended access turns off: it always needs a password."), OK: L("Remove password"), Cancel: L("Cancel")}})
	}
	un.Sections = append(un.Sections, remoteview.PrefSection{Title: L("Password"), Rows: pass})
	if cfg.Unattended.Enabled && hasPass {
		code := strings.ReplaceAll(st.Pairing.Code, " ", "")
		shown := "— — —"
		if code != "" {
			shown = services.FormatCode(code)
		}
		un.Sections = append(un.Sections, remoteview.PrefSection{Title: L("Linking code"), Rows: []remoteview.PrefRow{
			{Kind: "code", Label: L("Linking code"), Value: shown,
				Detail: Lf("To link a device: open %s in its browser, or in the iPhone and iPad app go to My computers › Link, and type this code. You will be asked to accept it.", strings.TrimRight(u.opts.WebURL, "/")+"/link")},
		}})
		if len(cfg.Links) == 0 {
			un.Sections = append(un.Sections, remoteview.PrefSection{Title: L("Linked devices"), Rows: []remoteview.PrefRow{
				{Kind: "note", Label: L("No device is linked.")},
			}})
		}
		for i, l := range cfg.Links {
			name := l.Name
			if name == "" {
				name = L("A device")
			}
			title := ""
			if i == 0 {
				title = L("Linked devices")
			}
			p := l.Permissions
			perm := func(what, label string, on bool) remoteview.PrefRow {
				return remoteview.PrefRow{ID: "link:" + l.ID + ":" + what, Kind: "toggle", Label: label, On: on}
			}
			un.Sections = append(un.Sections, remoteview.PrefSection{Title: title, Rows: []remoteview.PrefRow{
				{Kind: "info", Label: name, Detail: Lf("Linked %s · last used %s", l.CreatedAt.Local().Format("2006-01-02"), l.LastSeen.Local().Format("2006-01-02 15:04"))},
				perm("control", L("Use your mouse and keyboard"), p.Control),
				perm("audio", L("Hear your computer's sound"), p.Audio),
				perm("clipboard", L("Share the clipboard (copy and paste)"), p.Clipboard),
				perm("files", L("Send and receive files"), p.Files),
				perm("record", L("Take screenshots and record the screen"), p.Record),
				{ID: "revoke:" + l.ID, Kind: "button", Label: L("Revoke this device"), Button: L("Revoke"), Tint: "danger",
					Confirm: &remoteview.PrefConfirm{Title: L("Revoke this device?"), Text: Lf("%s will not be able to come in again until it is linked with a new code.", name), OK: L("Revoke"), Cancel: L("Cancel")}},
			}})
		}
	}
	pages = append(pages, un)

	// Permissions.
	if p := u.perms; p != nil {
		rows := []remoteview.PrefRow{}
		for _, k := range []struct {
			kind  permission.Kind
			id    string
			label string
		}{
			{permission.ScreenRecording, "screen", L("Screen Recording: to show your screen")},
			{permission.Accessibility, "accessibility", L("Accessibility: to let the viewer use the mouse and keyboard")},
		} {
			if p.granted(k.kind) {
				rows = append(rows, remoteview.PrefRow{Kind: "info", Label: k.label, Value: L("Allowed"), Tint: "ok"})
			} else {
				rows = append(rows, remoteview.PrefRow{ID: "perm:" + k.id, Kind: "button", Label: k.label, Button: L("Allow…")})
			}
		}
		pages = append(pages, remoteview.PrefPage{ID: "permissions", Title: L("Permissions"), Symbol: "hand.raised", Sections: []remoteview.PrefSection{
			{Title: L("This computer's permissions"), Footer: L("After allowing it in System Settings, quit go-link Desk and open it again."), Rows: rows},
		}})
	}
	return remoteview.Prefs{Pages: pages}
}

// setting takes a change from the native Settings.
func (u *ui) setting(arg string) {
	id, value, _ := strings.Cut(arg, "\n")
	set := u.opts.Settings
	update := func(fn func(*models.Config)) { go func() { _ = set.Update(fn); fyne.Do(u.refreshPrefs) }() }
	on := value == "1"
	switch {
	case id == "app_icon":
		if value != "dark" {
			value = ""
		}
		u.app.SetIcon(fyne.NewStaticResource("go-link-desk.png", appIcon(value)))
		mainSetIcon(appIcon(value))
		update(func(c *models.Config) { c.AppIcon = value })
	case id == "tray_icon":
		if value != "dark" {
			value = ""
		}
		u.trayDark = value == "dark"
		u.trayLive = !u.trayLive // repaint
		u.renderTray(activeOf(u.lastSt))
		update(func(c *models.Config) { c.TrayIcon = value })
	case id == "theme":
		mainSetTheme(themeOf(value))
		update(func(c *models.Config) { c.Theme = value })
	case id == "signal_url":
		v := strings.TrimSpace(value)
		if v != "" {
			if _, err := signalclient.CheckURL(v); err != nil {
				u.prefs.serverErr = L("That is not a server address: it starts with wss:// (or ws:// on this computer).")
				u.prefs.last = ""
				u.refreshPrefs()
				return
			}
		}
		if v == models.DefaultSignalURL {
			v = ""
		}
		u.prefs.serverErr = ""
		update(func(c *models.Config) { c.SignalURL = v })
	case id == "web_url":
		v := strings.TrimRight(strings.TrimSpace(value), "/")
		if v != "" && !strings.HasPrefix(v, "https://") && !strings.HasPrefix(v, "http://") {
			u.prefs.serverErr = L("The website's address starts with https://.")
			u.prefs.last = ""
			u.refreshPrefs()
			return
		}
		if v == models.DefaultWebURL {
			v = ""
		}
		u.prefs.serverErr = ""
		update(func(c *models.Config) { c.WebURL = v })
	case id == "udp_port":
		port, msg := parseUDPPort(value)
		if msg != "" {
			u.prefs.netErr = msg
			u.prefs.last = ""
			u.refreshPrefs()
			return
		}
		u.prefs.netErr = ""
		update(func(c *models.Config) { c.UDPPort = port })
	case id == "map_udp_port":
		update(func(c *models.Config) { c.MapUDPPort = on })
	case id == "announce_ips":
		ips, msg := parseAnnounce(value)
		if msg != "" {
			u.prefs.netErr = msg
			u.prefs.last = ""
			u.refreshPrefs()
			return
		}
		u.prefs.netErr = ""
		update(func(c *models.Config) { c.AnnounceIPs = ips })
	case id == "server_default":
		u.prefs.serverErr = ""
		update(func(c *models.Config) { c.SignalURL, c.WebURL = "", "" })
	case id == "relaunch":
		go u.relaunch()
	case id == "language":
		update(func(c *models.Config) { c.Language = value })
	case id == "autostart":
		go func() {
			if err := u.opts.Autostart.Set(on); err != nil {
				u.opts.Logger.Warn("cannot change the login item", "err", err)
			}
			fyne.Do(u.refreshPrefs)
		}()
	case id == "idle":
		if m, err := strconv.Atoi(value); err == nil {
			update(func(c *models.Config) { c.IdleMinutes = m })
		}
	case id == "lock":
		update(func(c *models.Config) { c.LockOnEnd = on })
	case id == "request_ticks":
		if value != "view" {
			value = ""
		}
		update(func(c *models.Config) { c.RequestTicks = value })
	case id == "viewer_name":
		name := strings.TrimSpace(value)
		if r := []rune(name); len(r) > models.MaxKnockName {
			name = string(r[:models.MaxKnockName])
		}
		update(func(c *models.Config) { c.ViewerName = name })
	case id == "tabs":
		update(func(c *models.Config) { c.SessionWindows = !on })
	case id == "unattended":
		if on && set.Get().Unattended.PasswordHash == "" {
			u.prefs.passErr, u.prefs.passOK = L("Set a password first: unattended access always has one."), ""
			u.prefs.last = ""
			u.refreshPrefs()
			return
		}
		update(func(c *models.Config) { c.Unattended.Enabled = on })
	case id == "password":
		pw, again, twice := strings.Cut(value, "\n")
		if twice && pw != again {
			u.prefs.passErr, u.prefs.passOK = L("The two passwords are not the same. Type them again."), ""
			u.prefs.last = ""
			u.refreshPrefs()
			return
		}
		hash, err := services.HashPassword(pw)
		switch {
		case errors.Is(err, services.ErrPasswordLength):
			u.prefs.passErr, u.prefs.passOK = Lf("The password must have %d to %d characters.", services.MinPassword, services.MaxPassword), ""
			u.prefs.last = ""
			u.refreshPrefs()
		case err != nil:
			u.opts.Logger.Warn("cannot set the password", "err", err)
		default:
			u.prefs.passErr, u.prefs.passOK = "", L("Password set. Linked devices will type it to come in.")
			update(func(c *models.Config) { c.Unattended.PasswordHash = hash })
		}
	case id == "password_clear":
		u.prefs.passErr, u.prefs.passOK = "", ""
		update(func(c *models.Config) { c.Unattended.PasswordHash, c.Unattended.Enabled = "", false })
	case strings.HasPrefix(id, "perm:") && u.perms != nil:
		kind := permission.ScreenRecording
		if id == "perm:accessibility" {
			kind = permission.Accessibility
		}
		go func() {
			u.perms.request(kind)
			fyne.Do(u.refreshPrefs)
		}()
	case strings.HasPrefix(id, "link:") && u.opts.Links != nil:
		parts := strings.Split(id, ":")
		if len(parts) != 3 {
			return
		}
		go func() {
			u.opts.Links.SetLink(parts[1], func(l *models.Link) {
				switch parts[2] {
				case "control":
					l.Permissions.Control = on
				case "audio":
					l.Permissions.Audio = on
				case "clipboard":
					l.Permissions.Clipboard = on
				case "files":
					l.Permissions.Files = on
				case "record":
					l.Permissions.Record = on
				}
			})
			fyne.Do(u.refreshPrefs)
		}()
	case strings.HasPrefix(id, "revoke:") && u.opts.Links != nil:
		go func() {
			u.opts.Links.Revoke(strings.TrimPrefix(id, "revoke:"))
			fyne.Do(u.refreshPrefs)
		}()
	}
}

// appIcon is the app icon chosen in Settings.
func appIcon(choice string) []byte {
	if choice == "dark" {
		return trayicon.AppDarkPNG()
	}
	return trayicon.AppPNG()
}

// themeOf is the windows' theme for the setting.
func themeOf(v string) int {
	switch v {
	case "light":
		return remoteview.ThemeLight
	case "system":
		return remoteview.ThemeSystem
	}
	return remoteview.ThemeDark
}

// serverSection is where this computer finds signalhub: go-link's, or the
// person's own (github.com/lordbasex/signalhub).
func (u *ui) serverSection(cfg models.Config) remoteview.PrefSection {
	rows := []remoteview.PrefRow{
		{ID: "signal_url", Kind: "text", Label: L("Signaling server"), Value: cfg.SignalURL, Placeholder: models.DefaultSignalURL,
			Detail: L("Your own signalhub (github.com/lordbasex/signalhub). Empty: go-link's.")},
		{ID: "web_url", Kind: "text", Label: L("Website for viewers"), Value: cfg.WebURL, Placeholder: models.DefaultWebURL,
			Detail: L("The one shown with your code. It must use the same server.")},
	}
	if u.prefs.serverErr != "" {
		rows = append(rows, remoteview.PrefRow{Kind: "note", Label: u.prefs.serverErr, Tint: "danger"})
	}
	want := cfg.EffectiveSignalURL()
	if enc, err := signalclient.CheckURL(want); err == nil && !enc && !signalclient.IsLoopback(want) {
		rows = append(rows, remoteview.PrefRow{Kind: "note", Label: L("ws:// is not encrypted: use wss:// outside this computer."), Tint: "warn"})
	}
	if u.opts.SignalURL != "" && u.opts.SignalURL != want {
		rows = append(rows, remoteview.PrefRow{Kind: "info", Label: L("In use now"), Value: u.opts.SignalURL, Tint: "warn"},
			remoteview.PrefRow{ID: "relaunch", Kind: "button", Label: L("The new server is used when go-link Desk opens again."), Button: L("Open again now")})
	}
	if cfg.SignalURL != "" || cfg.WebURL != "" {
		rows = append(rows, remoteview.PrefRow{ID: "server_default", Kind: "button", Label: L("Go back to go-link's server"), Button: L("Use go-link's")})
	}
	return remoteview.PrefSection{Title: L("Server"), Footer: L("This computer, the computers it connects to and their browsers must all use the same server."), Rows: rows}
}

// directSection is the advanced part of the network: one UDP port for
// every connection, and the addresses where viewers reach it through a
// router that forwards it (a computer behind a second router, which
// browsers on the first one cannot reach on their own).
func (u *ui) directSection(cfg models.Config) remoteview.PrefSection {
	local := localIP()
	port := ""
	if cfg.UDPPort > 0 {
		port = strconv.Itoa(cfg.UDPPort)
	}
	rows := []remoteview.PrefRow{
		{Kind: "info", Label: L("This computer's address"), Value: local},
		{ID: "udp_port", Kind: "text", Label: L("UDP port"), Value: port, Placeholder: L("Random"),
			Detail: L("Every connection on one port, to open it in a router.")},
		{ID: "map_udp_port", Kind: "toggle", Label: L("Open it in the router by itself"), On: cfg.MapUDPPort,
			Detail: L("With UPnP or NAT-PMP, while go-link Desk is open. Off: forward it in the router by hand.")},
		{ID: "announce_ips", Kind: "text", Label: L("Announced address"), Value: strings.Join(cfg.AnnounceIPs, ", "), Placeholder: Lf("For example: %s", "192.168.1.20"),
			Detail: L("Where viewers reach that port through the router: its address on their network. Several, with commas.")},
	}
	if u.prefs.netErr != "" {
		rows = append(rows, remoteview.PrefRow{Kind: "note", Label: u.prefs.netErr, Tint: "danger"})
	}
	if text, tint := u.portStatus(cfg); text != "" {
		rows = append(rows, remoteview.PrefRow{Kind: "note", Label: text, Tint: tint})
	}
	if cfg.UDPPort > 0 && !cfg.MapUDPPort && local != "" {
		rows = append(rows, remoteview.PrefRow{Kind: "note", Label: Lf("In the router, forward UDP port %d to %s.", cfg.UDPPort, local)})
	}
	if u.directChanged(cfg) {
		rows = append(rows, remoteview.PrefRow{ID: "relaunch", Kind: "button", Label: L("The port and address are used when go-link Desk opens again."), Button: L("Open again now")})
	}
	return remoteview.PrefSection{Title: L("Direct connection (advanced)"),
		Footer: L("For a computer behind a second router: without it, viewers on the other network go through a relay, which adds delay."), Rows: rows}
}

// localIP is the address this computer leaves its network with: the one
// a router forwards to (not a virtual machine's or a VPN's network).
func localIP() string {
	if a := netprobe.LocalIP(); a.IsValid() {
		return a.String()
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && !n.IP.IsLoopback() && n.IP.To4() != nil && !n.IP.IsLinkLocalUnicast() {
			return n.IP.String()
		}
	}
	return ""
}

// The icons' rows in each system's words: the Mac's Dock and menu bar,
// Windows' taskbar and notification area, a Linux desktop's dock or
// taskbar and its system tray.
// themeSystemLabel words the Theme setting's third choice as the system's
// own settings do.
func themeSystemLabel() string {
	if runtime.GOOS == "windows" {
		return "Use system setting"
	}
	return "Like the system"
}

func appIconDetail() string {
	switch runtime.GOOS {
	case "windows":
		return "The one in the taskbar while go-link Desk is open."
	case "linux":
		return "The one in the dock or taskbar while go-link Desk is open."
	}
	return "The one in the Dock while go-link Desk is open."
}

func trayIconLabel() string {
	switch runtime.GOOS {
	case "windows":
		return "Notification area icon"
	case "linux":
		return "System tray icon"
	}
	return "Menu bar icon"
}

func trayIconDetail() string {
	if runtime.GOOS == "linux" {
		return "In the panel, where the desktop has a system tray. It turns red while someone is connected."
	}
	return "Next to the clock. It turns red while someone is connected."
}
