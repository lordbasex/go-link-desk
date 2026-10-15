// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package mcpserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ServerName is go-link Desk's entry in an assistant's MCP settings.
const ServerName = "go-link-desk"

// Client is an AI assistant that runs MCP servers from a settings file.
type Client struct {
	ID   string
	Name string
	// Dir exists when the assistant is installed; File is its MCP settings.
	Dir  string
	File string
	// stdioType: the entry says "type": "stdio" (Claude Code's).
	stdioType bool
}

// Clients are the assistants go-link Desk knows, for this user (home and,
// on Windows, %APPDATA%).
func Clients(goos, home, appData string) []Client {
	desktop := filepath.Join(home, ".config", "Claude")
	switch goos {
	case "darwin":
		desktop = filepath.Join(home, "Library", "Application Support", "Claude")
	case "windows":
		desktop = filepath.Join(appData, "Claude")
	}
	return []Client{
		{ID: "claude-desktop", Name: "Claude", Dir: desktop, File: filepath.Join(desktop, "claude_desktop_config.json")},
		{ID: "claude-code", Name: "Claude Code", Dir: filepath.Join(home, ".claude"), File: filepath.Join(home, ".claude.json"), stdioType: true},
		{ID: "cursor", Name: "Cursor", Dir: filepath.Join(home, ".cursor"), File: filepath.Join(home, ".cursor", "mcp.json")},
	}
}

// serverEntry is go-link Desk in mcpServers.
type serverEntry struct {
	Type    string   `json:"type,omitempty"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// Registration is where go-link Desk stands with one assistant.
type Registration struct {
	Client Client
	// Installed: the assistant is on this computer.
	Installed bool
	// Registered: its settings run this app (with "mcp").
	Registered bool
	Err        error
}

// Sync adds go-link Desk to every installed assistant (exe, the app's own
// path, with "mcp"), or takes it out of them all when on is false. A
// settings file is written only when it changes, keeping the rest of it.
func Sync(clients []Client, exe string, on bool) []Registration {
	out := make([]Registration, 0, len(clients))
	for _, c := range clients {
		r := Registration{Client: c}
		if st, err := os.Stat(c.Dir); err == nil && st.IsDir() {
			r.Installed = true
		}
		var entry []byte
		if on && r.Installed {
			e := serverEntry{Command: exe, Args: []string{"mcp"}}
			if c.stdioType {
				e.Type = "stdio"
			}
			entry, _ = json.Marshal(e)
		}
		r.Err = setEntry(c.File, entry)
		r.Registered = on && r.Installed && r.Err == nil
		out = append(out, r)
	}
	return out
}

// setEntry sets mcpServers[ServerName] in the file to entry (nil: takes it
// out). A missing file is made only to add an entry.
func setEntry(path string, entry []byte) error {
	data, err := os.ReadFile(path)
	mode := fs.FileMode(0o600)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if entry == nil {
			return nil
		}
		data = []byte("{}")
	case err != nil:
		return err
	default:
		if st, err := os.Stat(path); err == nil {
			mode = st.Mode().Perm()
		}
	}
	if len(bytes.TrimSpace(data)) == 0 {
		data = []byte("{}")
	}
	top, err := parseMembers(data)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	servers := members{}
	if raw, ok := top.get("mcpServers"); ok && string(raw) != "null" {
		if servers, err = parseMembers(raw); err != nil {
			return fmt.Errorf("%s: mcpServers: %w", path, err)
		}
	}
	if old, ok := servers.get(ServerName); entry == nil && !ok || ok && entry != nil && sameJSON(old, entry) {
		return nil // as it should be
	}
	servers = servers.set(ServerName, entry)
	top = top.set("mcpServers", servers.encode())
	var out bytes.Buffer
	if err := json.Indent(&out, top.encode(), "", "  "); err != nil {
		return err
	}
	out.WriteByte('\n')
	return writeAtomic(path, out.Bytes(), mode)
}

func sameJSON(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	ja, _ := json.Marshal(x)
	jb, _ := json.Marshal(y)
	return bytes.Equal(ja, jb)
}

// member is a key of a JSON object, in the order the file has them.
type member struct {
	key   string
	value json.RawMessage
}

type members []member

// parseMembers reads an object's keys and values, in order.
func parseMembers(data []byte) (members, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	t, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := t.(json.Delim); !ok || d != '{' {
		return nil, errors.New("not a JSON object")
	}
	var m members
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, err
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		m = append(m, member{key: t.(string), value: v})
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m members) get(key string) (json.RawMessage, bool) {
	for _, x := range m {
		if x.key == key {
			return x.value, true
		}
	}
	return nil, false
}

// set changes key's value, adds it at the end, or takes it out (nil).
func (m members) set(key string, value []byte) members {
	out := make(members, 0, len(m)+1)
	found := false
	for _, x := range m {
		if x.key == key {
			found = true
			if value == nil {
				continue
			}
			x.value = value
		}
		out = append(out, x)
	}
	if !found && value != nil {
		out = append(out, member{key: key, value: value})
	}
	return out
}

func (m members) encode() []byte {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, x := range m {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(x.key)
		b.Write(k)
		b.WriteByte(':')
		var c bytes.Buffer
		if json.Compact(&c, x.value) == nil {
			b.Write(c.Bytes())
		} else {
			b.Write(x.value)
		}
	}
	b.WriteByte('}')
	return b.Bytes()
}

// writeAtomic replaces the file in one step, so the assistant never reads
// half of it.
func writeAtomic(path string, data []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".go-link-desk-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
