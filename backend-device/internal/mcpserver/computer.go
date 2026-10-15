// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package mcpserver

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/draw"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/viewer"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/decoder"
)

// session is what a computer's connection needs of viewer.Session (tests
// replace it).
type session interface {
	Dial(ctx context.Context)
	Status() viewer.Status
	Host() viewer.HostInfo
	Permissions() models.Permissions
	SendPassword(password string)
	SendTerminal(id uint32, data []byte)
	OpenTerminal(name string)
	SetHidden(hidden bool)
	RequestKeyframe()
	AskScreenshot(ctx context.Context) bool
	SendPointer(packet []byte)
	SendInput(packet []byte)
	SendClipboard(text string, paste bool) bool
	HangUp()
}

const (
	// keepOutput is what is kept of each terminal's output.
	keepOutput = 256 << 10
	// assistantTab names the terminal the assistant runs its commands in,
	// beside the person's own.
	assistantTab = "assistant"
	// shotWidth bounds a screenshot's width (wider ones are scaled down).
	shotWidth = 1568
)

// computer is one linked computer the assistant is connected to.
type computer struct {
	device models.RemoteDevice
	s      session

	mu       sync.Mutex
	changed  chan struct{} // closed and replaced on every change
	status   viewer.Status
	tabs     []viewer.TerminalTab
	out      map[uint32]*output
	exits    map[uint32]int
	mine     uint32 // the assistant's terminal (0: none yet)
	pass     string // the saved password, given when the computer asks
	asked    bool   // the password went once
	shot     chan *decoder.I420
	vp8      *decoder.VP8
	run      sync.Mutex // one command at a time
	shotW    int        // the last screenshot's size: click's x and y are on it
	shotH    int
	clip     string        // what the computer copied last
	files    *viewer.Files // its files channel, once open
	filesAt  time.Time     // when it opened
	liveAt   time.Time     // when the computer let the assistant in
	input    *viewer.Input
	end      context.CancelFunc
	lastSeen time.Time
}

// output is a terminal's recent output; total counts every byte so far.
type output struct {
	buf   []byte
	total int64
}

func newComputer(d models.RemoteDevice) *computer {
	return &computer{device: d, status: viewer.Status{Kind: viewer.StatusJoining}, changed: make(chan struct{}), out: map[uint32]*output{}, exits: map[uint32]int{}}
}

// events are the session's events for this computer.
func (c *computer) events() viewer.Events {
	return viewer.Events{
		Status: func(st viewer.Status) {
			c.mu.Lock()
			if st.Kind == viewer.StatusLive && c.status.Kind != viewer.StatusLive {
				c.liveAt = time.Now()
			}
			c.status = st
			send := st.Kind == viewer.StatusPassword && !st.Checking && st.PasswordError == "" && !c.asked
			if send {
				c.asked = true
			}
			c.changeLocked()
			c.mu.Unlock()
			if send {
				go c.sendPassword()
			}
		},
		TerminalTabs: func(tabs []viewer.TerminalTab, _ bool) {
			c.mu.Lock()
			c.tabs = tabs
			c.changeLocked()
			c.mu.Unlock()
		},
		TerminalOutput: func(id uint32, data []byte) {
			c.mu.Lock()
			o := c.out[id]
			if o == nil {
				o = &output{}
				c.out[id] = o
			}
			o.buf = append(o.buf, data...)
			if over := len(o.buf) - keepOutput; over > 0 {
				o.buf = append(o.buf[:0:0], o.buf[over:]...)
			}
			o.total += int64(len(data))
			c.changeLocked()
			c.mu.Unlock()
		},
		TerminalExit: func(id uint32, code int) {
			c.mu.Lock()
			c.exits[id] = code
			if c.mine == id {
				c.mine = 0
			}
			c.changeLocked()
			c.mu.Unlock()
		},
		TerminalReset: func() {
			c.mu.Lock()
			c.out = map[uint32]*output{}
			c.changeLocked()
			c.mu.Unlock()
		},
		Video: c.onVideo,
		Clipboard: func(text string) {
			c.mu.Lock()
			c.clip = text
			c.mu.Unlock()
		},
		Files: func(f *viewer.Files) {
			c.mu.Lock()
			c.files = f
			c.filesAt = time.Now()
			c.changeLocked()
			c.mu.Unlock()
		},
	}
}

