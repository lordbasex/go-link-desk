// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"github.com/lordbasex/go-link-desk/backend-device/pkg/signalclient"
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

// A demonstration host shows the test card and only logs the input.
func TestCodeFileOnlyOnADemonstrationHost(t *testing.T) {
	demo := []string{"--test-code-file", "x", "--test-input-log", "--test-clipboard-file", "c", "--test-files-folder", "d", "--test-auto-accept", "--source", "testpattern"}
	if f, err := parseFlags(demo); err != nil || f.codeFile != "x" || f.filesDir != "d" {
		t.Fatalf("%+v %v", f, err)
	}
	// Each piece that keeps the computer's own things out is required.
	for _, drop := range []string{"--test-input-log", "--test-clipboard-file", "--test-files-folder"} {
		var args []string
		for i := 0; i < len(demo); i++ {
			if demo[i] == drop {
				if drop != "--test-input-log" {
					i++ // and its value
				}
				continue
			}
			args = append(args, demo[i])
		}
		if _, err := parseFlags(args); err == nil {
			t.Fatalf("a demonstration host without %s was allowed", drop)
		}
	}
	// A demonstration computer shows its real screen and takes the input.
	screen := []string{"--test-code-file", "x", "--test-clipboard-file", "c", "--test-files-folder", "d", "--test-auto-accept", "--source", "screen", "--test-demo-screen"}
	if f, err := parseFlags(screen); err != nil || !f.demoScreen || f.inputLog {
		t.Fatalf("%+v %v", f, err)
	}
	if _, err := parseFlags(screen[:len(screen)-1]); err == nil && !allowAutoAcceptScreen {
		t.Fatal("the real screen to everyone without --test-demo-screen was allowed")
	}
	if f, err := parseFlags(append(screen, "--test-name", "go-link Desk Demo")); err != nil || f.name == "" {
		t.Fatalf("%+v %v", f, err)
	}
	if f, err := parseFlags(append(screen, "--test-relay-only")); err != nil || !f.relayOnly {
		t.Fatalf("%+v %v", f, err)
	}
	if _, err := parseFlags([]string{"--test-relay-only"}); err == nil {
		t.Fatal("--test-relay-only outside a demonstration host was allowed")
	}
	if _, err := parseFlags([]string{"--test-demo-screen", "--test-auto-accept", "--source", "screen"}); err == nil {
		t.Fatal("--test-demo-screen outside a demonstration host was allowed")
	}
	if _, err := parseFlags([]string{"--test-files-folder", "d", "--test-auto-accept", "--source", "testpattern"}); err == nil {
		t.Fatal("a files folder outside a demonstration host was allowed")
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
		if got := signalclient.IsLoopback(in); got != want {
			t.Errorf("signalclient.IsLoopback(%q) = %v", in, got)
		}
	}
}
