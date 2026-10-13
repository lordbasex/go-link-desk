// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
)

func readCodeFile(t *testing.T, path string) codeFileData {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var d codeFileData
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

// The file follows the code: written when it changes, offline while the
// host has none, and the link points at the site's /c/ page.
func TestCodeFileFollowsTheCode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.json")
	c := &CodeFile{Path: path, WebURL: "https://desk.go-link.org/", Name: "go-link Desk Demo", Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	c.Update(models.Status{})
	if d := readCodeFile(t, path); d.Online || d.Code != "" {
		t.Fatalf("no code yet: %+v", d)
	}
	c.Update(models.Status{Session: &models.SessionStatus{Code: "915355636", Invite: "abcdefghijklmnopqrstuv"}})
	d := readCodeFile(t, path)
	if !d.Online || d.Code != "915 355 636" || d.Link != "https://desk.go-link.org/c/abcdefghijklmnopqrstuv" || d.Name != "go-link Desk Demo" || d.Updated == "" {
		t.Fatalf("%+v", d)
	}
	// The same code again does not write: the file keeps its time.
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.Update(models.Status{Session: &models.SessionStatus{Code: "915355636", Invite: "abcdefghijklmnopqrstuv"}})
	if b, _ := os.ReadFile(path); string(b) != "{}" {
		t.Fatalf("rewritten for the same code: %s", b)
	}
	// Reconnecting: no code, then a new one.
	c.Update(models.Status{Session: &models.SessionStatus{RoomID: "r"}})
	if d := readCodeFile(t, path); d.Online {
		t.Fatalf("still online without a code: %+v", d)
	}
	c.Update(models.Status{Session: &models.SessionStatus{Code: "113134323", Invite: "zyxwvutsrqponmlkjihgfe"}})
	if d := readCodeFile(t, path); d.Code != "113 134 323" {
		t.Fatalf("%+v", d)
	}
}

// The pairing code and the password go in with the session code, and a
// new pairing code rewrites the file.
func TestCodeFileHasThePairingCode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.json")
	c := &CodeFile{Path: path, WebURL: "https://desk.go-link.org", Password: "demo-pass", Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	st := models.Status{Session: &models.SessionStatus{Code: "915355636", Invite: "abcdefghijklmnopqrstuv"}, Pairing: models.PairingStatus{Code: "113 134 323"}}
	c.Update(st)
	if d := readCodeFile(t, path); d.Pairing != "113 134 323" || d.Password != "demo-pass" {
		t.Fatalf("%+v", d)
	}
	st.Pairing.Code = "222 333 444"
	c.Update(st)
	if d := readCodeFile(t, path); d.Pairing != "222 333 444" {
		t.Fatalf("%+v", d)
	}
}
