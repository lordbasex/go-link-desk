// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/viewer"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/decoder"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/keyring"
)

type decoderI420 = decoder.I420

// fakeHost is a linked computer as the assistant's session sees it: a
// terminal computer whose shell answers what is typed, or a desktop.
type fakeHost struct {
	mu       sync.Mutex
	opts     viewer.Options
	host     viewer.HostInfo
	perms    models.Permissions
	password string // what the computer takes
	tabs     []viewer.TerminalTab
	typed    map[uint32]string
	gotPass  []string
	hidden   bool
	asked    int
	hungUp   bool
}

func (h *fakeHost) ev() viewer.Events { return h.opts.Events }

func (h *fakeHost) Dial(context.Context) {
	go h.ev().Status(viewer.Status{Kind: viewer.StatusPassword})
}

func (h *fakeHost) SendPassword(p string) {
	h.mu.Lock()
	h.gotPass = append(h.gotPass, p)
	ok := p == h.password
	h.mu.Unlock()
	if !ok {
		h.ev().Status(viewer.Status{Kind: viewer.StatusPassword, PasswordError: "wrong", Left: 4})
		return
	}
	h.ev().Status(viewer.Status{Kind: viewer.StatusLive})
	if h.host.TerminalTabs {
		h.mu.Lock()
		h.tabs = []viewer.TerminalTab{{ID: 1, Name: "shell"}}
		tabs := append([]viewer.TerminalTab(nil), h.tabs...)
		h.mu.Unlock()
		h.ev().TerminalTabs(tabs, true)
		h.ev().TerminalOutput(1, []byte("pi@desk-pi:~ $ "))
	}
}

func (h *fakeHost) Status() viewer.Status { return viewer.Status{} }
func (h *fakeHost) Host() viewer.HostInfo { return h.host }
func (h *fakeHost) Permissions() models.Permissions {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.perms
}

