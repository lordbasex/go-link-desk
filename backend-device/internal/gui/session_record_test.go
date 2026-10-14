// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"testing"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
)

func TestThePanelShowsARecording(t *testing.T) {
	u, _, _ := newTestUI(t)
	at := time.Date(2026, 10, 14, 15, 4, 0, 0, time.Local)
	a := &models.ActiveSession{PeerID: "P", Name: "Ana", Since: at, Permissions: models.Permissions{View: true, Record: true}}
	u.panel.render(a)
	if u.panel.recording.Visible() || !u.panel.checks.record.Checked {
		t.Fatal("a recording shown before any")
	}
	a.Screenshots, a.LastScreenshot = 2, at
	u.panel.render(a)
	if got := u.panel.recording.Text; !u.panel.recording.Visible() || got != "Took 2 screenshots, the last at 15:04" {
		t.Fatalf("shows %q", got)
	}
	a.RecordingSince = at
	u.panel.render(a)
	if got := u.panel.recording.Text; got != "● Recording your screen since 15:04" {
		t.Fatalf("shows %q", got)
	}
}
