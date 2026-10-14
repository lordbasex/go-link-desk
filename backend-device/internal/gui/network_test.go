// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import "testing"

func TestTheConnectionTestPage(t *testing.T) {
	if got := networkPage("https://desk.go-link.org/"); got != "https://desk.go-link.org/network" {
		t.Fatal(got)
	}
	if got := networkPage(""); got != "https://desk.go-link.org/network" {
		t.Fatal(got)
	}
	if got := signalHost("wss://signal.example.org/ws"); got != "signal.example.org" {
		t.Fatal(got)
	}
	if got := signalHost(""); got != "signal.go-link.org" {
		t.Fatal(got)
	}
}
