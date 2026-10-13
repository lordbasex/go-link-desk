// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package updater

import (
	"errors"
	"path/filepath"
	"testing"
)

// testExe is the user's own copy (install.sh's ~/.local/bin).
func testExe(t *testing.T) string {
	home := t.TempDir()
	t.Setenv("HOME", home)
	return filepath.Join(home, ".local", "bin", "go-link-desk")
}

func TestLinuxPlans(t *testing.T) {
	p, err := (&Updater{Exe: testExe(t)}).plan()
	if err != nil {
		t.Fatal(err)
	}
	if got := p.asset("v1.3.1"); got != "go-link-desk-v1.3.1-linux-"+arch()+".tar.gz" {
		t.Errorf("asset %s", got)
	}
	for kind, want := range map[string]string{
		"deb":    "go-link-desk_1.3.1_amd64.deb",
		"rpm":    "go-link-desk-1.3.1-1.x86_64.rpm",
		"pacman": "go-link-desk-1.3.1-1-x86_64.pkg.tar.zst",
	} {
		if arch() != "amd64" {
			break
		}
		if got := (&pkgPlan{kind: kind}).asset("v1.3.1"); got != want {
			t.Errorf("%s: %s, want %s", kind, got, want)
		}
	}
	if _, err := (&Updater{Exe: "/opt/elsewhere/go-link-desk"}).plan(); !errors.Is(err, ErrManual) {
		t.Errorf("a copy elsewhere: %v", err)
	}
}
