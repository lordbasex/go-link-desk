// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless && !windows

package gui

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// relaunch opens go-link Desk again (a new server takes effect): a new
// process starts once this one has let go of its lock, and this one
// quits.
func (u *ui) relaunch() {
	exe, err := os.Executable()
	if err != nil {
		u.opts.Logger.Warn("cannot open again", "err", err)
		return
	}
	args := []string{}
	for _, a := range os.Args[1:] {
		if strings.HasPrefix(a, "--server-signaling") || strings.HasPrefix(a, "-server-signaling") || strings.HasPrefix(a, "--connect") {
			continue
		}
		args = append(args, a)
	}
	script := `sleep 2; exec "$0" "$@"`
	cmd := exec.Command("/bin/sh", append([]string{"-c", script, exe}, args...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		u.opts.Logger.Warn("cannot open again", "err", err)
		return
	}
	if u.opts.Quit != nil {
		u.opts.Quit()
	}
}
