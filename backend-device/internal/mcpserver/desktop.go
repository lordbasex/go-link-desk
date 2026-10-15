// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lordbasex/go-link-desk/backend-device/internal/viewer"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/keymap"
)

// A desktop's mouse, keyboard and clipboard, and any computer's files: the
// assistant clicks where it saw something in its last screenshot, types,
// presses shortcuts, and moves files to and from the computer.

func (s *Server) addDesktopTools(srv *mcp.Server) {
	no, yes := false, true
	mcp.AddTool(srv, &mcp.Tool{Name: "click", Description: "Click on a desktop computer's screen at x, y: pixels of your last screenshot of it (take one first). Button left (default), right or middle; double for a double click.",
		Annotations: &mcp.ToolAnnotations{Title: "Click", DestructiveHint: &yes, OpenWorldHint: &yes}}, s.click)
	mcp.AddTool(srv, &mcp.Tool{Name: "scroll", Description: "Scroll a desktop computer's screen at x, y (pixels of your last screenshot): lines down (positive) or up (negative), and right or left with dx.",
		Annotations: &mcp.ToolAnnotations{Title: "Scroll", DestructiveHint: &no, OpenWorldHint: &yes}}, s.scroll)
	mcp.AddTool(srv, &mcp.Tool{Name: "type_text", Description: "Type text on a desktop computer, where its focus is, character by character (any language, whatever its keyboard layout). For Enter, shortcuts and special keys use key.",
		Annotations: &mcp.ToolAnnotations{Title: "Type text", DestructiveHint: &yes, OpenWorldHint: &yes}}, s.typeText)
	mcp.AddTool(srv, &mcp.Tool{Name: "key", Description: "Press a key or a shortcut on a desktop computer, as \"enter\", \"esc\", \"tab\", \"ctrl+c\", \"cmd+space\", \"alt+f4\", \"shift+arrowdown\", \"f5\". Modifiers: ctrl, shift, alt (option), cmd (win, super). Keys: a letter, a digit, enter, esc, tab, space, backspace, delete, up, down, left, right, home, end, pageup, pagedown, f1 to f24, or a browser key code (\"Minus\", \"BracketLeft\").",
		Annotations: &mcp.ToolAnnotations{Title: "Press keys", DestructiveHint: &yes, OpenWorldHint: &yes}}, s.key)
	mcp.AddTool(srv, &mcp.Tool{Name: "clipboard_get", Description: "The text the computer copied last (since you connected).",
		Annotations: &mcp.ToolAnnotations{Title: "Read the clipboard", ReadOnlyHint: true, OpenWorldHint: &yes}}, s.clipboardGet)
	mcp.AddTool(srv, &mcp.Tool{Name: "clipboard_set", Description: "Put text on the computer's clipboard (paste it there with key cmd+v or ctrl+v).",
		Annotations: &mcp.ToolAnnotations{Title: "Set the clipboard", DestructiveHint: &no, OpenWorldHint: &yes}}, s.clipboardSet)
	mcp.AddTool(srv, &mcp.Tool{Name: "file_list", Description: "List a folder of the computer, inside its user's home (path relative to it; \"\" is home).",
		Annotations: &mcp.ToolAnnotations{Title: "List files", ReadOnlyHint: true, OpenWorldHint: &yes}}, s.fileList)
	mcp.AddTool(srv, &mcp.Tool{Name: "file_download", Description: "Copy a file of the computer (path relative to its user's home) into this computer's Downloads folder; returns where it was saved, and its text when it is a small text file.",
		Annotations: &mcp.ToolAnnotations{Title: "Download a file", ReadOnlyHint: true, OpenWorldHint: &yes}}, s.fileDownload)
	mcp.AddTool(srv, &mcp.Tool{Name: "file_upload", Description: "Send a file of this computer (an absolute path) to the computer: it lands in its Downloads folder, never over a file there.",
		Annotations: &mcp.ToolAnnotations{Title: "Upload a file", DestructiveHint: &no, OpenWorldHint: &yes}}, s.fileUpload)
}

