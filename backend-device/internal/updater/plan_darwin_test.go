// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package updater

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// testExe is a program inside an app bundle in a folder the test writes.
func testExe(t *testing.T) string {
	exe := filepath.Join(t.TempDir(), "go-link-desk.app", "Contents", "MacOS", "go-link-desk")
	_ = os.MkdirAll(filepath.Dir(exe), 0o755)
	return exe
}

func TestMacPlan(t *testing.T) {
	u := &Updater{Exe: testExe(t)}
	p, err := u.plan()
	if err != nil {
		t.Fatal(err)
	}
	if got := p.asset("v1.3.1"); got != "go-link-desk-v1.3.1-macos-universal.dmg" {
		t.Errorf("asset %s", got)
	}
	if _, err := (&Updater{Exe: "/usr/local/bin/go-link-desk"}).plan(); !errors.Is(err, ErrManual) {
		t.Errorf("a program outside an app: %v", err)
	}
}
