// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package mcpserver lets AI assistants (MCP clients such as Claude) use
// the computers linked in go-link Desk's My computers: the app itself runs
// as their MCP server ("go-link-desk mcp", over stdio). Each computer must
// be allowed in Settings › Assistants, which keeps its unattended password
// in the system's keychain: the assistant never sees it. The computer sees
// the assistant come in as a named viewer, with the link's permissions.
// Assistants only look unless Settings lets them type and run commands,
// and every call goes to an activity log.
package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/viewer"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/keyring"
)

// PasswordService is the keychain service the computers' passwords are
// kept under (the account is the computer's device id).
const PasswordService = "go-link-desk-assistants"

// Options configure a Server.
type Options struct {
	Version string
	// SignalURL is the signaling server for computers linked without one.
	SignalURL   string
	OwnDeviceID string
	// Config and Remotes are read again on every call, so a change in the
	// app's Settings counts at once.
	Config  func() (models.Config, error)
	Remotes func() (models.Remotes, error)
	// Password is a computer's saved password (keyring.ErrNotFound: the
	// computer is not allowed). Default: the system's keychain.
	Password func(deviceID string) (string, error)
	Audit    *Audit
	Logger   *slog.Logger
	// Downloads is where file_download saves (default: ~/Downloads).
	Downloads string

	newSession func(viewer.Options) session
}

// Server answers an assistant.
type Server struct {
	opts Options
	log  *slog.Logger

	mu        sync.Mutex
	computers map[string]*computer // by device id
}

// New makes a server.
func New(opts Options) *Server {
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	if opts.Password == nil {
		opts.Password = func(id string) (string, error) { return keyring.Get(PasswordService, id) }
	}
	if opts.newSession == nil {
		opts.newSession = func(o viewer.Options) session { return viewer.New(o) }
	}
	return &Server{opts: opts, log: opts.Logger, computers: map[string]*computer{}}
}

const instructions = `go-link Desk connects you to the user's own computers linked in its My computers (desktops, Raspberry Pis and servers with go-link-desk-terminal), over an encrypted peer-to-peer connection.

Start with list_computers. Tools that take a computer connect to it by themselves; the person at that computer sees you come in by name. On a terminal computer, terminal_run runs a command in a terminal of your own (named "assistant", beside the person's) and returns its output and exit code; terminal_read shows what a terminal printed. On a desktop, screenshot shows the screen (the computer must allow screenshots for this link); click and scroll at pixels of your last screenshot, type_text, key for Enter and shortcuts, clipboard_get and clipboard_set; take a new screenshot after acting to see the result. On any computer that allows files, file_list, file_download (into this computer's Downloads) and file_upload. Disconnect when you are done.

The user decides in go-link Desk › Settings › Assistants which computers you may use, and whether you may type and run commands or only look.`

