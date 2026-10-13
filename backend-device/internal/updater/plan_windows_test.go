// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package updater

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// testExe is the installer's copy, with its uninstaller.
func testExe(t *testing.T) string {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	dir := filepath.Join(local, "Programs", "go-link Desk")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "uninstall.exe"), nil, 0o644)
	return filepath.Join(dir, "go-link-desk.exe")
}

func TestWindowsPlan(t *testing.T) {
	p, err := (&Updater{Exe: testExe(t)}).plan()
	if err != nil {
		t.Fatal(err)
	}
	if got := p.asset("v1.3.1"); got != "go-link-desk-v1.3.1-windows-"+arch()+"-setup.exe" {
		t.Errorf("asset %s", got)
	}
	if _, err := (&Updater{Exe: filepath.Join(t.TempDir(), "go-link-desk.exe")}).plan(); !errors.Is(err, ErrManual) {
		t.Errorf("a portable copy: %v", err)
	}
}
