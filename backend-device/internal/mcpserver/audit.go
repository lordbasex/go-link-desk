// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package mcpserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuditFile is the assistants' activity log, next to device.json: one
// JSON line per tool call (never a password).
const AuditFile = "assistant-activity.log"

// maxAudit is the log's size before it starts again (the old one is kept
// as .1).
const maxAudit = 5 << 20

// Audit appends what the assistants did.
type Audit struct {
	mu   sync.Mutex
	path string
}

// NewAudit logs to path ("" logs nothing).
func NewAudit(path string) *Audit { return &Audit{path: path} }

// AuditEntry is one tool call.
type AuditEntry struct {
	Time      time.Time `json:"time"`
	Assistant string    `json:"assistant"`
	Tool      string    `json:"tool"`
	Computer  string    `json:"computer,omitempty"`
	// Detail is what was asked: the command, the text typed.
	Detail string `json:"detail,omitempty"`
	// Result: "ok", or the error.
	Result string `json:"result"`
}

// maxDetail bounds what is kept of a command or text.
const maxDetail = 2000

// Log appends an entry.
func (a *Audit) Log(e AuditEntry) {
	if a == nil || a.path == "" {
		return
	}
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	if r := []rune(e.Detail); len(r) > maxDetail {
		e.Detail = string(r[:maxDetail]) + "…"
	}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if st, err := os.Stat(a.path); err == nil && st.Size() > maxAudit {
		_ = os.Rename(a.path, a.path+".1")
	}
	if os.MkdirAll(filepath.Dir(a.path), 0o700) != nil {
		return
	}
	f, err := os.OpenFile(a.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}