// MCP is the server's MCP side.
func (s *Server) MCP() *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "go-link-desk", Title: "go-link Desk", Version: s.opts.Version}, &mcp.ServerOptions{Instructions: instructions})
	no := false
	yes := true
	mcp.AddTool(srv, &mcp.Tool{Name: "list_computers", Description: "The computers linked in go-link Desk's My computers: name, system, whether you may use them, and whether you are connected.",
		Annotations: &mcp.ToolAnnotations{Title: "List computers", ReadOnlyHint: true, OpenWorldHint: &no}}, s.listComputers)
	mcp.AddTool(srv, &mcp.Tool{Name: "connect", Description: "Connect to a computer (the other tools do it by themselves). Tells what it is (a desktop or a terminal), its terminals and what this link may do.",
		Annotations: &mcp.ToolAnnotations{Title: "Connect", ReadOnlyHint: true, OpenWorldHint: &yes}}, s.connect)
	mcp.AddTool(srv, &mcp.Tool{Name: "disconnect", Description: "Leave a computer.",
		Annotations: &mcp.ToolAnnotations{Title: "Disconnect", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &yes}}, s.disconnect)
	mcp.AddTool(srv, &mcp.Tool{Name: "terminal_run", Description: "Run a shell command on a terminal computer (go-link-desk-terminal), in a terminal of your own named \"assistant\", and wait for it to end: returns its output and exit code. Do not start interactive programs (editors, pagers, top): use non-interactive flags. A command still running when the time is up keeps running; see it later with terminal_read, stop it with terminal_send and \"\\u0003\".",
		Annotations: &mcp.ToolAnnotations{Title: "Run a command", DestructiveHint: &yes, OpenWorldHint: &yes}}, s.terminalRun)
	mcp.AddTool(srv, &mcp.Tool{Name: "terminal_send", Description: "Type text into a terminal as it is, without waiting: an answer to a prompt, or keys such as \"\\u0003\" (Ctrl+C), \"\\u0004\" (Ctrl+D) or \"\\r\" (Enter).",
		Annotations: &mcp.ToolAnnotations{Title: "Type into a terminal", DestructiveHint: &yes, OpenWorldHint: &yes}}, s.terminalSend)
	mcp.AddTool(srv, &mcp.Tool{Name: "terminal_read", Description: "What a terminal printed last, as plain text, and the list of terminals.",
		Annotations: &mcp.ToolAnnotations{Title: "Read a terminal", ReadOnlyHint: true, OpenWorldHint: &yes}}, s.terminalRead)
	mcp.AddTool(srv, &mcp.Tool{Name: "screenshot", Description: "A picture of a desktop computer's screen (JPEG). The person at the computer is shown that a screenshot was taken.",
		Annotations: &mcp.ToolAnnotations{Title: "Screenshot", ReadOnlyHint: true, OpenWorldHint: &yes}}, s.screenshot)
	s.addDesktopTools(srv)
	return srv
}

// Run answers the assistant on this process' standard input and output
// until it leaves, then leaves every computer.
func (s *Server) Run(ctx context.Context, t mcp.Transport) error {
	defer s.Close()
	return s.MCP().Run(ctx, t)
}

// Close leaves every computer.
func (s *Server) Close() {
	s.mu.Lock()
	cs := s.computers
	s.computers = map[string]*computer{}
	s.mu.Unlock()
	for _, c := range cs {
		c.close()
	}
}

type computerIn struct {
	Computer string `json:"computer" jsonschema:"the computer: its name as list_computers shows it, or its id"`
}

type runIn struct {
	Computer       string `json:"computer" jsonschema:"the computer: its name as list_computers shows it, or its id"`
	Command        string `json:"command" jsonschema:"the shell command (sh syntax; several lines are fine)"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" jsonschema:"how long to wait for it to end (default 30, at most 600)"`
}

type sendIn struct {
	Computer string `json:"computer" jsonschema:"the computer: its name as list_computers shows it, or its id"`
	Text     string `json:"text" jsonschema:"what to type, as is (\\r is Enter)"`
	Terminal string `json:"terminal,omitempty" jsonschema:"which terminal: its number or name (default: yours, else the first)"`
}

type readIn struct {
	Computer string `json:"computer" jsonschema:"the computer: its name as list_computers shows it, or its id"`
	Terminal string `json:"terminal,omitempty" jsonschema:"which terminal: its number or name (default: yours, else the first)"`
	MaxChars int    `json:"max_chars,omitempty" jsonschema:"at most this many characters, the last ones (default 8000)"`
}

// errOff: the user did not turn assistants on.
var errOff = errors.New("go-link Desk does not let assistants in: the user can turn it on in go-link Desk › Settings › Assistants")

// errLook: assistants may only look.
var errLook = errors.New("go-link Desk lets assistants only look: the user can allow typing and commands in go-link Desk › Settings › Assistants")

// settings is the app's Assistants setting now.
func (s *Server) settings() (models.Assistants, error) {
	cfg, err := s.opts.Config()
	if err != nil {
		return models.Assistants{}, err
	}
	if !cfg.Assistants.Enabled {
		return cfg.Assistants, errOff
	}
	return cfg.Assistants, nil
}

func text(format string, a ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, a...)}}}
}

// assistant is the name the computer is shown ("Claude Code (AI assistant)").
func assistant(req *mcp.CallToolRequest) string {
	name := ""
	if req != nil && req.Session != nil {
		if p := req.Session.InitializeParams(); p != nil && p.ClientInfo != nil {
			name = p.ClientInfo.Title
			if name == "" {
				name = p.ClientInfo.Name
			}
		}
	}
	switch strings.ToLower(name) {
	case "claude-ai", "claude", "claude desktop":
		name = "Claude"
	case "claude-code", "claude code":
		name = "Claude Code"
	case "cursor", "cursor-vscode":
		name = "Cursor"
	case "":
		name = "An assistant"
	}
	return name + " (AI assistant)"
}

