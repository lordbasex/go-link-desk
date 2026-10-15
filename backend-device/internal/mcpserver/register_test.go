// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package mcpserver

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Turning assistants on adds go-link Desk to the ones installed (and
// only those), keeping everything else in their settings and its order;
// off takes it out again; an update to the app's path is followed.
func TestSyncAddsAndRemovesTheEntry(t *testing.T) {
	home := t.TempDir()
	clients := Clients("darwin", home, "")
	desktop, code, cursor := clients[0], clients[1], clients[2]
	if err := os.MkdirAll(desktop.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(code.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Claude Code's file, with other servers and settings of its own.
	before := `{"numStartups": 7, "mcpServers": {"other": {"command": "x"}}, "projects": {"/a": {"allowedTools": []}}}`
	if err := os.WriteFile(code.File, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}

	exe := "/Applications/go-link Desk.app/Contents/MacOS/go-link-desk"
	regs := Sync(clients, exe, true)
	if !regs[0].Registered || !regs[1].Registered || regs[2].Registered || regs[2].Installed {
		t.Fatalf("registrations: %+v", regs)
	}
	if _, err := os.Stat(cursor.File); !os.IsNotExist(err) {
		t.Fatal("Cursor is not installed, but its settings were made")
	}
	var c struct {
		NumStartups int                        `json:"numStartups"`
		Servers     map[string]json.RawMessage `json:"mcpServers"`
		Projects    map[string]any             `json:"projects"`
	}
	read := func(path string, v any) string {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, v); err != nil {
			t.Fatalf("%s: %v\n%s", path, err, b)
		}
		return string(b)
	}
	text := read(code.File, &c)
	if c.NumStartups != 7 || c.Projects["/a"] == nil || c.Servers["other"] == nil {
		t.Fatalf("Claude Code's own settings were lost:\n%s", text)
	}
	var entry bytes.Buffer
	json.Compact(&entry, c.Servers[ServerName])
	if entry.String() != `{"type":"stdio","command":"`+exe+`","args":["mcp"]}` {
		t.Fatalf("entry: %s", c.Servers[ServerName])
	}
	if strings.Index(text, "numStartups") > strings.Index(text, "projects") {
		t.Fatalf("the keys' order changed:\n%s", text)
	}
	var d struct {
		Servers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	read(desktop.File, &d)
	if e := d.Servers[ServerName]; e.Command != exe || len(e.Args) != 1 {
		t.Fatalf("Claude's entry: %+v", e)
	}

	// Again: nothing to write.
	st1, _ := os.Stat(code.File)
	Sync(clients, exe, true)
	if st2, _ := os.Stat(code.File); !st2.ModTime().Equal(st1.ModTime()) {
		t.Fatal("an unchanged file was written again")
	}

	// The app moved: the entry follows.
	Sync(clients, "/Users/a/Applications/go-link-desk", true)
	read(desktop.File, &d)
	if d.Servers[ServerName].Command != "/Users/a/Applications/go-link-desk" {
		t.Fatalf("not updated: %+v", d.Servers)
	}

	// Off: out of every file, the others stay.
	regs = Sync(clients, exe, false)
	for _, r := range regs {
		if r.Registered || r.Err != nil {
			t.Fatalf("off: %+v", r)
		}
	}
	c.Servers = nil
	read(code.File, &c)
	if _, ok := c.Servers[ServerName]; ok || c.Servers["other"] == nil || c.NumStartups != 7 {
		t.Fatalf("off: %+v", c)
	}
}

// A settings file that is not JSON is left alone, with the error told.
func TestSyncLeavesABrokenFileAlone(t *testing.T) {
	home := t.TempDir()
	clients := Clients("linux", home, "")[2:]
	os.MkdirAll(clients[0].Dir, 0o700)
	os.WriteFile(clients[0].File, []byte("{not json"), 0o600)
	regs := Sync(clients, "/usr/bin/go-link-desk", true)
	if regs[0].Err == nil || regs[0].Registered {
		t.Fatalf("%+v", regs[0])
	}
	if b, _ := os.ReadFile(clients[0].File); string(b) != "{not json" {
		t.Fatal("the broken file was changed")
	}
	if got := Clients("windows", `C:\Users\a`, `C:\Users\a\AppData\Roaming`)[0].File; got != filepath.Join(`C:\Users\a\AppData\Roaming`, "Claude", "claude_desktop_config.json") {
		t.Fatal(got)
	}
}
