// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !windows

package autostart

import (
	"os"
	"strings"
	"testing"
)

func TestEnableAndDisable(t *testing.T) {
	if !Supported {
		t.Skip("no autostart entry on this system")
	}
	dir := t.TempDir()
	home = func() (string, error) { return dir, nil }
	t.Setenv("XDG_CONFIG_HOME", "")
	defer func() { home = os.UserHomeDir }()

	if Enabled() {
		t.Fatal("enabled in an empty home")
	}
	if err := Enable("relative/desk"); err == nil {
		t.Fatal("a relative path was taken")
	}
	exe := "/Applications/go-link Desk.app/Contents/MacOS/go-link-desk & \"co\" $HOME"
	if err := Enable(exe); err != nil {
		t.Fatal(err)
	}
	if !Enabled() {
		t.Fatal("not enabled")
	}
	p, _ := entryPath()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if strings.Contains(text, "& \"co\"") || !strings.Contains(text, "go-link-desk") {
		t.Fatalf("the path must be escaped:\n%s", text)
	}
	if err := Disable(); err != nil || Enabled() {
		t.Fatalf("disable: %v", err)
	}
	if err := Disable(); err != nil {
		t.Fatalf("disabling twice: %v", err)
	}
}
