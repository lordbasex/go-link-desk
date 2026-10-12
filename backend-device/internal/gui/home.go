// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"encoding/json"
	"strings"
	"time"
	"unicode"

	"fyne.io/fyne/v2"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
	"github.com/lordbasex/go-link-desk/backend-device/internal/viewer"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/permission"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/remoteview"
)

// openMain builds the system's own main window: Home (this computer,
// connecting to another one, My computers, the recent sessions) and a tab
// per session.
func (u *ui) openMain() {
	labels := map[string]string{
		"thisComputer":   L("This computer"),
		"yourCode":       L("Your code"),
		"copy":           L("Copy"),
		"copied":         L("Copied"),
		"newCode":        L("New code"),
		"hint":           L("Share this code only with someone you know. Nobody legitimate will call you asking for access to your computer."),
		"permissions":    L("Permissions"),
		"allowed":        L("Allowed"),
		"allow":          L("Allow…"),
		"settings":       L("Settings"),
		"connectTo":      L("Connect to another computer"),
		"address":        L("Its 9-digit code"),
		"doorSession":    L("Session"),
		"doorLink":       L("Link (unattended)"),
		"doorTip":        L("Session: the person there accepts this time. Link: with its pairing code, to come back without a code."),
		"connect":        L("Connect"),
		"myComputers":    L("My computers"),
		"recents":        L("Recent sessions"),
		"clear":          L("Clear"),
		"empty":          L("Computers you link with their pairing code show up here, to connect to them without a code."),
		"rename":         L("Rename…"),
		"renameTitle":    L("Name of this computer here"),
		"forget":         L("Forget this computer"),
		"save":           L("Save"),
		"cancel":         L("Cancel"),
		"favorites":      L("Favorites"),
		"favorite":       L("Add to favorites"),
		"unfavorite":     L("Remove from favorites"),
		"emptyFavorites": L("Mark a linked computer with the star to see it here."),
		"emptyRecents":   L("The computers you connect to with a code show up here."),
		"copyCode":       L("Copy the code"),
		"removeRecent":   L("Remove from the list"),
		"colName":        L("Name"),
		"colCode":        L("Code"),
		"colSystem":      L("System"),
		"colWhen":        L("Last connection"),
		"viewCards":      L("Cards"),
		"viewList":       L("List"),
		"viewDetails":    L("Detailed list"),
		"copyHint":       L("Click to copy"),
	}
	if err := mainOpen(labels, u); err != nil {
		u.opts.Logger.Warn("main window", "err", err)
		return
	}
	u.native = true
	mainSetMenu(map[string]string{
		"about":      L("About go-link Desk"),
		"settings":   L("Settings…"),
		"services":   L("Services"),
		"hide":       L("Hide go-link Desk"),
		"hideOthers": L("Hide others"),
		"showAll":    L("Show all"),
		"quit":       L("Quit go-link Desk"),
	})
	if u.opts.Settings != nil {
		cfg := u.opts.Settings.Get()
		u.prefs.start, u.prefs.startSet = cfg, true
		mainSetIcon(appIcon(cfg.AppIcon))
		mainSetTheme(themeOf(cfg.Theme))
	}
	u.opts.Remotes.OnChange(func(models.Remotes) { u.pushHome() })
	if u.opts.Settings != nil {
		u.opts.Settings.OnChange(func(models.Config) { fyne.Do(u.refreshPrefs) })
	}
	// The system permissions change outside the app: look again now and
	// then.
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-u.opts.Ctx.Done():
				return
			case <-t.C:
				u.pushHome()
			}
		}
	}()
}

