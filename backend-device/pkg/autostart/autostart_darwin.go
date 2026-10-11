// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

package autostart

import (
	"bytes"
	"encoding/xml"
	"path/filepath"
)

// Supported reports whether this system has an entry.
const Supported = true

func entryPath() (string, error) {
	h, err := home()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, "Library", "LaunchAgents", Label+".plist"), nil
}

// entry is the LaunchAgent: run exe at login, in the graphical session
// only. AssociatedBundleIdentifiers makes System Settings › Login Items
// show it under the app's name.
func entry(exe string) []byte {
	var path bytes.Buffer
	_ = xml.EscapeText(&path, []byte(exe))
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + Label + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + path.String() + `</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>LimitLoadToSessionType</key>
	<string>Aqua</string>
	<key>ProcessType</key>
	<string>Interactive</string>
	<key>AssociatedBundleIdentifiers</key>
	<string>` + Label + `</string>
</dict>
</plist>
`)
}