// sendPassword gives the saved password, once, when the computer asks.
func (c *computer) sendPassword() {
	c.mu.Lock()
	pass := c.pass
	c.mu.Unlock()
	c.s.SendPassword(pass)
}

func (c *computer) changeLocked() {
	close(c.changed)
	c.changed = make(chan struct{})
}

// wait waits until ok (checked with c.mu held) or the context ends.
func (c *computer) wait(ctx context.Context, ok func() bool) bool {
	for {
		c.mu.Lock()
		if ok() {
			c.mu.Unlock()
			return true
		}
		ch := c.changed
		c.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return false
		}
	}
}

// connect waits until the computer let the assistant in (or ctx ends);
// the session itself lives until close.
func (c *computer) connect(ctx context.Context) error {
	life, end := context.WithCancel(context.Background())
	c.mu.Lock()
	c.end = end
	c.mu.Unlock()
	c.s.Dial(life)
	c.wait(ctx, func() bool {
		return c.status.Kind == viewer.StatusLive || c.status.Over() || c.status.PasswordError != ""
	})
	c.mu.Lock()
	st := c.status
	c.mu.Unlock()
	switch {
	case st.Kind == viewer.StatusLive:
	case st.PasswordError != "":
		c.close()
		return fmt.Errorf("the computer refused the saved password (%s): set it again in go-link Desk › Settings › Assistants", st.PasswordError)
	case ctx.Err() != nil:
		c.close()
		return fmt.Errorf("the computer did not answer in time (%s)", st.Kind)
	default:
		c.close()
		return fmt.Errorf("could not connect: %s", describe(st))
	}
	// No picture until a screenshot asks for one.
	c.s.SetHidden(true)
	if c.s.Host().TerminalTabs {
		wctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		c.wait(wctx, func() bool { return len(c.tabs) > 0 })
	}
	return nil
}

// describe is a status in words.
func describe(st viewer.Status) string {
	s := string(st.Kind)
	if st.Reason != "" {
		s += " (" + st.Reason + ")"
	}
	switch st.Kind {
	case viewer.StatusOffline:
		return "the computer is not on, or go-link Desk is not running there"
	case viewer.StatusUnlinked:
		return "the computer no longer has this link; link it again from go-link Desk"
	case viewer.StatusRejected:
		if st.Reason == "busy" {
			return "someone else is connected to it"
		}
	case viewer.StatusPasswordRefused:
		return "too many wrong passwords: the computer locked the password for a while"
	}
	return s
}

// live reports whether the connection is still up.
func (c *computer) live() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.status.Over()
}

// tabsText lists the terminals.
func (c *computer) tabsText() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.tabs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(c.tabs))
	for i, t := range c.tabs {
		p := fmt.Sprintf("%d %q", i+1, t.Name)
		if t.ID == c.mine {
			p += " (yours)"
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, ", ")
}

// assistantTerminal is the assistant's own terminal, opened the first time.
func (c *computer) assistantTerminal(ctx context.Context) (uint32, error) {
	if !c.s.Host().TerminalTabs {
		return 0, nil // an older computer: its one shell
	}
	c.mu.Lock()
	if c.mine != 0 && c.hasTabLocked(c.mine) {
		id := c.mine
		c.mu.Unlock()
		return id, nil
	}
	before := map[uint32]bool{}
	for _, t := range c.tabs {
		before[t.ID] = true
	}
	c.mu.Unlock()
	c.s.OpenTerminal(assistantTab)
	wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var id uint32
	c.wait(wctx, func() bool {
		for _, t := range c.tabs {
			if !before[t.ID] {
				id = t.ID
				return true
			}
		}
		return false
	})
	if id == 0 {
		return 0, errors.New("the computer did not open a terminal (it has 16 open already, or it refused)")
	}
	c.mu.Lock()
	c.mine = id
	c.mu.Unlock()
	// Its prompt first.
	qctx, qcancel := context.WithTimeout(ctx, 2*time.Second)
	defer qcancel()
	c.wait(qctx, func() bool { o := c.out[id]; return o != nil && o.total > 0 })
	quiet(ctx, c, id, 300*time.Millisecond)
	return id, nil
}

