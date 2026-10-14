// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"testing"
	"time"
)

// Captures are named without spaces, after the computer and the time.
func TestCaptureNamesHaveNoSpaces(t *testing.T) {
	at := time.Date(2026, 10, 14, 15, 4, 5, 0, time.Local)
	for host, want := range map[string]string{
		"Office Mac":                  "go-link-desk-office-mac-2026-10-14-150405.png",
		` a/b\c:*?"<>|` + "\u202e d ": "go-link-desk-a-b-c-d-2026-10-14-150405.png",
		"Café de Ana":                 "go-link-desk-café-de-ana-2026-10-14-150405.png",
		"":                            "go-link-desk-2026-10-14-150405.png",
	} {
		if got := captureName(host, at, "png"); got != want {
			t.Errorf("captureName(%q) = %q, want %q", host, got, want)
		}
	}
}