type pointIn struct {
	Computer string `json:"computer" jsonschema:"the computer: its name as list_computers shows it, or its id"`
	X        int    `json:"x" jsonschema:"pixels from the left of your last screenshot"`
	Y        int    `json:"y" jsonschema:"pixels from the top of your last screenshot"`
	Button   string `json:"button,omitempty" jsonschema:"left (default), right or middle"`
	Double   bool   `json:"double,omitempty" jsonschema:"a double click"`
}

type scrollIn struct {
	Computer string `json:"computer" jsonschema:"the computer: its name as list_computers shows it, or its id"`
	X        int    `json:"x" jsonschema:"pixels from the left of your last screenshot"`
	Y        int    `json:"y" jsonschema:"pixels from the top of your last screenshot"`
	Lines    int    `json:"lines,omitempty" jsonschema:"lines to scroll: positive down, negative up"`
	DX       int    `json:"dx,omitempty" jsonschema:"lines to scroll sideways: positive right, negative left"`
}

type textIn struct {
	Computer string `json:"computer" jsonschema:"the computer: its name as list_computers shows it, or its id"`
	Text     string `json:"text" jsonschema:"the text"`
}

type keyIn struct {
	Computer string `json:"computer" jsonschema:"the computer: its name as list_computers shows it, or its id"`
	Keys     string `json:"keys" jsonschema:"a key or a shortcut, as enter, ctrl+c, cmd+shift+t"`
}

type pathIn struct {
	Computer string `json:"computer" jsonschema:"the computer: its name as list_computers shows it, or its id"`
	Path     string `json:"path" jsonschema:"a path inside the computer's user's home, relative to it"`
}

type uploadIn struct {
	Computer string `json:"computer" jsonschema:"the computer: its name as list_computers shows it, or its id"`
	Path     string `json:"path" jsonschema:"an absolute path of a file on this computer"`
}

// desktop is a connected desktop computer whose mouse and keyboard the
// assistant may use.
func (s *Server) desktop(ctx context.Context, req *mcp.CallToolRequest, which string) (*computer, error) {
	set, err := s.settings()
	if err != nil {
		return nil, err
	}
	if !set.Act {
		return nil, errLook
	}
	c, err := s.computer(ctx, req, which)
	if err != nil {
		return nil, err
	}
	if c.s.Host().Terminal {
		return nil, fmt.Errorf("%s is a terminal computer: use terminal_run and terminal_send", title(c.device))
	}
	if !c.s.Permissions().Control {
		return nil, fmt.Errorf("%s lets this link only watch: allow \"Use your mouse and keyboard\" for it in its go-link Desk (Settings › Unattended access)", title(c.device))
	}
	return c, nil
}

// inputOf is the computer's input, allowed as the link is now.
func (c *computer) inputOf() *viewer.Input {
	c.mu.Lock()
	if c.input == nil {
		c.input = viewer.NewInput(c.s)
	}
	in := c.input
	c.mu.Unlock()
	p := c.s.Permissions()
	in.Allow(p.Control, p.Clipboard)
	return in
}

// at is a point of the last screenshot, as 0..1 of the screen.
func (c *computer) at(x, y int) (float64, float64, error) {
	c.mu.Lock()
	w, h := c.shotW, c.shotH
	c.mu.Unlock()
	if w == 0 || h == 0 {
		return 0, 0, errors.New("take a screenshot first: x and y are pixels of it")
	}
	if x < 0 || y < 0 || x >= w || y >= h {
		return 0, 0, fmt.Errorf("%d, %d is outside the screenshot (%d×%d)", x, y, w, h)
	}
	return (float64(x) + 0.5) / float64(w), (float64(y) + 0.5) / float64(h), nil
}

var buttons = map[string]int{"": 0, "left": 0, "middle": 1, "right": 2}