// SendTerminal: the shell echoes, and runs the one command it knows.
func (h *fakeHost) SendTerminal(id uint32, data []byte) {
	h.mu.Lock()
	if h.typed == nil {
		h.typed = map[uint32]string{}
	}
	h.typed[id] += string(data)
	h.mu.Unlock()
	echo := strings.ReplaceAll(string(data), "\n", "\r\n")
	h.ev().TerminalOutput(id, []byte(echo))
	if m := regexp.MustCompile(`__gldone ([0-9a-f]+) \$\?`).FindStringSubmatch(string(data)); m != nil {
		out, code := "\x1b[1mcommand not found\x1b[0m\r\n", 127
		if strings.Contains(string(data), "uname -a") {
			out, code = "Linux desk-pi 6.6.51 aarch64 GNU/Linux\r\n", 0
		}
		h.ev().TerminalOutput(id, []byte(out+"\r\n__gldone_"+m[1]+" "+itoa(code)+"\r\npi@desk-pi:~ $ "))
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func (h *fakeHost) OpenTerminal(name string) {
	h.mu.Lock()
	id := uint32(len(h.tabs) + 1)
	h.tabs = append(h.tabs, viewer.TerminalTab{ID: id, Name: name})
	tabs := append([]viewer.TerminalTab(nil), h.tabs...)
	h.mu.Unlock()
	go func() {
		h.ev().TerminalTabs(tabs, false)
		h.ev().TerminalOutput(id, []byte("pi@desk-pi:~ $ "))
	}()
}

func (h *fakeHost) SetHidden(v bool) { h.mu.Lock(); h.hidden = v; h.mu.Unlock() }

// RequestKeyframe: a desktop sends a whole picture (a VP8 keyframe made
// by the real encoder would need libvpx here: the test feeds the decoder's
// output through encodeShot instead, below).
func (h *fakeHost) RequestKeyframe() {}

func (h *fakeHost) AskScreenshot(context.Context) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.asked++
	return h.perms.Record
}

func (h *fakeHost) HangUp() { h.mu.Lock(); h.hungUp = true; h.mu.Unlock() }

type rig struct {
	t      *testing.T
	cfg    models.Config
	hosts  map[string]*fakeHost
	cs     *mcp.ClientSession
	audit  string
	server *Server
}

func newRig(t *testing.T, cfg models.Config, saved map[string]string, hosts map[string]*fakeHost) *rig {
	t.Helper()
	r := &rig{t: t, cfg: cfg, hosts: hosts, audit: filepath.Join(t.TempDir(), AuditFile)}
	remotes := models.Remotes{Devices: []models.RemoteDevice{
		{DeviceID: "dev-pi", Name: "desk-pi", OS: "linux", Token: "t1"},
		{DeviceID: "dev-mac", Name: "Studio", Label: "Office Mac", OS: "darwin", Token: "t2"},
		{DeviceID: "dev-other", Name: "laptop", OS: "windows", Token: "t3"},
	}}
	r.server = New(Options{
		Version: "1.6.0",
		Config:  func() (models.Config, error) { return r.cfg, nil },
		Remotes: func() (models.Remotes, error) { return remotes, nil },
		Password: func(id string) (string, error) {
			if p, ok := saved[id]; ok {
				return p, nil
			}
			return "", keyring.ErrNotFound
		},
		Audit: NewAudit(r.audit),
		newSession: func(o viewer.Options) session {
			h := hosts[o.Door.Device.DeviceID]
			h.opts = o
			return h
		},
	})
	ct, st := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = r.server.Run(ctx, st) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "claude-code", Version: "2.1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	r.cs = cs
	return r
}

// call runs a tool: its text, and whether it was an error.
func (r *rig) call(name string, args map[string]any) (string, bool, *mcp.CallToolResult) {
	r.t.Helper()
	res, err := r.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		r.t.Fatalf("%s: %v", name, err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String(), res.IsError, res
}

func on(act bool) models.Config {
	return models.Config{Assistants: models.Assistants{Enabled: true, Act: act}}
}

func terminalHost() *fakeHost {
	return &fakeHost{host: viewer.HostInfo{Name: "desk-pi", OS: "linux", Terminal: true, TerminalTabs: true},
		perms: models.Permissions{View: true, Control: true, Files: true, Record: true}, password: "pi-secret"}
}

// The assistant lists the computers, connects to an allowed terminal
// computer with the saved password, runs commands in a terminal of its
// own, reads them back, and every call is in the activity log.
func TestAnAssistantRunsCommandsOnATerminalComputer(t *testing.T) {
	pi := terminalHost()
	r := newRig(t, on(true), map[string]string{"dev-pi": "pi-secret"}, map[string]*fakeHost{"dev-pi": pi})

	list, isErr, _ := r.call("list_computers", nil)
	if isErr || !strings.Contains(list, "desk-pi (Linux, id dev-pi): allowed") || !strings.Contains(list, "Office Mac (macOS, id dev-mac): not allowed") {
		t.Fatalf("list:\n%s", list)
	}

	got, isErr, _ := r.call("connect", map[string]any{"computer": "DESK-PI"})
	if isErr || !strings.Contains(got, "a terminal computer") || !strings.Contains(got, `1 "shell"`) {
		t.Fatalf("connect: %s", got)
	}
	if pi.opts.Name != "Claude Code (AI assistant)" || pi.opts.Door.Kind != viewer.DoorReach || len(pi.opts.Codecs) != 1 || pi.opts.Codecs[0] != "vp8" {
		t.Fatalf("session options: %+v", pi.opts)
	}
	if len(pi.gotPass) != 1 || pi.gotPass[0] != "pi-secret" || !pi.hidden {
		t.Fatalf("password %q, hidden %v", pi.gotPass, pi.hidden)
	}

	got, isErr, _ = r.call("terminal_run", map[string]any{"computer": "desk-pi", "command": "uname -a"})
	if isErr || got != "Exit code 0.\nLinux desk-pi 6.6.51 aarch64 GNU/Linux" {
		t.Fatalf("run: %q", got)
	}
	// In a terminal of its own, not the person's.
	if pi.typed[1] != "" || !strings.Contains(pi.typed[2], "uname -a") || pi.tabs[1].Name != "assistant" {
		t.Fatalf("typed: %v, tabs %+v", pi.typed, pi.tabs)
	}
	got, _, _ = r.call("terminal_run", map[string]any{"computer": "desk-pi", "command": "nope"})
	if got != "Exit code 127.\ncommand not found" {
		t.Fatalf("second run: %q", got)
	}
	if len(pi.tabs) != 2 {
		t.Fatal("a second terminal was opened for the second command")
	}

	got, _, _ = r.call("terminal_read", map[string]any{"computer": "desk-pi"})
	if !strings.Contains(got, `2 "assistant" (yours)`) || !strings.Contains(got, "Linux desk-pi") || strings.Contains(got, "\x1b") {
		t.Fatalf("read: %q", got)
	}
	got, _, _ = r.call("terminal_read", map[string]any{"computer": "desk-pi", "terminal": "shell"})
	if strings.Contains(got, "Linux desk-pi") {
		t.Fatalf("read the person's terminal: %q", got)
	}
	r.call("terminal_send", map[string]any{"computer": "desk-pi", "text": "\u0003", "terminal": "1"})
	if !strings.HasSuffix(pi.typed[1], "\u0003") {
		t.Fatalf("send: %v", pi.typed)
	}

	got, _, _ = r.call("disconnect", map[string]any{"computer": "dev-pi"})
	if got != "Disconnected from desk-pi." || !pi.hungUp {
		t.Fatalf("disconnect: %q", got)
	}

	log, err := os.ReadFile(r.audit)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(log)), "\n")
	var first AuditEntry
	json.Unmarshal([]byte(lines[2]), &first)
	if len(lines) != 8 || first.Tool != "terminal_run" || first.Detail != "uname -a" || first.Assistant != "Claude Code (AI assistant)" || first.Result != "ok" {
		t.Fatalf("activity log:\n%s", log)
	}
	if strings.Contains(string(log), "pi-secret") {
		t.Fatal("the password is in the activity log")
	}
}

