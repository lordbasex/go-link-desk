// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux

package autostart

import (
	"os"
	"path/filepath"
	"strings"
)

// Supported reports whether this system has an entry.
const Supported = true

func entryPath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" || !filepath.IsAbs(dir) {
		h, err := home()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(h, ".config")
	}
	return filepath.Join(dir, "autostart", "go-link-desk.desktop"), nil
}

// entry is the XDG autostart file; Exec quotes the path as the Desktop
// Entry specification says.
func entry(exe string) []byte {
	quoted := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", `$`, `\$`).Replace(exe)
	quoted = strings.ReplaceAll(quoted, "%", "%%")
	return []byte("[Desktop Entry]\nType=Application\nName=go-link Desk\nExec=\"" + quoted + "\"\nX-GNOME-Autostart-enabled=true\nTerminal=false\n")
}
