// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"testing"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
)

func TestParseFlags(t *testing.T) {
	f, err := parseFlags([]string{"--headless", "--source", "testpattern", "--server-signaling", "ws://127.0.0.1:8090/ws"})
	if err != nil || !f.headless || f.source != "testpattern" || f.signal != "ws://127.0.0.1:8090/ws" {
		t.Fatalf("%+v %v", f, err)
	}
	if f, err := parseFlags(nil); err != nil || f.source != "screen" {
		t.Fatalf("defaults %+v %v", f, err)
	}
	if _, err := parseFlags([]string{"--source", "camera"}); err == nil {
		t.Fatal("an unknown source was accepted")
	}
}

func TestInputLogOnlyWithAutoAccept(t *testing.T) {
	if _, err := parseFlags([]string{"--test-input-log", "--source", "testpattern"}); err == nil {
		t.Fatal("logging input without auto-accept was allowed")
	}
	if f, err := parseFlags([]string{"--test-input-log", "--test-auto-accept", "--source", "testpattern"}); err != nil || !f.inputLog {
		t.Fatalf("%+v %v", f, err)
	}
}

func TestClipboardFileOnlyWithInputLog(t *testing.T) {
	if _, err := parseFlags([]string{"--test-clipboard-file", "x", "--test-auto-accept", "--source", "testpattern"}); err == nil {
		t.Fatal("a clipboard file without the input log was allowed")
	}
	if f, err := parseFlags([]string{"--test-clipboard-file", "x", "--test-input-log", "--test-auto-accept", "--source", "testpattern"}); err != nil || f.clipboardFile != "x" {
		t.Fatalf("%+v %v", f, err)
	}
}

func TestAutoAcceptOnlyWithTheTestCard(t *testing.T) {
	if allowAutoAcceptScreen {
		t.Skip("a measurement build (desk_test_screen)")
	}
	// Skipping consent must never show anyone a real screen.
	if _, err := parseFlags([]string{"--test-auto-accept"}); err == nil {
		t.Fatal("auto-accept with the screen was allowed")
	}
	if f, err := parseFlags([]string{"--test-auto-accept", "--source", "testpattern"}); err != nil || !f.autoAccept {
		t.Fatalf("%+v %v", f, err)
	}
}

func TestResolveSignalURL(t *testing.T) {
	if got, _ := resolveSignalURL("", models.Config{}); got != models.DefaultSignalURL {
		t.Fatalf("default %q", got)
	}
	if got, _ := resolveSignalURL("", models.Config{SignalURL: "wss://own.example/ws"}); got != "wss://own.example/ws" {
		t.Fatalf("config %q", got)
	}
	if got, _ := resolveSignalURL("ws://127.0.0.1:8090/ws", models.Config{SignalURL: "wss://own.example/ws"}); got != "ws://127.0.0.1:8090/ws" {
		t.Fatalf("flag %q", got)
	}
	if _, err := resolveSignalURL("http://nope", models.Config{}); err == nil {
		t.Fatal("a non-WebSocket URL was accepted")
	}
}

func TestIsLoopback(t *testing.T) {
	for in, want := range map[string]bool{
		"ws://127.0.0.1:8090/ws":      true,
		"ws://localhost:8090/ws":      true,
		"ws://[::1]:8090/ws":          true,
		"wss://signal.go-link.org/ws": false,
	} {
		if got := isLoopback(in); got != want {
			t.Errorf("isLoopback(%q) = %v", in, got)
		}
	}
}