// Off in Settings: nothing. Allowed only to look: no commands. A computer
// not allowed, or a wrong saved password: told how to fix it.
func TestWhatTheSettingsAllow(t *testing.T) {
	pi := terminalHost()
	r := newRig(t, models.Config{}, map[string]string{"dev-pi": "old-password"}, map[string]*fakeHost{"dev-pi": pi})
	if got, isErr, _ := r.call("list_computers", nil); !isErr || !strings.Contains(got, "Settings › Assistants") {
		t.Fatalf("off: %q", got)
	}

	r.cfg = on(false)
	if got, isErr, _ := r.call("terminal_run", map[string]any{"computer": "desk-pi", "command": "id"}); !isErr || !strings.Contains(got, "only look") {
		t.Fatalf("look only: %q", got)
	}
	if got, isErr, _ := r.call("connect", map[string]any{"computer": "laptop"}); !isErr || !strings.Contains(got, "not allowed for assistants") {
		t.Fatalf("not allowed: %q", got)
	}
	if got, isErr, _ := r.call("connect", map[string]any{"computer": "desk-pi"}); !isErr || !strings.Contains(got, "refused the saved password") {
		t.Fatalf("wrong password: %q", got)
	}
	if len(pi.gotPass) != 1 {
		t.Fatalf("the password was tried %d times", len(pi.gotPass))
	}
	if got, isErr, _ := r.call("connect", map[string]any{"computer": "nothing"}); !isErr || !strings.Contains(got, "no linked computer") {
		t.Fatalf("unknown: %q", got)
	}
}

// A desktop: screenshot asks the computer first, and needs the record
// permission.
func TestScreenshotNeedsTheRecordPermission(t *testing.T) {
	mac := &fakeHost{host: viewer.HostInfo{Name: "Studio", OS: "darwin"}, perms: models.Permissions{View: true}, password: "m"}
	r := newRig(t, on(false), map[string]string{"dev-mac": "m"}, map[string]*fakeHost{"dev-mac": mac})
	got, isErr, _ := r.call("connect", map[string]any{"computer": "office"})
	if isErr || !strings.Contains(got, "a desktop") || !strings.Contains(got, "may not take screenshots") {
		t.Fatalf("connect: %q", got)
	}
	if got, isErr, _ := r.call("screenshot", map[string]any{"computer": "Office Mac"}); !isErr || !strings.Contains(got, "Take screenshots") {
		t.Fatalf("screenshot without the permission: %q", got)
	}
	if got, isErr, _ := r.call("terminal_read", map[string]any{"computer": "Office Mac"}); !isErr || !strings.Contains(got, "is a desktop") {
		t.Fatalf("terminal on a desktop: %q", got)
	}
}

// A picture is a JPEG no wider than shotWidth, its proportions kept.
func TestEncodeShot(t *testing.T) {
	w, h := 2560, 1600
	y := bytes.Repeat([]byte{200}, w*h)
	uv := bytes.Repeat([]byte{128}, (w/2)*(h/2))
	jpg, gw, gh, err := encodeShot(&decoderI420{Width: w, Height: h, Y: y, U: uv, V: uv, StrideY: w, StrideUV: w / 2})
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(jpg))
	if err != nil {
		t.Fatal(err)
	}
	if gw != shotWidth || gh != 980 || img.Bounds() != image.Rect(0, 0, gw, gh) {
		t.Fatalf("%dx%d %v", gw, gh, img.Bounds())
	}
}

// Terminal output as text: colors, rewritten lines and backspaces as a
// terminal shows them.
func TestTidy(t *testing.T) {
	in := "$ { uname\n> }; printf '\\n%s_%s %d\\n' __gldone 66e33927976e $?\nDarwin\n\n__gldone_66e33927976e 0\n$ "
	if got := tidy(in); got != "$ { uname\nDarwin\n$ " {
		t.Fatalf("%q", got)
	}
}

func TestClean(t *testing.T) {
	in := "\x1b[32mok\x1b[0m\r\n50%\r100%\r\nab\bc\r\n\x1b]0;title\x07done"
	if got := string(clean([]byte(in))); got != "ok\n100%\nac\ndone" {
		t.Fatalf("%q", got)
	}
}