// filesSettle is how long after the link is in the files channel is read.
const filesSettle = time.Second

// pause lets the computer act between the steps of a click.
const pause = 40 * time.Millisecond

func (s *Server) click(ctx context.Context, req *mcp.CallToolRequest, in pointIn) (*mcp.CallToolResult, any, error) {
	c, err := s.desktop(ctx, req, in.Computer)
	var fx, fy float64
	if err == nil {
		fx, fy, err = c.at(in.X, in.Y)
	}
	b, ok := buttons[strings.ToLower(in.Button)]
	if err == nil && !ok {
		err = fmt.Errorf("no button %q: left, right or middle", in.Button)
	}
	s.audit(req, "click", in.Computer, fmt.Sprintf("%d,%d %s double=%v", in.X, in.Y, in.Button, in.Double), err)
	if err != nil {
		return nil, nil, err
	}
	input := c.inputOf()
	input.Move(fx, fy, true)
	time.Sleep(pause)
	times := 1
	if in.Double {
		times = 2
	}
	for range times {
		input.Button(b, true, fx, fy, true)
		time.Sleep(pause)
		input.Button(b, false, fx, fy, true)
		time.Sleep(pause)
	}
	return text("Clicked at %d, %d. Take a screenshot to see what changed.", in.X, in.Y), nil, nil
}

func (s *Server) scroll(ctx context.Context, req *mcp.CallToolRequest, in scrollIn) (*mcp.CallToolResult, any, error) {
	c, err := s.desktop(ctx, req, in.Computer)
	var fx, fy float64
	if err == nil {
		fx, fy, err = c.at(in.X, in.Y)
	}
	if err == nil && in.Lines == 0 && in.DX == 0 {
		err = errors.New("nothing to scroll: give lines or dx")
	}
	s.audit(req, "scroll", in.Computer, fmt.Sprintf("%d,%d lines=%d dx=%d", in.X, in.Y, in.Lines, in.DX), err)
	if err != nil {
		return nil, nil, err
	}
	input := c.inputOf()
	input.Move(fx, fy, true)
	time.Sleep(pause)
	input.Wheel(float64(in.DX), float64(in.Lines), true)
	return text("Scrolled."), nil, nil
}

func (s *Server) typeText(ctx context.Context, req *mcp.CallToolRequest, in textIn) (*mcp.CallToolResult, any, error) {
	c, err := s.desktop(ctx, req, in.Computer)
	if err == nil && in.Text == "" {
		err = errors.New("no text")
	}
	s.audit(req, "type_text", in.Computer, in.Text, err)
	if err != nil {
		return nil, nil, err
	}
	c.inputOf().TypeText(in.Text)
	return text("Typed %d characters.", len([]rune(in.Text))), nil, nil
}

func (s *Server) key(ctx context.Context, req *mcp.CallToolRequest, in keyIn) (*mcp.CallToolResult, any, error) {
	c, err := s.desktop(ctx, req, in.Computer)
	var codes []string
	if err == nil {
		codes, err = parseKeys(in.Keys)
	}
	s.audit(req, "key", in.Computer, in.Keys, err)
	if err != nil {
		return nil, nil, err
	}
	c.inputOf().Combo(codes...)
	return text("Pressed %s.", in.Keys), nil, nil
}

var modifiers = map[string]string{
	"ctrl": "ControlLeft", "control": "ControlLeft",
	"shift": "ShiftLeft",
	"alt":   "AltLeft", "option": "AltLeft", "opt": "AltLeft",
	"cmd": "MetaLeft", "command": "MetaLeft", "meta": "MetaLeft", "win": "MetaLeft", "windows": "MetaLeft", "super": "MetaLeft",
}