func (s *Server) audit(req *mcp.CallToolRequest, tool, computer, detail string, err error) {
	result := "ok"
	if err != nil {
		result = err.Error()
	}
	s.opts.Audit.Log(AuditEntry{Assistant: assistant(req), Tool: tool, Computer: computer, Detail: detail, Result: result})
}

func (s *Server) listComputers(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	if _, err := s.settings(); err != nil {
		s.audit(req, "list_computers", "", "", err)
		return nil, nil, err
	}
	r, err := s.opts.Remotes()
	if err != nil {
		return nil, nil, err
	}
	s.audit(req, "list_computers", "", "", nil)
	if len(r.Devices) == 0 {
		return text("No computer is linked in go-link Desk's My computers yet."), nil, nil
	}
	devices := append([]models.RemoteDevice(nil), r.Devices...)
	sort.SliceStable(devices, func(i, j int) bool { return title(devices[i]) < title(devices[j]) })
	var b strings.Builder
	for _, d := range devices {
		fmt.Fprintf(&b, "- %s (%s, id %s): ", title(d), osName(d.OS), d.DeviceID)
		if _, err := s.opts.Password(d.DeviceID); err != nil {
			b.WriteString("not allowed for assistants (the user can allow it in go-link Desk › Settings › Assistants)")
		} else if c := s.connected(d.DeviceID); c != nil {
			h := c.s.Host()
			kind := "a desktop"
			if h.Terminal {
				kind = "a terminal"
			}
			fmt.Fprintf(&b, "connected, %s", kind)
		} else {
			b.WriteString("allowed")
		}
		b.WriteByte('\n')
	}
	return text("%s", strings.TrimRight(b.String(), "\n")), nil, nil
}

func title(d models.RemoteDevice) string {
	if t := d.Title(); t != "" {
		return t
	}
	return "Linked computer"
}

func osName(os string) string {
	switch os {
	case "darwin", "macos":
		return "macOS"
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	case "":
		return "unknown system"
	}
	return os
}

