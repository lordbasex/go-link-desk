// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package autostart

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

// The entry is a quoted command under the Run key (a key of the test's
// own here, never the user's real one).
func TestEnableAndDisableOnWindows(t *testing.T) {
	runKey = `Software\go-link-desk-test\Run`
	defer func() {
		registry.DeleteKey(registry.CURRENT_USER, runKey)
		registry.DeleteKey(registry.CURRENT_USER, `Software\go-link-desk-test`)
		runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
	}()
	if Enabled() {
		t.Fatal("enabled in an empty key")
	}
	if err := Enable(`relative\desk.exe`); err == nil {
		t.Fatal("a relative path was taken")
	}
	exe := `C:\Program Files\go-link Desk\go-link-desk.exe`
	if err := Enable(exe); err != nil {
		t.Fatal(err)
	}
	if !Enabled() {
		t.Fatal("not enabled")
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := k.GetStringValue(value)
	k.Close()
	if err != nil || got != `"`+exe+`"` {
		t.Fatalf("command %q %v", got, err)
	}
	if err := Disable(); err != nil || Enabled() {
		t.Fatalf("disable: %v", err)
	}
	if err := Disable(); err != nil {
		t.Fatalf("disabling twice: %v", err)
	}
}