// pushHome sends Home what it shows now, when it changed.
func (u *ui) pushHome() {
	fyne.Do(func() {
		st := u.lastSt
		h := remoteview.Home{Version: "v" + strings.TrimPrefix(u.opts.Version, "v")}
		if st.Session != nil {
			h.Code = services.FormatCode(st.Session.Code)
			if st.Session.Invite != "" {
				h.Link = strings.TrimRight(u.opts.WebURL, "/") + "/c/" + st.Session.Invite
			}
			h.CanNew = true
			if a := st.Session.Active; a != nil {
				h.InSession = true
				name := a.Name
				if name == "" {
					name = L("Someone")
				}
				h.Status = Lf("In session with %s", name)
			}
		}
		if h.Status == "" {
			switch st.Signal.State {
			case models.SignalConnected:
				h.Status, h.StatusOK = L("Connected"), true
			case models.SignalDisconnected:
				h.Status = L("Offline, reconnecting…")
			default:
				h.Status = L("Connecting…")
			}
		}
		h.StatusOK = h.StatusOK || h.InSession
		if p := u.perms; p != nil {
			for _, k := range []struct {
				kind  permission.Kind
				id    string
				label string
			}{
				{permission.ScreenRecording, "screen", L("Screen Recording: to show your screen")},
				{permission.Accessibility, "accessibility", L("Accessibility: to let the viewer use the mouse and keyboard")},
			} {
				h.Perms = append(h.Perms, remoteview.Perm{ID: k.id, Label: k.label, Granted: p.granted(k.kind)})
			}
		}
		r := u.opts.Remotes.Get()
		for _, d := range r.Devices {
			title := d.Title()
			if title == "" {
				title = L("Linked computer")
			}
			sub := osName(d.OS)
			if !d.LastUsed.IsZero() {
				sub = strings.TrimPrefix(sub+" · "+d.LastUsed.Local().Format("02/01 15:04"), " · ")
			}
			when := ""
			if !d.LastUsed.IsZero() {
				when = d.LastUsed.Local().Format("02/01/2006 15:04")
			}
			h.Devices = append(h.Devices, remoteview.Tile{ID: d.DeviceID, Title: title, Subtitle: sub, Thumb: d.Thumb, Favorite: d.Favorite,
				Name: title, Code: L("Linked"), System: osName(d.OS), When: when})
		}
		for _, s := range r.Recents {
			if s.DeviceID != "" || s.Code == "" {
				continue // a linked computer is in My computers already
			}
			code := services.FormatCode(s.Code)
			name := s.Name
			if name == "" {
				name = L("Unnamed computer")
			}
			h.Recents = append(h.Recents, remoteview.Tile{ID: code, Title: code, Subtitle: name + " · " + s.At.Local().Format("02/01 15:04"),
				Thumb: s.Thumb, Name: name, Code: code, System: osName(s.OS), When: s.At.Local().Format("02/01/2006 15:04")})
		}
		b, _ := json.Marshal(h)
		u.homeMu.Lock()
		same := string(b) == u.homeLast
		u.homeLast = string(b)
		u.homeMu.Unlock()
		if !same {
			mainSetHome(h)
		}
		u.refreshPrefs()
	})
}

// MainAction takes the main window's actions (on the main thread).
func (u *ui) MainAction(name, arg string) {
	remotes := u.opts.Remotes
	switch name {
	case "copy":
		fyne.Do(func() {
			if u.lastSt.Session != nil && u.lastSt.Session.Code != "" {
				u.app.Clipboard().SetContent(services.FormatCode(u.lastSt.Session.Code))
			}
		})
	case "newcode":
		if u.opts.NewCode != nil {
			go u.opts.NewCode()
		}
	case "settings":
		fyne.Do(u.showSettings)
	case "about":
		fyne.Do(u.showAbout)
	case "setting":
		fyne.Do(func() { u.setting(arg) })
	case "allow":
		if u.perms != nil {
			kind := permission.ScreenRecording
			if arg == "accessibility" {
				kind = permission.Accessibility
			}
			go func() {
				u.perms.request(kind)
				u.pushHome()
			}()
		}
	case "connect":
		kind, code, _ := strings.Cut(arg, ":")
		door := viewer.Door{Kind: viewer.DoorJoin, Code: digits(code)}
		if kind == "claim" {
			door.Kind = viewer.DoorClaim
		}
		if len(door.Code) == 9 {
			go u.connect(door)
		}
	case "device":
		if d, ok := remotes.Device(arg); ok {
			go u.connect(viewer.Door{Kind: viewer.DoorReach, Device: &d})
		}
	case "recent":
		if code := digits(arg); len(code) == 9 {
			go u.connect(viewer.Door{Kind: viewer.DoorJoin, Code: code})
		}
	case "favorite":
		go remotes.Update(arg, func(d *models.RemoteDevice) { d.Favorite = !d.Favorite })
	case "rename":
		id, label, _ := strings.Cut(arg, "\n")
		label = strings.TrimSpace(label)
		if r := []rune(label); len(r) > 64 {
			label = string(r[:64])
		}
		go remotes.Update(id, func(d *models.RemoteDevice) {
			d.Label = label
			if label == d.Name {
				d.Label = ""
			}
		})
	case "forget":
		go remotes.Forget(arg)
	case "clear":
		go remotes.ClearRecents()
	case "remove-recent":
		go remotes.RemoveRecent(digits(arg))
	}
}

// digits keeps a code's digits.
func digits(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, s)
}

// osName shows a computer's system.
func osName(os string) string {
	switch os {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	}
	return os
}
