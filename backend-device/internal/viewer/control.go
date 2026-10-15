// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package viewer

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
)

// Limits of what the computer sends, as the website keeps them.
const (
	MaxChat      = 2000
	maxShapes    = 64
	maxShapePNG  = 300 << 10
	maxDisplays  = 16
	maxClipParts = services.MaxClipboard/services.ClipboardPiece + 1
)

// controlMsg is any control message: the fields of all of them.
type controlMsg struct {
	Type string `json:"type"`
	// welcome
	Host struct {
		Name string `json:"name"`
		OS   string `json:"os"`
	} `json:"host"`
	Source   string   `json:"source"`
	Features []string `json:"features"`
	// terminal_tabs, terminal_exit
	Tabs    []TerminalTab `json:"tabs"`
	Resumed bool          `json:"resumed"`
	// stream_stats
	FPS          float64 `json:"fps"`
	Width        float64 `json:"width"`
	Height       float64 `json:"height"`
	Kbps         float64 `json:"kbps"`
	Codec        string  `json:"codec"`
	TargetKbps   float64 `json:"target_kbps"`
	EstimateKbps float64 `json:"estimate_kbps"`
	// displays
	Displays []struct {
		ID           int    `json:"id"`
		Name         string `json:"name"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		PointsWidth  int    `json:"points_width"`
		PointsHeight int    `json:"points_height"`
		Primary      bool   `json:"primary"`
	} `json:"displays"`
	Current int `json:"current"`
	// permissions
	Permissions *models.Permissions `json:"permissions"`
	// notice, session_end, password_result
	Code   string `json:"code"`
	Reason string `json:"reason"`
	// cursor, cursor_pos
	ID      json.RawMessage `json:"id"`
	PNG     string          `json:"png"`
	HotX    float64         `json:"hot_x"`
	HotY    float64         `json:"hot_y"`
	Pixels  float64         `json:"pixels"`
	X       float64         `json:"x"`
	Y       float64         `json:"y"`
	Visible bool            `json:"visible"`
	// clipboard
	Part  int    `json:"part"`
	Parts int    `json:"parts"`
	Text  string `json:"text"`
	// link
	LinkID     string `json:"link_id"`
	Proof      string `json:"proof"`
	DeviceID   string `json:"device_id"`
	Token      string `json:"token"`
	OK         bool   `json:"ok"`
	Left       int    `json:"left"`
	RetryAfter int    `json:"retry_after"`
}

// onControl takes one message from the computer's control channel.
func (s *Session) onControl(data []byte) {
	var head struct {
		Type string          `json:"type"`
		ID   json.RawMessage `json:"id"`
	}
	if json.Unmarshal(data, &head) != nil {
		return
	}
	if head.Type == "ping" {
		b, _ := json.Marshal(map[string]any{"type": "pong", "id": head.ID})
		s.mu.Lock()
		p := s.peer
		s.mu.Unlock()
		if p != nil {
			p.sendControl(b)
		}
		return
	}
	ev := s.opts.Events
	if head.Type == "terminal_exit" {
		// Its code is a number; controlMsg's (a notice's) is text.
		var e struct {
			ID   uint32 `json:"id"`
			Code int    `json:"code"`
		}
		if json.Unmarshal(data, &e) == nil && ev.TerminalExit != nil {
			ev.TerminalExit(e.ID, e.Code)
		}
		return
	}
	var m controlMsg
	if json.Unmarshal(data, &m) != nil {
		return
	}
	switch m.Type {
	case "welcome":
		h := HostInfo{Name: truncate(m.Host.Name, 64), OS: truncate(m.Host.OS, 16), Terminal: m.Source == "terminal"}
		h.TerminalTabs = h.Terminal && slices.Contains(m.Features, "terminal-tabs")
		s.mu.Lock()
		s.welcome = h
		s.mu.Unlock()
		if ev.Host != nil {
			ev.Host(h)
		}
	case "terminal_tabs":
		tabs := m.Tabs
		if len(tabs) > maxTerminalTabs {
			tabs = tabs[:maxTerminalTabs]
		}
		for i := range tabs {
			tabs[i].Name = truncate(tabs[i].Name, 40)
		}
		if ev.TerminalTabs != nil {
			ev.TerminalTabs(tabs, m.Resumed)
		}

	case "stream_stats":
		if ev.Sent != nil {
			ev.Sent(Sent{FPS: m.FPS, Width: int(m.Width), Height: int(m.Height), Kbps: int(m.Kbps), Codec: truncate(m.Codec, 16),
				TargetKbps: int(m.TargetKbps), EstimateKbps: int(m.EstimateKbps)})
		}
	case "displays":
		var out []Display
		for i, d := range m.Displays {
			if i == maxDisplays {
				break
			}
			out = append(out, Display{ID: d.ID, Name: truncate(d.Name, 64), Width: d.Width, Height: d.Height,
				PointsWidth: d.PointsWidth, PointsHeight: d.PointsHeight, Primary: d.Primary})
		}
		if ev.Displays != nil {
			ev.Displays(out, m.Current)
		}
	case "permissions":
		if m.Permissions == nil {
			return
		}
		p := *m.Permissions
		p.View = true
		s.mu.Lock()
		s.perms = p
		s.mu.Unlock()
		s.recordAllowed(p.Record)
		if ev.Permissions != nil {
			ev.Permissions(p)
		}
	case "notice":
		if ev.Notice != nil {
			ev.Notice(truncate(m.Code, 40))
		}
	case "cursor":
		s.cursor(m)
	case "cursor_pos":
		if ev.CursorPos != nil {
			ev.CursorPos(m.X, m.Y, m.Visible)
		}
	case "clipboard":
		var id uint32
		_ = json.Unmarshal(m.ID, &id)
		text, ok := s.clipIn.add(id, m.Part, m.Parts, m.Text)
		if ok && s.Permissions().Clipboard && ev.Clipboard != nil {
			ev.Clipboard(text)
		}
	case "chat":
		text := truncate(m.Text, MaxChat)
		if text != "" && ev.Chat != nil {
			ev.Chat(ChatLine{FromHost: true, Text: text, At: time.Now()})
		}
	case "typing":
		var t struct {
			On bool `json:"on"`
		}
		if json.Unmarshal(data, &t) == nil && ev.Typing != nil {
			ev.Typing(t.On)
		}
	case "screenshot", "recording":
		s.answered(m.Type, data)
	case "session_end":
		s.finish(Status{Kind: StatusEnded, Reason: endReason(m.Reason)})
	default:
		s.onLinkMessage(m)
	}
}

// cursor keeps the computer's pointer images by id: each comes once.
func (s *Session) cursor(m controlMsg) {
	var id string
	if json.Unmarshal(m.ID, &id) != nil {
		return
	}
	id = truncate(id, 32)
	s.mu.Lock()
	shape, ok := s.shapes[id]
	if !ok && m.PNG != "" && len(m.PNG) < maxShapePNG && len(s.shapes) < maxShapes {
		png, err := base64.StdEncoding.DecodeString(m.PNG)
		if err == nil {
			shape = CursorShape{ID: id, PNG: png, HotX: m.HotX, HotY: m.HotY, Width: m.Width, Height: m.Height, Pixels: m.Pixels}
			s.shapes[id] = shape
			ok = true
		}
	}
	s.mu.Unlock()
	if ok && s.opts.Events.Cursor != nil {
		s.opts.Events.Cursor(shape)
	}
}

// onLinkMessage takes the link's messages: the computer's proof, the
// token, the password.
func (s *Session) onLinkMessage(m controlMsg) {
	door := s.opts.Door
	switch m.Type {
	case "auth_proof":
		s.mu.Lock()
		nonce := s.nonce
		s.mu.Unlock()
		if door.Kind != DoorReach || nonce == "" {
			return
		}
		if m.LinkID != door.Device.LinkID || !hmac.Equal([]byte(m.Proof), []byte(linkProof(door.Device.Token, nonce))) {
			s.finish(Status{Kind: StatusUntrusted})
			return
		}
		s.mu.Lock()
		s.nonce, s.proven = "", true
		s.mu.Unlock()
		s.sendControl(map[string]any{"type": "auth", "link_id": door.Device.LinkID, "token": door.Device.Token})
	case "auth_ok":
		if door.Kind == DoorClaim && m.Token != "" && m.DeviceID != "" && m.LinkID != "" {
			if fn := s.opts.Events.Linked; fn != nil {
				fn(NewLink{DeviceID: m.DeviceID, LinkID: m.LinkID, Token: m.Token})
			}
			return
		}
		s.mu.Lock()
		if door.Kind != DoorReach || !s.proven || m.LinkID != door.Device.LinkID {
			s.mu.Unlock()
			return
		}
		s.authed = true
		early := s.early
		s.early = nil
		s.mu.Unlock()
		if early != nil {
			s.admitted(*early)
		}
	case "auth_failed", "unlinked":
		if door.Kind == DoorJoin {
			return
		}
		if fn := s.opts.Events.Unlinked; fn != nil {
			fn()
		}
		s.finish(Status{Kind: StatusUnlinked})
	case "password_required":
		// Only a computer that proved itself and took the token may ask.
		s.mu.Lock()
		ok := door.Kind == DoorReach && s.authed && !s.status.Over()
		if ok {
			s.status = Status{Kind: StatusPassword}
		}
		s.mu.Unlock()
		if ok {
			s.emitStatus()
		}
	case "password_result":
		s.mu.Lock()
		if s.status.Kind != StatusPassword {
			s.mu.Unlock()
			return
		}
		if m.OK {
			s.status = Status{Kind: StatusWaiting}
			s.mu.Unlock()
			s.emitStatus()
			return
		}
		reason := m.Reason
		if reason != "blocked" && reason != "locked" {
			s.status = Status{Kind: StatusPassword, PasswordError: "wrong", Left: m.Left}
			s.mu.Unlock()
			s.emitStatus()
			return
		}
		s.mu.Unlock()
		s.finish(Status{Kind: StatusPasswordRefused, PasswordError: reason, RetryAfter: m.RetryAfter})
	}
}

// linkProof is what the linked computer must answer to nonce.
func linkProof(token, nonce string) string {
	h := sha256.Sum256([]byte(token))
	return services.LinkProof(h[:], nonce)
}

// randomNonce is 32 random bytes, base64url.
func randomNonce() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// clipboardAssembler puts a clipboard text's pieces back together.
type clipboardAssembler struct {
	id    uint32
	parts []string
}

func (c *clipboardAssembler) add(id uint32, part, parts int, text string) (string, bool) {
	if parts < 1 || parts > maxClipParts || part < 0 || part >= parts {
		return "", false
	}
	if part == 0 || id != c.id {
		c.id, c.parts = id, nil
	}
	if part != len(c.parts) {
		c.parts = nil
		return "", false
	}
	c.parts = append(c.parts, text)
	if len(c.parts) < parts {
		return "", false
	}
	out := strings.Join(c.parts, "")
	c.parts = nil
	if len(out) > services.MaxClipboard || !utf8.ValidString(out) {
		return "", false
	}
	return out, true
}

// The window's side: what it sends.

// SendPassword sends the unattended password the computer asked for.
func (s *Session) SendPassword(password string) {
	s.mu.Lock()
	if s.status.Kind != StatusPassword || password == "" {
		s.mu.Unlock()
		return
	}
	s.status.Checking = true
	s.mu.Unlock()
	s.emitStatus()
	s.sendControl(map[string]any{"type": "password", "password": truncate(password, 512)})
}

// SetVideoWant tells the computer the size the picture is shown at, in
// pixels.
func (s *Session) SetVideoWant(width, height int) {
	s.sendControl(map[string]any{"type": "video_want", "width": width, "height": height})
}

// Preset is a picture preset: speed, balanced or quality.
type Preset string

const (
	PresetSpeed    Preset = "speed"
	PresetBalanced Preset = "balanced"
	PresetQuality  Preset = "quality"
)

// SetQuality picks the preset, and whether to send the display's native
// pixels.
func (s *Session) SetQuality(p Preset, retina bool) {
	s.sendControl(map[string]any{"type": "quality", "preset": string(p), "retina": retina})
}

// SelectDisplay shows another of the computer's displays.
func (s *Session) SelectDisplay(id int) {
	s.sendControl(map[string]any{"type": "display_select", "id": id})
}

// SetCursorInVideo draws the pointer into the picture (true) instead of
// sending its shape and position.
func (s *Session) SetCursorInVideo(on bool) {
	s.sendControl(map[string]any{"type": "cursor_mode", "in_video": on})
}

// SendPointer sends a pointer move (unreliable).
func (s *Session) SendPointer(packet []byte) {
	if p := s.livePeer(); p != nil {
		p.sendPointer(packet)
	}
}

// SendInput sends a button, wheel, key, text or release-all packet.
func (s *Session) SendInput(packet []byte) {
	if p := s.livePeer(); p != nil {
		p.sendInput(packet)
	}
}

// SendClipboard puts text on the computer's clipboard (with the clipboard
// permission); paste also pastes it there. False if it is too long.
func (s *Session) SendClipboard(text string, paste bool) bool {
	if len(text) > services.MaxClipboard || !utf8.ValidString(text) {
		return false
	}
	s.mu.Lock()
	s.clipSeq++
	id := s.clipSeq
	s.mu.Unlock()
	for _, piece := range services.ClipboardPieces(id, text, paste) {
		if p := s.livePeer(); p != nil {
			p.sendControl(piece)
		}
	}
	return true
}

// SendChat writes to the person at the computer; it returns the line, or
// false when there is nothing to send.
func (s *Session) SendChat(text string) (ChatLine, bool) {
	clean := truncate(strings.TrimSpace(text), MaxChat)
	if clean == "" || s.Status().Over() {
		return ChatLine{}, false
	}
	s.sendControl(map[string]any{"type": "chat", "text": clean})
	return ChatLine{Text: clean, At: time.Now()}, true
}

// SetHidden says whether the window is out of sight (reported to the
// computer, which may send less).
// SendTyping tells the computer this side is writing a chat message (on),
// or stopped.
func (s *Session) SendTyping(on bool) {
	if s.Status().Over() {
		return
	}
	s.sendControl(map[string]any{"type": "typing", "on": on})
}

func (s *Session) SetHidden(hidden bool) {
	s.mu.Lock()
	s.isHidden = hidden
	s.mu.Unlock()
}

func (s *Session) hidden() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isHidden
}

// livePeer is the peer once the computer let this viewer in.
func (s *Session) livePeer() *peer {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.hasPerms || s.status.Over() {
		return nil
	}
	return s.peer
}

// filesOpen: the files channel opened.
func (s *Session) filesOpen(f *Files) {
	if fn := s.opts.Events.Files; fn != nil {
		fn(f)
	}
}

// RequestKeyframe asks the computer for a whole picture (the window could
// not draw what came).
func (s *Session) RequestKeyframe() {
	if p := s.livePeer(); p != nil {
		p.keyframe()
	}
}

// maxTerminalTabs bounds the terminals a computer may list.
const maxTerminalTabs = 32

// Host is what the computer said of itself (its welcome).
func (s *Session) Host() HostInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.welcome
}

// onTerminal takes a message of the terminal channel: a terminal's id and
// its bytes, or (an older computer) the one shell's bytes.
func (s *Session) onTerminal(b []byte) {
	ev := s.opts.Events.TerminalOutput
	if ev == nil {
		return
	}
	if !s.Host().TerminalTabs {
		ev(0, b)
		return
	}
	if len(b) < 4 {
		return
	}
	ev(binary.BigEndian.Uint32(b), b[4:])
}

// SendTerminal types into a terminal (only with the control permission;
// the computer checks too).
func (s *Session) SendTerminal(id uint32, data []byte) {
	if !s.Permissions().Control || len(data) == 0 {
		return
	}
	p := s.livePeer()
	if p == nil {
		return
	}
	if !s.Host().TerminalTabs {
		p.sendTerminal(data)
		return
	}
	f := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(f, id)
	copy(f[4:], data)
	p.sendTerminal(f)
}

// SendTerminalSize tells the computer the terminal's size here.
func (s *Session) SendTerminalSize(cols, rows int) {
	if cols < 1 || rows < 1 {
		return
	}
	s.sendJSON(map[string]any{"type": "terminal_size", "cols": min(cols, 1000), "rows": min(rows, 1000)})
}

// OpenTerminal opens another terminal on the computer (a name, or its own).
func (s *Session) OpenTerminal(name string) {
	s.sendJSON(map[string]any{"type": "terminal_open", "name": truncate(name, 40)})
}

// CloseTerminal closes a terminal: its shell ends.
func (s *Session) CloseTerminal(id uint32) {
	s.sendJSON(map[string]any{"type": "terminal_close", "id": id})
}

// RenameTerminal renames a terminal.
func (s *Session) RenameTerminal(id uint32, name string) {
	if name = strings.TrimSpace(truncate(name, 40)); name != "" {
		s.sendJSON(map[string]any{"type": "terminal_rename", "id": id, "name": name})
	}
}

// sendJSON sends a control message to the computer in session.
func (s *Session) sendJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	if p := s.livePeer(); p != nil {
		p.sendControl(b)
	}
}

// terminalChannel notes a terminal channel; a second one is a new
// connection whose histories come again.
func (s *Session) terminalChannel() {
	s.mu.Lock()
	s.termChans++
	again := s.termChans > 1
	s.mu.Unlock()
	if again && s.opts.Events.TerminalReset != nil {
		s.opts.Events.TerminalReset()
	}
}
