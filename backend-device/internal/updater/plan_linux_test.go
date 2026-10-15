// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package updater

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
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

// The package manager runs as root on the package: it must get the bytes
// Download checked, not ones this user put there before the password
// prompt. A stand-in pkexec runs the command as is; a stand-in dpkg keeps
// what it was given.
func TestPackageCheckedAgainAsRoot(t *testing.T) {
	bin := t.TempDir()
	seen := filepath.Join(t.TempDir(), "seen")
	_ = os.WriteFile(filepath.Join(bin, "pkexec"), []byte("#!/bin/sh\nexec \"$@\"\n"), 0o755)
	_ = os.WriteFile(filepath.Join(bin, "dpkg"), []byte("#!/bin/sh\nfor last; do :; done\ncat \"$last\" > "+seen+"\n"), 0o755)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	good := []byte("the signed package")
	sum := sha256.Sum256(good)
	pkg := filepath.Join(t.TempDir(), "go-link-desk_9.9.9_amd64.deb")
	p := &pkgPlan{kind: "deb", exe: "/usr/bin/go-link-desk"}

	_ = os.WriteFile(pkg, []byte("swapped after the check"), 0o600)
	if _, err := p.install(context.Background(), pkg, hex.EncodeToString(sum[:])); !errors.Is(err, ErrSignature) {
		t.Fatalf("a swapped package: %v", err)
	}
	if _, err := os.Stat(seen); err == nil {
		t.Fatal("dpkg ran on the swapped package")
	}

	_ = os.WriteFile(pkg, good, 0o600)
	if _, err := p.install(context.Background(), pkg, hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(seen); !bytes.Equal(b, good) {
		t.Fatalf("dpkg got %q", b)
	}
}
