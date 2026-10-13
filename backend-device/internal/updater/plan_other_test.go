// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !darwin && !windows && !linux

package updater

import "testing"

func testExe(t *testing.T) string {
	t.Skip("no updates on this system")
	return ""
}
