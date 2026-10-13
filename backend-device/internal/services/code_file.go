// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
)

// CodeFile keeps a demonstration host's codes in a file as they change
// (--test-code-file): the session code whenever the host reconnects to
// signalhub, the pairing code every few minutes and once redeemed. A web
// page (desk.go-link.org/demo) shows them, with the unattended password,
// to app reviewers and anyone trying go-link Desk. Only for a host that
// shows the test card and accepts everyone.
type CodeFile struct {
	Path     string
	WebURL   string // the /c/<invite> link's site
	Name     string // the computer's name viewers see
	Password string // the unattended password ("": none)
	Log      *slog.Logger

	mu    sync.Mutex
	wrote bool
	last  string // the codes last written
}

// codeFileData is the file: online is false while the host has no code
// (reconnecting).
type codeFileData struct {
	Online bool   `json:"online"`
	Code   string `json:"code,omitempty"` // "915 355 636"
	Link   string `json:"link,omitempty"` // https://desk.go-link.org/c/<invite>
	Name   string `json:"name,omitempty"`
	// Pairing links the computer for unattended access ("113 134 323"),
	// with Password.
	Pairing  string `json:"pairing,omitempty"`
	Password string `json:"password,omitempty"`
	Updated  string `json:"updated"` // RFC 3339, UTC
}

// Update takes the host's status (StatusService.OnChange) and writes the
// file when the code changed.
func (c *CodeFile) Update(st models.Status) {
	code, invite := "", ""
	if st.Session != nil {
		code, invite = st.Session.Code, st.Session.Invite
	}
	pairing := st.Pairing.Code
	key := code + "|" + pairing
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.wrote && c.last == key {
		return
	}
	d := codeFileData{Online: code != "", Name: c.Name, Updated: time.Now().UTC().Format(time.RFC3339)}
	if code != "" {
		d.Code = FormatCode(code)
		if invite != "" {
			d.Link = strings.TrimRight(c.WebURL, "/") + "/c/" + invite
		}
		if pairing != "" && c.Password != "" {
			d.Pairing, d.Password = pairing, c.Password
		}
	}
	if err := writeAtomic(c.Path, d); err != nil {
		c.Log.Warn("cannot write the code file", "path", c.Path, "err", err)
		return
	}
	c.wrote, c.last = true, key
	c.Log.Info("code file written", "path", c.Path, "online", d.Online)
}

// writeAtomic writes the file whole: a reader never sees half of it.
func writeAtomic(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".code-*.json")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