var keyNames = map[string]string{
	"enter": "Enter", "return": "Enter", "esc": "Escape", "escape": "Escape", "tab": "Tab", "space": "Space",
	"backspace": "Backspace", "delete": "Delete", "del": "Delete", "insert": "Insert",
	"up": "ArrowUp", "down": "ArrowDown", "left": "ArrowLeft", "right": "ArrowRight",
	"arrowup": "ArrowUp", "arrowdown": "ArrowDown", "arrowleft": "ArrowLeft", "arrowright": "ArrowRight",
	"home": "Home", "end": "End", "pageup": "PageUp", "pagedown": "PageDown",
	"minus": "Minus", "equal": "Equal", "comma": "Comma", "period": "Period", "slash": "Slash",
}

// parseKeys turns "ctrl+shift+t" into the keys' codes, modifiers first.
func parseKeys(keys string) ([]string, error) {
	parts := strings.Split(strings.ReplaceAll(strings.TrimSpace(keys), " ", ""), "+")
	var mods, rest []string
	for _, p := range parts {
		if p == "" {
			return nil, fmt.Errorf("%q is not a key or a shortcut", keys)
		}
		low := strings.ToLower(p)
		code := ""
		switch {
		case modifiers[low] != "":
			mods = append(mods, modifiers[low])
			continue
		case keyNames[low] != "":
			code = keyNames[low]
		case len(p) == 1 && unicode.IsLetter(rune(p[0])) && p[0] < 128:
			code = "Key" + strings.ToUpper(p)
		case len(p) == 1 && p[0] >= '0' && p[0] <= '9':
			code = "Digit" + p
		case len(low) >= 2 && low[0] == 'f' && strings.Trim(low[1:], "0123456789") == "":
			code = "F" + low[1:]
		default:
			code = p // a browser key code, as is
		}
		if keymap.Index(code) == 0 {
			return nil, fmt.Errorf("no key %q", p)
		}
		rest = append(rest, code)
	}
	if len(rest) == 0 && len(mods) == 0 {
		return nil, fmt.Errorf("%q is not a key or a shortcut", keys)
	}
	return append(mods, rest...), nil
}

func (s *Server) clipboardGet(ctx context.Context, req *mcp.CallToolRequest, in computerIn) (*mcp.CallToolResult, any, error) {
	c, err := s.computer(ctx, req, in.Computer)
	if err == nil && !c.s.Permissions().Clipboard {
		err = fmt.Errorf("%s does not share its clipboard with this link", title(c.device))
	}
	s.audit(req, "clipboard_get", in.Computer, "", err)
	if err != nil {
		return nil, nil, err
	}
	c.mu.Lock()
	clip := c.clip
	c.mu.Unlock()
	if clip == "" {
		return text("Nothing copied there since you connected."), nil, nil
	}
	return text("%s", clip), nil, nil
}

func (s *Server) clipboardSet(ctx context.Context, req *mcp.CallToolRequest, in textIn) (*mcp.CallToolResult, any, error) {
	set, err := s.settings()
	if err == nil && !set.Act {
		err = errLook
	}
	var c *computer
	if err == nil {
		c, err = s.computer(ctx, req, in.Computer)
	}
	if err == nil && !c.s.Permissions().Clipboard {
		err = fmt.Errorf("%s does not share its clipboard with this link", title(c.device))
	}
	if err == nil && !c.s.SendClipboard(in.Text, false) {
		err = errors.New("the text is too long, or not text")
	}
	s.audit(req, "clipboard_set", in.Computer, in.Text, err)
	if err != nil {
		return nil, nil, err
	}
	return text("On its clipboard."), nil, nil
}

