// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package updater

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
)

// plan is how this copy of go-link Desk is updated: which release file it
// takes and how that file goes in.
type plan interface {
	asset(tag string) string
	install(ctx context.Context, path string) (restart func() error, err error)
}

// bare is a version without its v: 1.3.0.
func bare(tag string) string { return strings.TrimPrefix(tag, "v") }

// afterExit runs a command once this process has quit, from a shell of
// its own (macOS and Linux).
func afterExit(pid int, name string, args ...string) func() error {
	return func() error {
		script := `while kill -0 ` + strconv.Itoa(pid) + ` 2>/dev/null; do sleep 0.2; done; exec "$@"`
		cmd := exec.Command("/bin/sh", append([]string{"-c", script, "sh", name}, args...)...)
		detach(cmd)
		return cmd.Start()
	}
}
