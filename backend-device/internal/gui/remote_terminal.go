// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/viewer"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/remoteview"
)

// A session with go-link-desk-terminal: the window shows a page with
// xterm.js (terminal/, the same library as the web's) in place of the
// picture. Go hands it the terminals' output and list, and takes what is
// typed and asked for from it.

var (
	//go:embed terminal/page.html
	termPageHTML string
	//go:embed terminal/xterm.js
	termXtermJS string
	//go:embed terminal/xterm.css
	termXtermCSS string
	//go:embed terminal/addon-fit.js
	termFitJS string
)

// Text sizes a terminal may have; 14 is the normal one.
var termFonts = []int{11, 12, 13, 14, 16, 18, 20, 24}

const termFontNormal = 14

// termFlushEvery batches a terminal's output into the page.
const termFlushEvery = 16 * time.Millisecond

// terminalPage is the page, with the library and the texts in it.
func terminalPage() string {
	labels, _ := json.Marshal(map[string]string{
		"newTab":      L("New terminal"),
		"close":       L("Close “{name}” (ends its shell)"),
		"renameField": L("Terminal name"),
		"newOutput":   L("new output"),
		"resumed":     L("Resumed: {n} terminals were still open."),
		"resumedOne":  L("Resumed: the terminal was still open."),
		"readOnly":    L("You can watch this terminal, but not type: the computer lets you only see."),
	})
	r := strings.NewReplacer(
		"/*XTERM_CSS*/", termXtermCSS,
		"/*XTERM_JS*/", termXtermJS,
		"/*FIT_JS*/", termFitJS,
		"/*LABELS*/{}", string(labels),
	)
	return r.Replace(termPageHTML)
}

// termState is a remote's terminal side.
type termState struct {
	mu      sync.Mutex
	ready   bool     // the page said it is ready
	pending []string // scripts for the page until then
	out     map[uint32][]byte
	order   []uint32
	timer   *time.Timer
	font    int
	tabs    bool
}

// startTerminal turns the window into a terminal (once).
func (r *remote) startTerminal(h viewer.HostInfo) {
	r.mu.Lock()
	if r.term != nil {
		r.mu.Unlock()
		return
	}
	r.term = &termState{out: map[uint32][]byte{}, font: r.u.terminalFont(), tabs: h.TerminalTabs}
	r.mu.Unlock()
	r.view.Terminal(terminalPage())
	for _, t := range []string{"quality", "sound", "chat", "record"} {
		r.view.ToolState(t, false, false)
	}
}

// js runs a script in the page, or once it is ready.
func (r *remote) js(script string) {
	r.mu.Lock()
	t := r.term
	r.mu.Unlock()
	if t == nil {
		return
	}
	t.mu.Lock()
	if !t.ready {
		t.pending = append(t.pending, script)
		t.mu.Unlock()
		return
	}
	t.mu.Unlock()
	r.view.TerminalJS(script)
}

// onTerminalOutput keeps a terminal's output for the next flush.
func (r *remote) onTerminalOutput(id uint32, data []byte) {
	r.mu.Lock()
	t := r.term
	r.mu.Unlock()
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.out[id]; !ok {
		t.order = append(t.order, id)
	}
	t.out[id] = append(t.out[id], data...)
	if t.timer == nil && t.ready {
		t.timer = time.AfterFunc(termFlushEvery, r.flushTerminal)
	}
}

// flushTerminal hands the page what came, terminal by terminal, in order.
func (r *remote) flushTerminal() {
	r.mu.Lock()
	t := r.term
	r.mu.Unlock()
	t.mu.Lock()
	out, order := t.out, t.order
	t.out, t.order, t.timer = map[uint32][]byte{}, nil, nil
	t.mu.Unlock()
	var b strings.Builder
	for _, id := range order {
		fmt.Fprintf(&b, "gl.out(%d,%q);", id, base64.StdEncoding.EncodeToString(out[id]))
	}
	if b.Len() > 0 {
		r.view.TerminalJS(b.String())
	}
}

func (r *remote) onTerminalTabs(tabs []viewer.TerminalTab, resumed bool) {
	b, _ := json.Marshal(tabs)
	r.js(fmt.Sprintf("gl.tabs(%s,%t);", b, resumed))
}

func (r *remote) onTerminalExit(id uint32, code int) {
	r.mu.Lock()
	t := r.term
	r.mu.Unlock()
	// A computer with terminals closes that one only (its list says so);
	// an older one ends the session after it.
	if t != nil && !t.tabs {
		if code == 0 {
			r.notify(L("The shell ended."))
		} else {
			r.notify(Lf("The shell ended (exit code %d).", code))
		}
	}
}

