// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package netprobe

import (
	"os/exec"
	"strconv"
	"syscall"
)

// traceCommands: Windows' tracert, without name lookups.
func traceCommands(dst string) [][]string {
	return [][]string{{"tracert", "-d", "-h", strconv.Itoa(maxHops), "-w", "800", dst}}
}

// hideWindow keeps tracert's console from flashing up.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
