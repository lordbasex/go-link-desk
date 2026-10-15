// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"fyne.io/fyne/v2"

	"github.com/lordbasex/go-link-desk/backend-device/internal/mcpserver"
	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/keyring"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/remoteview"
)

// Settings › Assistants: AI assistants (MCP clients) may use the computers
// in My computers. The switch adds go-link Desk to the assistants found
// here (mcpserver.Sync, again at every start so a moved or updated app is
// followed); each computer is allowed by keeping its unattended password
// in the system's keychain, which the assistant never sees.

// assistantsState is what Settings shows of the assistants.
type assistantsState struct {
	mu    sync.Mutex
	regs  []mcpserver.Registration
	saved map[string]bool   // computers allowed (a password kept)
	errs  map[string]string // the last error per computer
}

// clients are the assistants of this user.
func assistantClients() []mcpserver.Client {
	home, _ := os.UserHomeDir()
	return mcpserver.Clients(runtime.GOOS, home, os.Getenv("APPDATA"))
}

// appPath is this app's executable, as the assistants should run it.
func appPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	return exe
}

// startAssistants keeps the assistants' settings right at start.
func (u *ui) startAssistants() {
	if u.opts.Settings == nil {
		return
	}
	on := u.opts.Settings.Get().Assistants.Enabled
	go func() {
		u.loadAllowed()
		if on {
			u.syncAssistants(true)
		}
		fyne.Do(u.refreshPrefs)
	}()
}

// syncAssistants adds go-link Desk to the assistants, or takes it out.
func (u *ui) syncAssistants(on bool) {
	exe := appPath()
	if exe == "" {
		return
	}
	regs := mcpserver.Sync(assistantClients(), exe, on)
	for _, r := range regs {
		if r.Err != nil {
			u.opts.Logger.Warn("cannot change an assistant's settings", "assistant", r.Client.ID, "err", r.Err)
		} else if r.Registered {
			u.opts.Logger.Info("go-link Desk is in an assistant's settings", "assistant", r.Client.ID, "path", r.Client.File)
		}
	}
	u.asst.mu.Lock()
	u.asst.regs = regs
	u.asst.mu.Unlock()
}

// loadAllowed learns which computers have a password kept.
func (u *ui) loadAllowed() {
	if u.opts.Remotes == nil {
		return
	}
	saved := map[string]bool{}
	for _, d := range u.opts.Remotes.Get().Devices {
		if _, err := keyring.Get(mcpserver.PasswordService, d.DeviceID); err == nil {
			saved[d.DeviceID] = true
		}
	}
	u.asst.mu.Lock()
	u.asst.saved = saved
	u.asst.mu.Unlock()
}