func (c *computer) hasTabLocked(id uint32) bool {
	for _, t := range c.tabs {
		if t.ID == id {
			return true
		}
	}
	return false
}

// tab finds a terminal by its number in the list (1, 2…), its name, or
// "" for the assistant's own (else the first).
func (c *computer) tab(which string) (uint32, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.s.Host().TerminalTabs {
		return 0, nil
	}
	if which == "" {
		if c.mine != 0 && c.hasTabLocked(c.mine) {
			return c.mine, nil
		}
		if len(c.tabs) > 0 {
			return c.tabs[0].ID, nil
		}
		return 0, errors.New("the computer has no terminal open")
	}
	if n, err := strconv.Atoi(which); err == nil && n >= 1 && n <= len(c.tabs) {
		return c.tabs[n-1].ID, nil
	}
	for _, t := range c.tabs {
		if strings.EqualFold(t.Name, which) {
			return t.ID, nil
		}
	}
	return 0, fmt.Errorf("no terminal %q; there are: %s", which, c.tabsTextLocked())
}

func (c *computer) tabsTextLocked() string {
	names := make([]string, 0, len(c.tabs))
	for i, t := range c.tabs {
		names = append(names, fmt.Sprintf("%d %q", i+1, t.Name))
	}
	return strings.Join(names, ", ")
}

// since is a terminal's output after the byte count from.
func (c *computer) since(id uint32, from int64) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.sinceLocked(id, from)...)
}

func (c *computer) sinceLocked(id uint32, from int64) []byte {
	o := c.out[id]
	if o == nil {
		return nil
	}
	skip := from - (o.total - int64(len(o.buf)))
	if skip < 0 {
		skip = 0
	}
	if skip > int64(len(o.buf)) {
		return nil
	}
	return o.buf[skip:]
}

func (c *computer) total(id uint32) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if o := c.out[id]; o != nil {
		return o.total
	}
	return 0
}

// quiet waits until a terminal has said nothing for d.
func quiet(ctx context.Context, c *computer, id uint32, d time.Duration) {
	last := c.total(id)
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(d):
		}
		n := c.total(id)
		if n == last {
			return
		}
		last = n
	}
}

// ranCommand is what a command printed and how it ended.
type ranCommand struct {
	Output   string
	ExitCode int
	Done     bool // false: still running when the time was up
}

// runCommand types a command into a terminal and waits for it to end: a
// mark printed after it tells its end and exit code.
func (c *computer) runCommand(ctx context.Context, id uint32, command string, timeout time.Duration) ranCommand {
	c.run.Lock()
	defer c.run.Unlock()
	nonce := make([]byte, 6)
	_, _ = rand.Read(nonce)
	mark := "__gldone_" + hex.EncodeToString(nonce)
	// The mark is printed joined, but typed apart, so the echo of what is
	// typed never looks like it.
	line := "{ " + strings.TrimRight(command, "\n") + "\n}; printf '\\n%s_%s %d\\n' __gldone " + hex.EncodeToString(nonce) + " $?\n"
	from := c.total(id)
	c.s.SendTerminal(id, []byte(line))
	end := regexp.MustCompile(regexp.QuoteMeta(mark) + ` (\d+)`)
	wctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	done := c.wait(wctx, func() bool { return end.Match(clean(append([]byte(nil), c.sinceLocked(id, from)...))) })
	text := string(clean(c.since(id, from)))
	// Leave out the echo of what was typed (up to the printf line).
	echo := "__gldone " + hex.EncodeToString(nonce)
	if i := strings.Index(text, echo); i >= 0 {
		if j := strings.IndexByte(text[i:], '\n'); j >= 0 {
			text = text[i+j+1:]
		} else {
			text = ""
		}
	}
	r := ranCommand{Done: done}
	if m := end.FindStringSubmatchIndex(text); m != nil {
		r.ExitCode, _ = strconv.Atoi(text[m[2]:m[3]])
		text = text[:m[0]]
	}
	r.Output = strings.TrimRight(strings.TrimSuffix(text, "\n"), " \n")
	return r
}