// find is the linked computer asked for: its id, or its name.
func (s *Server) find(which string) (models.RemoteDevice, error) {
	r, err := s.opts.Remotes()
	if err != nil {
		return models.RemoteDevice{}, err
	}
	which = strings.TrimSpace(which)
	var matches []models.RemoteDevice
	for _, d := range r.Devices {
		if d.DeviceID == which {
			return d, nil
		}
		if strings.EqualFold(title(d), which) || strings.EqualFold(d.Name, which) {
			matches = append(matches, d)
		}
	}
	if len(matches) == 0 {
		for _, d := range r.Devices {
			if which != "" && strings.Contains(strings.ToLower(title(d)), strings.ToLower(which)) {
				matches = append(matches, d)
			}
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return models.RemoteDevice{}, fmt.Errorf("no linked computer %q: see list_computers", which)
	}
	return models.RemoteDevice{}, fmt.Errorf("%q could be more than one computer: use its id (see list_computers)", which)
}

// connected is the live connection to a computer, if any.
func (s *Server) connected(id string) *computer {
	s.mu.Lock()
	c := s.computers[id]
	s.mu.Unlock()
	if c != nil && c.live() {
		return c
	}
	return nil
}

// connectTimeout bounds a connection.
const connectTimeout = 30 * time.Second

// computer is the connection to the computer asked for, made if needed.
func (s *Server) computer(ctx context.Context, req *mcp.CallToolRequest, which string) (*computer, error) {
	if _, err := s.settings(); err != nil {
		return nil, err
	}
	d, err := s.find(which)
	if err != nil {
		return nil, err
	}
	// Re-check the per-computer authorization on every call, even for a
	// cached live connection: if the user clicked "Stop allowing" (the
	// keyring entry is gone), drop any live session for this computer at
	// once instead of letting it keep working until the next connect.
	pass, err := s.opts.Password(d.DeviceID)
	if errors.Is(err, keyring.ErrNotFound) {
		s.drop(d.DeviceID)
		return nil, fmt.Errorf("%s is not allowed for assistants: the user can allow it in go-link Desk › Settings › Assistants", title(d))
	} else if err != nil {
		return nil, fmt.Errorf("cannot read %s's saved password: %w", title(d), err)
	}
	if c := s.connected(d.DeviceID); c != nil {
		return c, nil
	}
	c := newComputer(d)
	c.pass = pass
	signal := d.SignalURL
	if signal == "" {
		signal = s.opts.SignalURL
	}
	dev := d
	c.s = s.opts.newSession(viewer.Options{
		Door: viewer.Door{Kind: viewer.DoorReach, Device: &dev}, Name: assistant(req),
		Agent:  fmt.Sprintf("go-link Desk %s for AI assistants on %s", s.opts.Version, osName(runtime.GOOS)),
		Codecs: []string{"vp8"}, SignalURL: signal, OwnDeviceID: s.opts.OwnDeviceID, Logger: s.log, Events: c.events(),
	})
	cctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	if err := c.connect(cctx); err != nil {
		return nil, fmt.Errorf("%s: %w", title(d), err)
	}
	s.mu.Lock()
	if old := s.computers[d.DeviceID]; old != nil {
		go old.close()
	}
	s.computers[d.DeviceID] = c
	s.mu.Unlock()
	s.log.Info("assistant connected", "device_id", d.DeviceID, "assistant", assistant(req))
	return c, nil
}

func (s *Server) connect(ctx context.Context, req *mcp.CallToolRequest, in computerIn) (*mcp.CallToolResult, any, error) {
	c, err := s.computer(ctx, req, in.Computer)
	s.audit(req, "connect", in.Computer, "", err)
	if err != nil {
		return nil, nil, err
	}
	return text("%s", s.describe(c)), nil, nil
}

// describe is what a connected computer is and what may be done there.
func (s *Server) describe(c *computer) string {
	h := c.s.Host()
	p := c.s.Permissions()
	name, sys := h.Name, h.OS
	if name == "" {
		name = title(c.device)
	}
	if sys == "" {
		sys = c.device.OS
	}
	var b strings.Builder
	if h.Terminal {
		fmt.Fprintf(&b, "Connected to %s (%s), a terminal computer (go-link-desk-terminal).", name, osName(sys))
		if tabs := c.tabsText(); tabs != "" {
			fmt.Fprintf(&b, " Its terminals: %s.", tabs)
		}
		b.WriteString(" Use terminal_run, terminal_read and terminal_send.")
	} else {
		fmt.Fprintf(&b, "Connected to %s (%s), a desktop.", name, osName(sys))
		if p.Record {
			b.WriteString(" Use screenshot to see its screen")
			if p.Control {
				b.WriteString(", then click, scroll, type_text and key to use it")
			}
			b.WriteString(".")
		} else {
			b.WriteString(" This link may not take screenshots (the computer's Settings › Unattended access can allow it).")
		}
	}
	if p.Files {
		b.WriteString(" Its files: file_list, file_download, file_upload.")
	}
	if set, err := s.settings(); err == nil && !set.Act {
		b.WriteString(" You may only look: typing, clicking and commands are off in go-link Desk › Settings › Assistants.")
	}
	return b.String()
}

// drop closes and forgets the live connection to a computer, if any.
func (s *Server) drop(id string) {
	s.mu.Lock()
	c := s.computers[id]
	delete(s.computers, id)
	s.mu.Unlock()
	if c != nil {
		c.close()
	}
}

func (s *Server) disconnect(ctx context.Context, req *mcp.CallToolRequest, in computerIn) (*mcp.CallToolResult, any, error) {
	d, err := s.find(in.Computer)
	s.audit(req, "disconnect", in.Computer, "", err)
	if err != nil {
		return nil, nil, err
	}
	s.mu.Lock()
	c := s.computers[d.DeviceID]
	delete(s.computers, d.DeviceID)
	s.mu.Unlock()
	if c == nil {
		return text("Not connected to %s.", title(d)), nil, nil
	}
	c.close()
	return text("Disconnected from %s.", title(d)), nil, nil
}

// terminal is a connected terminal computer that lets the assistant type.
func (s *Server) terminal(ctx context.Context, req *mcp.CallToolRequest, which string, act bool) (*computer, error) {
	if act {
		set, err := s.settings()
		if err != nil {
			return nil, err
		}
		if !set.Act {
			return nil, errLook
		}
	}
	c, err := s.computer(ctx, req, which)
	if err != nil {
		return nil, err
	}
	if !c.s.Host().Terminal {
		return nil, fmt.Errorf("%s is a desktop, not a terminal computer: use screenshot", title(c.device))
	}
	if act && !c.s.Permissions().Control {
		return nil, fmt.Errorf("%s lets this link only watch its terminals", title(c.device))
	}
	return c, nil
}

const (
	defaultTimeout = 30 * time.Second
	maxTimeout     = 600 * time.Second
	defaultChars   = 8000
	maxChars       = 100000
)

func (s *Server) terminalRun(ctx context.Context, req *mcp.CallToolRequest, in runIn) (*mcp.CallToolResult, any, error) {
	c, err := s.terminal(ctx, req, in.Computer, true)
	if err == nil && strings.TrimSpace(in.Command) == "" {
		err = errors.New("no command")
	}
	var id uint32
	if err == nil {
		id, err = c.assistantTerminal(ctx)
	}
	s.audit(req, "terminal_run", in.Computer, in.Command, err)
	if err != nil {
		return nil, nil, err
	}
	timeout := defaultTimeout
	if in.TimeoutSeconds > 0 {
		timeout = min(time.Duration(in.TimeoutSeconds)*time.Second, maxTimeout)
	}
	r := c.runCommand(ctx, id, in.Command, timeout)
	out := r.Output
	if n := len(out); n > maxChars {
		out = "…" + out[n-maxChars:]
	}
	if !r.Done {
		return text("Still running after %s (it goes on in your terminal; terminal_read shows more, terminal_send with \"\\u0003\" stops it). Output so far:\n%s", timeout, out), nil, nil
	}
	return text("Exit code %d.\n%s", r.ExitCode, out), nil, nil
}

func (s *Server) terminalSend(ctx context.Context, req *mcp.CallToolRequest, in sendIn) (*mcp.CallToolResult, any, error) {
	c, err := s.terminal(ctx, req, in.Computer, true)
	var id uint32
	if err == nil {
		id, err = c.tab(in.Terminal)
	}
	s.audit(req, "terminal_send", in.Computer, in.Text, err)
	if err != nil {
		return nil, nil, err
	}
	c.s.SendTerminal(id, []byte(in.Text))
	quiet(ctx, c, id, 500*time.Millisecond)
	return text("Sent."), nil, nil
}

func (s *Server) terminalRead(ctx context.Context, req *mcp.CallToolRequest, in readIn) (*mcp.CallToolResult, any, error) {
	c, err := s.terminal(ctx, req, in.Computer, false)
	var id uint32
	if err == nil {
		id, err = c.tab(in.Terminal)
	}
	s.audit(req, "terminal_read", in.Computer, in.Terminal, err)
	if err != nil {
		return nil, nil, err
	}
	n := in.MaxChars
	if n <= 0 {
		n = defaultChars
	}
	n = min(n, maxChars)
	out := tidy(string(clean(c.since(id, 0))))
	if len(out) > n {
		out = "…" + out[len(out)-n:]
	}
	head := ""
	if tabs := c.tabsText(); tabs != "" {
		head = "Terminals: " + tabs + "\n"
	}
	return text("%s%s", head, out), nil, nil
}

func (s *Server) screenshot(ctx context.Context, req *mcp.CallToolRequest, in computerIn) (*mcp.CallToolResult, any, error) {
	c, err := s.computer(ctx, req, in.Computer)
	var (
		jpg  []byte
		w, h int
	)
	if err == nil {
		jpg, w, h, err = c.screenshot(ctx)
	}
	s.audit(req, "screenshot", in.Computer, "", err)
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{
		&mcp.ImageContent{MIMEType: "image/jpeg", Data: jpg},
		&mcp.TextContent{Text: fmt.Sprintf("%s's screen, %d×%d.", title(c.device), w, h)},
	}}, nil, nil
}
