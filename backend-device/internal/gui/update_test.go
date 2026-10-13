// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/widget"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/updater"
)

// A release found by the periodic check opens its window once; during a
// session it waits for the session to end.
func TestAnUpdateShowsOnceAndNotDuringASession(t *testing.T) {
	u, st, _ := newTestUI(t)
	u.opts.Version = "v1.3.0"
	info := models.UpdateInfo{Latest: "v1.3.1", URL: "https://github.com/lordbasex/go-link-desk/releases/tag/v1.3.1"}

	st.SetSession(&models.SessionStatus{RoomID: "R", Code: "915355636"})
	st.SetActive(&models.ActiveSession{PeerID: "P", Name: "Ana", Since: time.Now()})
	u.render(st.Snapshot())
	u.upd.found(info)
	if u.upd.win != nil && u.upd.win.Content().Visible() && u.upd.title.Text != "" {
		t.Fatalf("shown during a session: %q", u.upd.title.Text)
	}
	if got := u.upd.trayLabel(); got != "Update to 1.3.1…" {
		t.Errorf("tray %q", got)
	}

	st.SetActive(nil)
	u.render(st.Snapshot())
	if u.upd.win == nil || u.upd.title.Text != "go-link Desk 1.3.1 is available" {
		t.Fatal("not shown when the session ended")
	}
	if u.upd.text.Text == "" {
		t.Error("no text under the title")
	}

	u.upd.win.Hide()
	u.upd.title.SetText("")
	u.upd.found(info) // the next periodic check: the same release
	if u.upd.title.Text != "" {
		t.Fatal("the same release opened its window again")
	}
	u.upd.ask() // asked for: it shows
	if u.upd.title.Text != "go-link Desk 1.3.1 is available" {
		t.Fatalf("Check for Updates… showed %q", u.upd.title.Text)
	}
}

// A refused download says why and leaves Close where Update was.
func TestAFailedUpdateSaysWhy(t *testing.T) {
	u, _, _ := newTestUI(t)
	u.opts.Version = "v1.3.0"
	info := models.UpdateInfo{Latest: "v1.3.1", URL: "https://github.com/lordbasex/go-link-desk/releases/tag/v1.3.1"}
	u.upd.known = info
	u.upd.failed(info, fmt.Errorf("%w: test", updater.ErrSignature))
	if u.upd.title.Text != "The update was not installed" || !strings.Contains(u.upd.text.Text, "signature") {
		t.Fatalf("%q: %q", u.upd.title.Text, u.upd.text.Text)
	}
	last := u.upd.buttons.Objects[len(u.upd.buttons.Objects)-1].(*widget.Button)
	if last.Text != "Close" {
		t.Fatalf("the last button is %q", last.Text)
	}
	u.upd.failed(info, updater.ErrCancelled) // the password prompt was closed
	if u.upd.title.Text != "go-link Desk 1.3.1 is available" {
		t.Fatalf("after a cancel: %q", u.upd.title.Text)
	}
}
