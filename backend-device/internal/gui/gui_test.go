// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
)

func TestTheWindowShowsTheSessionCode(t *testing.T) {
	a := test.NewTempApp(t)
	st := services.NewStatusService("d", "1.0.0", "ws://x")
	asked := 0
	u := newUI(a, Options{Status: st, NewCode: func() { asked++ }, WebURL: "https://desk.example/", Language: "en"})

	u.render(st.Snapshot())
	if !u.copyBtn.Disabled() || !u.newBtn.Disabled() {
		t.Fatal("buttons work without a code")
	}

	st.SetSession(&models.SessionStatus{RoomID: "R", Code: "915355636", Invite: "abcdefghijklmnopqrstuv"})
	u.render(st.Snapshot())
	if u.code.Text != "915 355 636" {
		t.Fatalf("code %q", u.code.Text)
	}
	if u.link.Text != "Or open https://desk.example/c/abcdefghijklmnopqrstuv" {
		t.Fatalf("link %q", u.link.Text)
	}
	u.copyCode()
	if got := a.Clipboard().Content(); got != "915 355 636" {
		t.Fatalf("clipboard %q", got)
	}
	test.Tap(u.newBtn)
	if asked != 1 {
		t.Fatal("New code did not ask for one")
	}
}

func TestEveryTextIsTranslated(t *testing.T) {
	es, pt := catalogs["es"], catalogs["pt"]
	for k := range es {
		if _, ok := pt[k]; !ok {
			t.Errorf("pt lacks %q", k)
		}
	}
	for k := range pt {
		if _, ok := es[k]; !ok {
			t.Errorf("es lacks %q", k)
		}
	}
}
