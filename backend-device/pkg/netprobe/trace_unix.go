// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !windows

package netprobe

import (
	"os/exec"
	"strconv"
)

// traceCommands: macOS's traceroute; on Linux tracepath (iputils, no
// privileges) or traceroute when there is one.
func traceCommands(dst string) [][]string {
	m := strconv.Itoa(maxHops)
	return [][]string{
		{"traceroute", "-n", "-m", m, "-q", "1", "-w", "1", dst},
		{"tracepath", "-n", "-m", m, dst},
	}
}

func hideWindow(*exec.Cmd) {}