// screenshot asks the computer, then takes the next whole picture.
func (c *computer) screenshot(ctx context.Context) ([]byte, int, int, error) {
	if c.s.Host().Terminal {
		return nil, 0, 0, errors.New("this computer is a terminal: read it with terminal_read")
	}
	if !c.s.Permissions().Record {
		return nil, 0, 0, errors.New("this computer does not let this link take screenshots: allow \"Take screenshots and record the screen\" for it in its go-link Desk (Settings › Unattended access)")
	}
	if !c.s.AskScreenshot(ctx) {
		return nil, 0, 0, errors.New("the computer did not allow the screenshot")
	}
	ch := make(chan *decoder.I420, 1)
	c.mu.Lock()
	c.shot = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.shot = nil
		c.mu.Unlock()
		c.s.SetHidden(true)
	}()
	c.s.SetHidden(false)
	c.s.RequestKeyframe()
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	select {
	case pic := <-ch:
		jpg, w, h, err := encodeShot(pic)
		if err == nil {
			c.mu.Lock()
			c.shotW, c.shotH = w, h
			c.mu.Unlock()
		}
		return jpg, w, h, err
	case <-wctx.Done():
		return nil, 0, 0, errors.New("no picture came from the computer in time")
	}
}

// onVideo decodes a whole picture when a screenshot waits for one.
func (c *computer) onVideo(f viewer.Frame) {
	c.mu.Lock()
	ch := c.shot
	c.mu.Unlock()
	if ch == nil || !f.Keyframe || f.Codec != "vp8" {
		return
	}
	if c.vp8 == nil {
		d, err := decoder.NewVP8(2)
		if err != nil {
			return
		}
		c.vp8 = d
	}
	pic, err := c.vp8.Decode(f.Data)
	if err != nil || pic == nil {
		return
	}
	cp := *pic
	cp.Y, cp.U, cp.V = append([]byte(nil), pic.Y...), append([]byte(nil), pic.U...), append([]byte(nil), pic.V...)
	select {
	case ch <- &cp:
	default:
	}
}

// encodeShot makes a JPEG of a picture, no wider than shotWidth.
func encodeShot(p *decoder.I420) ([]byte, int, int, error) {
	img := &image.YCbCr{
		Y: p.Y, Cb: p.U, Cr: p.V, YStride: p.StrideY, CStride: p.StrideUV,
		SubsampleRatio: image.YCbCrSubsampleRatio420, Rect: image.Rect(0, 0, p.Width, p.Height),
	}
	var src image.Image = img
	w, h := p.Width, p.Height
	if w > shotWidth {
		h = h * shotWidth / w
		w = shotWidth
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Src, nil)
		src = dst
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, src, &jpeg.Options{Quality: 80}); err != nil {
		return nil, 0, 0, err
	}
	return b.Bytes(), w, h, nil
}

// close ends the connection.
func (c *computer) close() {
	c.s.HangUp()
	c.mu.Lock()
	if c.end != nil {
		c.end()
	}
	if c.vp8 != nil {
		c.vp8.Close()
		c.vp8 = nil
	}
	c.mu.Unlock()
}

// marks are terminal_run's own lines: the wrapper's end as it was typed,
// and the mark it printed (with the empty line before it).
var marks = regexp.MustCompile(`(?m)^> \}; printf '\\n%s_%s %d\\n' __gldone [0-9a-f]+ \$\?\n|\n?__gldone_[0-9a-f]+ \d+\n?`)

// tidy is a terminal's text without terminal_run's marks.
func tidy(s string) string { return marks.ReplaceAllString(s, "") }

var ansi = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[()][0-9A-Za-z]|\x1b[=>78DEHMNOZc]`)

// clean is terminal output as plain text: no escape sequences, and of a
// line rewritten with \r only what it shows last.
func clean(b []byte) []byte {
	b = ansi.ReplaceAll(b, nil)
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	lines := bytes.Split(b, []byte("\n"))
	for i, l := range lines {
		if j := bytes.LastIndexByte(l, '\r'); j >= 0 {
			lines[i] = l[j+1:]
		}
		lines[i] = backspaces(lines[i])
	}
	return bytes.Join(lines, []byte("\n"))
}

// backspaces applies \b as a terminal shows it.
func backspaces(l []byte) []byte {
	if bytes.IndexByte(l, '\b') < 0 {
		return l
	}
	out := make([]byte, 0, len(l))
	for _, ch := range l {
		if ch == '\b' {
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
			continue
		}
		out = append(out, ch)
	}
	return out
}