// filesOf is the computer's files channel, when the link may use it.
func (c *computer) filesOf(ctx context.Context) (*viewer.Files, error) {
	if !c.s.Permissions().Files {
		return nil, fmt.Errorf("%s does not let this link send and receive files", title(c.device))
	}
	wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var f *viewer.Files
	c.wait(wctx, func() bool { f = c.files; return f != nil })
	if f == nil {
		return nil, errors.New("the computer did not open its files")
	}
	// The computer reads the channel only once the link is in, a moment
	// after it opens: what comes before is dropped.
	c.mu.Lock()
	ready := c.filesAt
	if c.liveAt.After(ready) {
		ready = c.liveAt
	}
	c.mu.Unlock()
	if d := time.Until(ready.Add(filesSettle)); d > 0 {
		select {
		case <-time.After(d):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f, nil
}

func (s *Server) fileList(ctx context.Context, req *mcp.CallToolRequest, in pathIn) (*mcp.CallToolResult, any, error) {
	c, err := s.computer(ctx, req, in.Computer)
	var entries []viewer.FileEntry
	if err == nil {
		var f *viewer.Files
		if f, err = c.filesOf(ctx); err == nil {
			entries, err = f.List(in.Path)
		}
	}
	s.audit(req, "file_list", in.Computer, in.Path, err)
	if err != nil {
		return nil, nil, err
	}
	if len(entries) == 0 {
		return text("The folder is empty."), nil, nil
	}
	var b strings.Builder
	for _, e := range entries {
		when := time.Unix(e.MTime, 0).Format("2006-01-02 15:04")
		if e.Dir {
			fmt.Fprintf(&b, "%s/  (folder, %s)\n", e.Name, when)
		} else {
			fmt.Fprintf(&b, "%s  (%d bytes, %s)\n", e.Name, e.Size, when)
		}
	}
	return text("%s", strings.TrimRight(b.String(), "\n")), nil, nil
}

// downloadsDir is where downloads go: this computer's Downloads folder.
func (s *Server) downloadsDir() (string, error) {
	dir := s.opts.Downloads
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, "Downloads")
	}
	return dir, os.MkdirAll(dir, 0o755)
}

// maxShownText is the largest downloaded text file shown in the answer.
const maxShownText = 64 << 10

func (s *Server) fileDownload(ctx context.Context, req *mcp.CallToolRequest, in pathIn) (*mcp.CallToolResult, any, error) {
	c, err := s.computer(ctx, req, in.Computer)
	var saved string
	if err == nil && strings.TrimSpace(in.Path) == "" {
		err = errors.New("no path")
	}
	if err == nil {
		var f *viewer.Files
		var dir string
		if f, err = c.filesOf(ctx); err == nil {
			if dir, err = s.downloadsDir(); err == nil {
				saved, err = f.Download(in.Path, dir, nil, ctx.Done())
			}
		}
	}
	s.audit(req, "file_download", in.Computer, in.Path, err)
	if err != nil {
		return nil, nil, err
	}
	msg := "Saved as " + saved + "."
	if b, err := os.ReadFile(saved); err == nil && len(b) <= maxShownText && isText(b) {
		msg += "\n\n" + string(b)
	}
	return text("%s", msg), nil, nil
}

// isText: UTF-8 without NUL bytes.
func isText(b []byte) bool {
	return !strings.ContainsRune(string(b), 0) && strings.ToValidUTF8(string(b), "�") == string(b)
}

func (s *Server) fileUpload(ctx context.Context, req *mcp.CallToolRequest, in uploadIn) (*mcp.CallToolResult, any, error) {
	set, err := s.settings()
	if err == nil && !set.Act {
		err = errLook
	}
	if err == nil && !filepath.IsAbs(in.Path) {
		err = errors.New("give the file's absolute path on this computer")
	}
	var c *computer
	if err == nil {
		c, err = s.computer(ctx, req, in.Computer)
	}
	var name string
	if err == nil {
		var f *viewer.Files
		if f, err = c.filesOf(ctx); err == nil {
			name, err = f.Upload(in.Path, nil, ctx.Done())
		}
	}
	s.audit(req, "file_upload", in.Computer, in.Path, err)
	if err != nil {
		return nil, nil, err
	}
	return text("Sent: it is %s in the computer's Downloads folder.", name), nil, nil
}
