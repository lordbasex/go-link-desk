// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
)

func newSettingsUI(t *testing.T, cfg models.Config) (*ui, *services.SettingsService, *services.LinkService, *bool) {
	t.Helper()
	a := test.NewTempApp(t)
	st := services.NewStatusService("d", "1.0.0", "ws://x")
	set := services.NewSettingsService(cfg, nil, nil)
	links := services.NewLinkService(services.LinkConfig{
		DeviceID: "d", Links: cfg.Links,
		Save: func(l []models.Link) error { return set.Update(func(c *models.Config) { c.Links = l }) },
	}, st)
	started := false
	u := newUI(a, Options{Status: st, Language: "en", Settings: set, Links: links, Autostart: &Autostart{
		Supported: true,
		Enabled:   func() bool { return started },
		Set:       func(on bool) error { started = on; return nil },
	}})
	return u, set, links, &started
}

func TestUnattendedAccessIsOffUntilTurnedOn(t *testing.T) {
	u, set, _, _ := newSettingsUI(t, models.Config{})
	s := u.settings
	if s.unattended.Checked || s.details.Visible() {
		t.Fatal("unattended access shows on by default")
	}
	// Not without a password: unattended access always has one.
	test.Tap(s.unattended)
	if set.UnattendedOn() || s.details.Visible() || !s.passErr.Visible() {
		t.Fatal("turned on without a password")
	}
	s.setPassword("a long enough one", "a long enough one")
	test.Tap(s.unattended)
	if !set.UnattendedOn() || !s.details.Visible() {
		t.Fatal("not turned on")
	}
	// The pairing code shows where the person links a browser.
	u.opts.Status.SetCode("113134323", time.Now(), time.Now().Add(time.Minute))
	u.render(u.opts.Status.Snapshot())
	if s.code.Text != "113 134 323" {
		t.Fatalf("code %q", s.code.Text)
	}
}

func TestThePasswordIsKeptAsAHash(t *testing.T) {
	u, set, _, _ := newSettingsUI(t, models.Config{Unattended: models.Unattended{Enabled: true}})
	s := u.settings
	s.newPass.SetText("short")
	s.setPassword(s.newPass.Text, s.newPass.Text)
	if set.Get().Unattended.PasswordHash != "" || !s.passErr.Visible() {
		t.Fatal("a short password was taken")
	}
	// Typed twice: two different ones are not taken.
	s.setPassword("a long enough one", "a long enough two")
	if set.Get().Unattended.PasswordHash != "" || !s.passErr.Visible() {
		t.Fatal("two different passwords were taken")
	}
	s.setPassword("a long enough one", "a long enough one")
	hash := set.Get().Unattended.PasswordHash
	if hash == "" || !services.CheckPassword(hash, "a long enough one") || s.passErr.Visible() || !s.passOK.Visible() {
		t.Fatalf("hash %q", hash)
	}
	// Removing it turns unattended access off.
	if !s.clearPass.Disabled() {
		test.Tap(s.clearPass)
	}
	if c := set.Get().Unattended; c.PasswordHash != "" || c.Enabled {
		t.Fatalf("not removed, or still on: %+v", c)
	}
}

func TestLinkedBrowsersAreListedChangedAndRevoked(t *testing.T) {
	cfg := models.Config{Unattended: models.Unattended{Enabled: true}, Links: []models.Link{
		{ID: "0123456789abcdef", Name: "Firefox 140 on Linux", Permissions: models.Permissions{View: true, Control: true}},
	}}
	u, set, links, _ := newSettingsUI(t, cfg)
	s := u.settings
	if len(s.linkIDs) != 1 {
		t.Fatalf("listed %v", s.linkIDs)
	}
	links.SetLink("0123456789abcdef", func(l *models.Link) { l.Permissions.Files = true })
	if got := set.Get().Links[0].Permissions; !got.Files || !got.Control {
		t.Fatalf("permissions %+v", got)
	}
	links.Revoke("0123456789abcdef")
	if len(set.Get().Links) != 0 {
		t.Fatal("not revoked")
	}
}

func TestSessionSettings(t *testing.T) {
	u, set, _, started := newSettingsUI(t, models.Config{})
	s := u.settings
	test.Tap(s.startup)
	if !*started || !s.startup.Checked {
		t.Fatal("login item not set")
	}
	s.idle.SetSelectedIndex(2)
	test.Tap(s.lock)
	if c := set.Get(); c.IdleMinutes != 15 || !c.LockOnEnd {
		t.Fatalf("settings %+v", c)
	}
}

func TestALinkRequestSaysWhatALinkIs(t *testing.T) {
	a := test.NewTempApp(t)
	u := newUI(a, Options{Status: services.NewStatusService("d", "1", "ws://x"), Language: "en"})
	r := u.newRequest(services.ConsentRequest{PeerID: "P", Link: true, Knock: models.Knock{Name: "Ana"}}, time.Now().Add(time.Minute), func(error) {})
	if r.who.Text != "Ana wants to link this browser to your computer." {
		t.Fatalf("who %q", r.who.Text)
	}
}

func TestDirectConnection(t *testing.T) {
	u, set, _, _ := newSettingsUI(t, models.Config{})
	s := u.settings
	if s.mapPort.Checked || s.portNote.Visible() || s.relaunch.Visible() {
		t.Fatal("the direct connection shows something by default")
	}
	// A port out of range is refused, and nothing changes.
	s.udpPort.SetText("80")
	s.udpPort.OnSubmitted(s.udpPort.Text)
	if !s.netErr.Visible() || set.Get().UDPPort != 0 {
		t.Fatal("port 80 was taken")
	}
	s.udpPort.SetText("40000")
	s.announce.SetText("192.168.1.20, 10.0.0.2")
	s.announce.OnSubmitted(s.announce.Text)
	cfg := set.Get()
	if s.netErr.Visible() || cfg.UDPPort != 40000 || len(cfg.AnnounceIPs) != 2 {
		t.Fatalf("not kept: %+v", cfg)
	}
	// By hand: the rule to set; the change waits for the app to open again.
	if !s.portNote.Visible() || !s.relaunch.Visible() {
		t.Fatal("no rule or no relaunch")
	}
	test.Tap(s.mapPort)
	if !set.Get().MapUDPPort {
		t.Fatal("opening it in the router was not kept")
	}
	// Without a port, it cannot be opened.
	_ = set.Update(func(c *models.Config) { c.UDPPort = 0 })
	if text, tint := u.portStatus(set.Get()); tint != "warn" || text == "" {
		t.Fatalf("%q %q", text, tint)
	}
}
