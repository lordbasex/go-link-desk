// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// ReasonOutsideHome is the refusal of anything above the home folder.
const ReasonOutsideHome = "outside_home"

// CheckHomePath refuses what can't be a path in the home folder, before any
// disk is touched: an absolute path, a "..", a "~", a NUL. rel is relative
// to the home folder.
func CheckHomePath(rel string) bool {
	if strings.ContainsRune(rel, 0) || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, "\\") || strings.HasPrefix(rel, "~") {
		return false
	}
	for _, seg := range strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == '\\' }) {
		if seg == ".." {
			return false
		}
	}
	return true
}

// InsideHome reports whether p, symbolic links followed, is the home folder
// or under it. A path that doesn't exist yet is judged by the closest folder
// that does.
func InsideHome(home, p string) bool {
	root, err := filepath.EvalSymlinks(home)
	if err != nil {
		return false
	}
	for at := filepath.Clean(p); ; at = filepath.Dir(at) {
		real, err := filepath.EvalSymlinks(at)
		if err == nil {
			return real == root || strings.HasPrefix(real, root+string(filepath.Separator))
		}
		// A link that points nowhere leaves as much as one that points out.
		if _, lerr := os.Lstat(at); lerr == nil || filepath.Dir(at) == at {
			return false
		}
	}
}

// FilesGuard is the maintainer's rule for visitors in front of FilesService:
// they see only the user's home folder, nothing above it, and what they
// send goes to ~/Downloads. FilesService refuses most of this by itself;
// this holds the rule in one place that a sandbox can't replace, with the
// answer the app words (ReasonOutsideHome), and adds what FilesService
// leaves to its host: a Downloads folder that is a link out of the home, and
// listing entries that lead out of it.
type FilesGuard struct {
	Home      string
	Downloads string
	Send      func(peerID string, isString bool, data []byte) bool
	Next      func(peerID string, isString bool, data []byte)
	Log       *slog.Logger
}

// HandleFiles is FilesService.Handle with the rule in front of it.
func (g *FilesGuard) HandleFiles(peerID string, isString bool, data []byte) {
	if isString {
		var m filesMsg
		if err := json.Unmarshal(data, &m); err == nil && g.refused(peerID, m) {
			return
		}
	}
	g.Next(peerID, isString, data)
}

func (g *FilesGuard) refused(peerID string, m filesMsg) bool {
	out := func(reply string, ok map[string]any) bool {
		g.log("a visitor asked for something outside the home folder", "peer_id", peerID, "type", m.Type, "path", m.Path)
		ok["type"], ok["id"] = reply, m.ID
		b, err := json.Marshal(ok)
		if err == nil {
			g.Send(peerID, true, b)
		}
		return true
	}
	switch m.Type {
	case "list", "download":
		bad := !CheckHomePath(m.Path)
		if !bad {
			if _, err := os.Lstat(filepath.Join(g.Home, filepath.FromSlash(m.Path))); err == nil {
				bad = !InsideHome(g.Home, filepath.Join(g.Home, filepath.FromSlash(m.Path)))
			}
		}
		if !bad {
			return false
		}
		if m.Type == "download" {
			return out("download_error", map[string]any{"error": ReasonOutsideHome})
		}
		return out("files_error", map[string]any{"error": ReasonOutsideHome})
	case "begin":
		if !InsideHome(g.Home, g.Downloads) {
			return out("upload_result", map[string]any{"ok": false, "error": ReasonOutsideHome})
		}
	}
	return false
}

func (g *FilesGuard) log(msg string, args ...any) {
	if g.Log != nil {
		g.Log.Warn(msg, args...)
	}
}

// SendFiles is what FilesService sends through: listings lose the entries
// that lead out of the home folder.
func (g *FilesGuard) SendFiles(peerID string, isString bool, data []byte) bool {
	if isString && bytes.Contains(data, []byte(`"type":"listing"`)) {
		data = g.filter(data)
	}
	return g.Send(peerID, isString, data)
}

func (g *FilesGuard) filter(data []byte) []byte {
	var m struct {
		Type    string      `json:"type"`
		ID      string      `json:"id"`
		Path    string      `json:"path"`
		Entries []fileEntry `json:"entries"`
	}
	if err := json.Unmarshal(data, &m); err != nil || m.Type != "listing" {
		return data
	}
	kept := m.Entries[:0]
	for _, e := range m.Entries {
		if InsideHome(g.Home, filepath.Join(g.Home, filepath.FromSlash(m.Path), e.Name)) {
			kept = append(kept, e)
		}
	}
	m.Entries = kept
	if b, err := json.Marshal(m); err == nil {
		return b
	}
	return data
}
