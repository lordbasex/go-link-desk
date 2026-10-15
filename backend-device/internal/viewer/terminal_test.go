// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !windows

package viewer

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
	"github.com/lordbasex/go-link-desk/backend-device/internal/terminal"
)

// The app's viewer with go-link-desk-terminal: it learns the computer is a
// terminal with tabs, gets each terminal's output by id, types, opens and
// renames terminals, and an exit closes only its own.
func TestATerminalSession(t *testing.T) {
	stream, err := services.NewStreamService(services.StreamConfig{
		IncludeLoopback: true, Logger: quiet, Source: "terminal", Features: []string{"terminal-tabs"},
		DataOnly: true, Files: true, Channels: []string{terminal.Channel}, Welcome: services.SessionMeta{Name: "pi", OS: "linux"},
	}, services.NewICEStore())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = stream.Run(ctx) }()
	sh := terminal.ShellConfig{Path: "/bin/sh", Home: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin", "TERM=xterm-256color", "PS1=$ "}}
	term := terminal.NewService(sh, stream.SendControl, quiet)
	t.Cleanup(term.Close)
	stream.OnChannel(term.Channel)
	stream.OnControl(func(peer, kind string, data []byte) { term.HandleControl(peer, kind, data) })

	var mu sync.Mutex
	out := map[uint32]*bytes.Buffer{}
	var tabs []TerminalTab
	exits := map[uint32]int{}
	rec := &recorder{}
	ev := rec.events()
	ev.TerminalOutput = func(id uint32, b []byte) {
		mu.Lock()
		if out[id] == nil {
			out[id] = &bytes.Buffer{}
		}
		out[id].Write(b)
		mu.Unlock()
	}
	ev.TerminalTabs = func(t []TerminalTab, _ bool) { mu.Lock(); tabs = t; mu.Unlock() }
	ev.TerminalExit = func(id uint32, code int) { mu.Lock(); exits[id] = code; mu.Unlock() }
	s := New(Options{Door: Door{Kind: DoorJoin, Code: "915355636"}, Name: "Ana", Agent: "go-link Desk test", Logger: quiet, Events: ev})
	perms := models.Permissions{View: true, Control: true}
	hub := &fakeHub{t: t, s: s, stream: stream, perms: perms}
	hub.decide = func(h *fakeHub) {
		term.Allow(viewerID, perms)
		h.accept()
	}
	stream.SetSender(hostSender{hub})
	s.Start(hub)
	s.Connected(nil)
	t.Cleanup(s.HangUp)

	text := func(id uint32) string {
		mu.Lock()
		defer mu.Unlock()
		if out[id] == nil {
			return ""
		}
		return out[id].String()
	}
	names := func() string {
		mu.Lock()
		defer mu.Unlock()
		var n []string
		for _, x := range tabs {
			n = append(n, x.Name)
		}
		return strings.Join(n, ",")
	}

	waitFor(t, "a terminal host", func() bool { h := s.Host(); return h.Terminal && h.TerminalTabs && h.Name == "pi" })
	waitFor(t, "the first terminal", func() bool { return names() == "shell" })
	s.SendTerminalSize(100, 30)
	s.SendTerminal(1, []byte("echo hola-$((40+2)); stty size\n"))
	waitFor(t, "its output", func() bool { return strings.Contains(text(1), "hola-42") && strings.Contains(text(1), "30 100") })

	s.OpenTerminal("logs")
	waitFor(t, "a second terminal", func() bool { return names() == "shell,logs" })
	s.RenameTerminal(2, " build ")
	waitFor(t, "the new name", func() bool { return names() == "shell,build" })
	s.SendTerminal(2, []byte("exit 4\n"))
	waitFor(t, "only the second one ended", func() bool {
		mu.Lock()
		code, ok := exits[2]
		mu.Unlock()
		return ok && code == 4 && names() == "shell"
	})
	if s.Status().Kind != StatusLive {
		t.Fatalf("the session ended with a terminal: %+v", s.Status())
	}
}
