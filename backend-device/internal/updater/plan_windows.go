// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package updater

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// winPlan runs the release's installer silently: it closes this copy,
// installs over it and opens the new one (/UPDATE). A portable copy (the
// .zip) is updated by hand.
type winPlan struct{}

func (u *Updater) plan() (plan, error) {
	exe, err := u.exe()
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(exe)
	want := filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "go-link Desk")
	if os.Getenv("LOCALAPPDATA") == "" || !strings.EqualFold(filepath.Clean(dir), filepath.Clean(want)) {
		return nil, ErrManual
	}
	if _, err := os.Stat(filepath.Join(dir, "uninstall.exe")); err != nil {
		return nil, ErrManual
	}
	return winPlan{}, nil
}

func (winPlan) asset(tag string) string {
	return "go-link-desk-" + tag + "-windows-" + arch() + "-setup.exe"
}

func (winPlan) install(_ context.Context, setup string) (func() error, error) {
	cmd := exec.Command(setup, "/S", "/UPDATE")
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return nil, nil // the installer opens the new version
}

func detach(cmd *exec.Cmd) {
	const detached, newGroup = 0x00000008, 0x00000200
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detached | newGroup}
}