// onTerminalMessage takes a message of the page.
func (r *remote) onTerminalMessage(msg string) {
	var m struct {
		T    string `json:"t"`
		ID   uint32 `json:"id"`
		Data string `json:"data"`
		Cols int    `json:"cols"`
		Rows int    `json:"rows"`
		Name string `json:"name"`
		Text string `json:"text"`
	}
	if json.Unmarshal([]byte(msg), &m) != nil {
		return
	}
	r.mu.Lock()
	t := r.term
	control := r.perms.Control
	r.mu.Unlock()
	if t == nil {
		return
	}
	switch m.T {
	case "ready":
		t.mu.Lock()
		t.ready = true
		pending := t.pending
		t.pending = nil
		font, tabs := t.font, t.tabs
		t.mu.Unlock()
		r.view.TerminalJS(fmt.Sprintf("gl.font(%d);gl.control(%t);gl.start(%t);%s", font, control, tabs, strings.Join(pending, "")))
		r.flushTerminal()
	case "input":
		if b, err := base64.StdEncoding.DecodeString(m.Data); err == nil {
			r.s.SendTerminal(m.ID, b)
		}
	case "size":
		r.s.SendTerminalSize(m.Cols, m.Rows)
	case "open":
		r.s.OpenTerminal("")
	case "close":
		r.s.CloseTerminal(m.ID)
	case "rename":
		r.s.RenameTerminal(m.ID, m.Name)
	case "copy":
		if m.Text != "" {
			r.board.SetText(m.Text)
		}
	case "paste":
		if text, ok := r.board.Text(); ok && text != "" {
			r.s.SendTerminal(m.ID, []byte(text))
		}
	}
}

// terminalFontMenu is the monitor button's menu for a terminal: its text size.
func (r *remote) terminalFontMenu() []remoteview.MenuItem {
	r.mu.Lock()
	font := termFontNormal
	if r.term != nil {
		r.term.mu.Lock()
		font = r.term.font
		r.term.mu.Unlock()
	}
	r.mu.Unlock()
	return []remoteview.MenuItem{
		{Title: Lf("Text size: %d", font), Disabled: true},
		{ID: "font:+", Title: L("A+  Bigger text")},
		{ID: "font:-", Title: L("A−  Smaller text")},
		{Separator: true},
		{ID: "font:0", Title: L("Normal size"), Checked: font == termFontNormal},
	}
}

// terminalKeysMenu is the Keys button's menu for a terminal: what a
// keyboard may not send easily.
func (r *remote) terminalKeysMenu() []remoteview.MenuItem {
	return []remoteview.MenuItem{
		{ID: "tkey:esc", Title: "Esc"},
		{ID: "tkey:tab", Title: "Tab"},
		{ID: "tkey:ctrl-c", Title: L("Ctrl+C (interrupt)")},
		{ID: "tkey:ctrl-d", Title: L("Ctrl+D (end of input)")},
		{ID: "tkey:ctrl-z", Title: L("Ctrl+Z (suspend)")},
		{ID: "tkey:ctrl-l", Title: L("Ctrl+L (clear)")},
		{Separator: true},
		{ID: "type-clipboard", Title: L("Type the clipboard")},
	}
}

var termKeys = map[string]string{
	"tkey:esc": "\x1b", "tkey:tab": "\t", "tkey:ctrl-c": "\x03", "tkey:ctrl-d": "\x04", "tkey:ctrl-z": "\x1a", "tkey:ctrl-l": "\x0c",
}

// chooseTerminal takes a menu choice for a terminal; false if it is not one.
func (r *remote) chooseTerminal(id string) bool {
	r.mu.Lock()
	t := r.term
	r.mu.Unlock()
	if t == nil {
		return false
	}
	switch {
	case strings.HasPrefix(id, "font:"):
		t.mu.Lock()
		t.font = nextFont(t.font, strings.TrimPrefix(id, "font:"))
		font := t.font
		t.mu.Unlock()
		r.u.setTerminalFont(font)
		r.js(fmt.Sprintf("gl.font(%d);", font))
	case strings.HasPrefix(id, "tkey:"):
		if seq, ok := termKeys[id]; ok {
			r.js(fmt.Sprintf("gl.type(%q);gl.focus();", base64.StdEncoding.EncodeToString([]byte(seq))))
		}
	case id == "type-clipboard":
		if text, ok := r.board.Text(); ok && text != "" {
			r.js(fmt.Sprintf("gl.type(%q);gl.focus();", base64.StdEncoding.EncodeToString([]byte(text))))
		}
	default:
		return false
	}
	return true
}

// nextFont is one size bigger ("+"), smaller ("-"), or the normal one.
func nextFont(font int, step string) int {
	if step == "0" {
		return termFontNormal
	}
	i := 3
	for j, f := range termFonts {
		if f == font {
			i = j
		}
	}
	if step == "+" {
		i++
	} else {
		i--
	}
	return termFonts[max(0, min(len(termFonts)-1, i))]
}

// terminalFont is the text size chosen for terminals (kept in the app's
// preferences, as the browser keeps its own).
func (u *ui) terminalFont() int {
	if u.app == nil {
		return termFontNormal
	}
	n := u.app.Preferences().IntWithFallback("terminal-font", termFontNormal)
	for _, f := range termFonts {
		if f == n {
			return n
		}
	}
	return termFontNormal
}

func (u *ui) setTerminalFont(n int) {
	if u.app != nil {
		u.app.Preferences().SetInt("terminal-font", n)
	}
}

// isTerminal: this session is with go-link-desk-terminal.
func (r *remote) isTerminal() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.term != nil
}

// onTerminalReset: a new connection replays every history; what was kept goes.
func (r *remote) onTerminalReset() {
	r.mu.Lock()
	t := r.term
	r.mu.Unlock()
	if t == nil {
		return
	}
	t.mu.Lock()
	t.out, t.order = map[uint32][]byte{}, nil
	t.mu.Unlock()
	r.js("gl.reset();")
}