// assistantsPage is Settings' Assistants page.
func (u *ui) assistantsPage(cfg models.Config) remoteview.PrefPage {
	a := &u.asst
	a.mu.Lock()
	defer a.mu.Unlock()
	on := cfg.Assistants.Enabled
	rows := []remoteview.PrefRow{
		{ID: "assistants", Kind: "toggle", Label: L("Let AI assistants use your computers"), On: on,
			Detail: L("Adds go-link Desk to Claude, Claude Code and Cursor on this computer (as an MCP server). Open them again after turning it on.")},
	}
	if on {
		for _, r := range a.regs {
			row := remoteview.PrefRow{Kind: "info", Label: r.Client.Name}
			switch {
			case r.Err != nil:
				row.Value, row.Tint, row.Detail = L("Could not add it"), "danger", r.Err.Error()
			case r.Registered:
				row.Value, row.Tint = L("Added"), "ok"
			default:
				row.Value = L("Not installed")
			}
			rows = append(rows, row)
		}
	}
	page := remoteview.PrefPage{ID: "assistants", Title: L("Assistants"), Symbol: "sparkles", Sections: []remoteview.PrefSection{{Rows: rows}}}
	if !on {
		return page
	}
	page.Sections = append(page.Sections, remoteview.PrefSection{Title: L("What they may do"), Rows: []remoteview.PrefRow{
		{ID: "assistants_act", Kind: "toggle", Label: L("Let them type and run commands"), On: cfg.Assistants.Act,
			Detail: L("Off: they only look: your computers, screenshots, terminals, the clipboard, and files they copy here.")},
	}})
	var devs []remoteview.PrefRow
	if u.opts.Remotes != nil {
		for _, d := range u.opts.Remotes.Get().Devices {
			name := d.Title()
			if name == "" {
				name = L("Linked computer")
			}
			if a.saved[d.DeviceID] {
				devs = append(devs, remoteview.PrefRow{ID: "assistant_forget:" + d.DeviceID, Kind: "button", Label: name, Detail: L("Allowed: its password is kept in this computer's keychain."),
					Button: L("Stop allowing"), Tint: "danger"})
			} else {
				devs = append(devs, remoteview.PrefRow{ID: "assistant_pass:" + d.DeviceID, Kind: "password", Label: name,
					Placeholder: L("Its password"), Button: L("Allow")})
			}
			if msg := a.errs[d.DeviceID]; msg != "" {
				devs = append(devs, remoteview.PrefRow{Kind: "note", Label: msg, Tint: "danger"})
			}
		}
	}
	if len(devs) == 0 {
		devs = append(devs, remoteview.PrefRow{Kind: "note", Label: L("No computer is linked yet: link one from My computers first.")})
	}
	page.Sections = append(page.Sections, remoteview.PrefSection{Title: L("Computers they may use"), Rows: devs,
		Footer: L("Each computer's unattended password is kept in this computer's keychain: assistants never see it. They come in by name, with that link's permissions, and the computer shows it.")})
	page.Sections = append(page.Sections, remoteview.PrefSection{Rows: []remoteview.PrefRow{
		{ID: "assistants_log", Kind: "button", Label: L("Everything they do is written to an activity log."), Button: L("Show the log")},
	}})
	return page
}

// assistantSetting takes a change of the Assistants page; false if it is
// not one.
func (u *ui) assistantSetting(id, value string, update func(func(*models.Config))) bool {
	refresh := func() { fyne.Do(func() { u.prefs.last = ""; u.refreshPrefs() }) }
	switch {
	case id == "assistants":
		on := value == "1"
		update(func(c *models.Config) { c.Assistants.Enabled = on })
		go func() {
			u.syncAssistants(on)
			refresh()
		}()
	case id == "assistants_act":
		on := value == "1"
		update(func(c *models.Config) { c.Assistants.Act = on })
	case strings.HasPrefix(id, "assistant_pass:"):
		dev := strings.TrimPrefix(id, "assistant_pass:")
		pass, _, _ := strings.Cut(value, "\n")
		go func() {
			msg := ""
			if pass == "" {
				msg = L("Type its password first.")
			} else if err := keyring.Set(mcpserver.PasswordService, dev, pass); err != nil {
				msg = L("The keychain did not keep it: ") + err.Error()
				if errors.Is(err, keyring.ErrUnavailable) {
					msg = L("This computer has no keychain go-link Desk can use (on Linux, install secret-tool).")
				}
			}
			u.asst.mu.Lock()
			if u.asst.errs == nil {
				u.asst.errs = map[string]string{}
			}
			u.asst.errs[dev] = msg
			if msg == "" {
				if u.asst.saved == nil {
					u.asst.saved = map[string]bool{}
				}
				u.asst.saved[dev] = true
			}
			u.asst.mu.Unlock()
			refresh()
		}()
	case strings.HasPrefix(id, "assistant_forget:"):
		dev := strings.TrimPrefix(id, "assistant_forget:")
		go func() {
			u.forgetAssistantPassword(dev)
			refresh()
		}()
	case id == "assistants_log":
		if u.opts.ConfigDir != "" {
			path := filepath.Join(u.opts.ConfigDir, mcpserver.AuditFile)
			if _, err := os.Stat(path); err != nil {
				_ = os.WriteFile(path, nil, 0o600)
			}
			reveal(path)
		}
	default:
		return false
	}
	return true
}

// forgetAssistantPassword stops letting assistants use a computer.
func (u *ui) forgetAssistantPassword(dev string) {
	if err := keyring.Delete(mcpserver.PasswordService, dev); err != nil {
		u.opts.Logger.Warn("cannot remove a password from the keychain", "err", err)
	}
	u.asst.mu.Lock()
	delete(u.asst.saved, dev)
	delete(u.asst.errs, dev)
	u.asst.mu.Unlock()
}
