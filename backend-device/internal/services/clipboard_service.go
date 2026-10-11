// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/clipboard"
)

// Clipboard limits: a piece fits well under a DataChannel message, and the
// whole text under what anyone pastes by hand.
const (
	ClipboardPiece = 16 << 10
	MaxClipboard   = 1 << 20
	// clipboardEvery is how often the host's clipboard is looked at.
	clipboardEvery = 300 * time.Millisecond
)

// clipboardMsg is one piece of a clipboard's text ("clipboard" control
// message). A text travels as parts pieces with the same id; paste (viewer
// to host, on the last piece) asks the host to paste it where the cursor
// is, as the viewer's Cmd+V or Ctrl+V meant.
type clipboardMsg struct {
	Type  string `json:"type"`
	ID    uint32 `json:"id"`
	Part  int    `json:"part"`
	Parts int    `json:"parts"`
	Text  string `json:"text"`
	Paste bool   `json:"paste,omitempty"`
}

// ClipboardService shares the clipboard's text with the viewer allowed to
// (Permissions.Clipboard), both ways: what is copied on the host goes to
// the viewer, and what the viewer pastes lands on the host's clipboard.
type ClipboardService struct {
	board clipboard.Board
	send  func(peerID string, msg []byte) bool
	paste func(peerID string) // presses the host's paste keys
	log   *slog.Logger
	every time.Duration

	mu      sync.Mutex
	peer    string             // the viewer allowed, "" for none
	stop    context.CancelFunc // its watcher
	seen    int                // the host's change count already handled
	nextID  uint32
	parts   []string // the viewer's text being put together
	partsID uint32
}

// NewClipboardService builds the service. send delivers a control message
// to a viewer; paste presses the host's paste shortcut for a viewer that
// may control (nil: none).
func NewClipboardService(board clipboard.Board, send func(string, []byte) bool, paste func(string), log *slog.Logger) *ClipboardService {
	if log == nil {
		log = slog.Default()
	}
	return &ClipboardService{board: board, send: send, paste: paste, log: log, every: clipboardEvery}
}

// Allow starts or stops sharing with a viewer. Only what is copied from
// then on is shared, never what was already on the clipboard.
func (c *ClipboardService) Allow(peerID string, on bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if on && c.peer == peerID {
		return
	}
	if !on && c.peer != peerID {
		return
	}
	if c.stop != nil {
		c.stop()
		c.stop = nil
	}
	c.peer, c.parts = "", nil
	if !on {
		return
	}
	c.peer = peerID
	c.seen = c.board.ChangeCount()
	ctx, stop := context.WithCancel(context.Background())
	c.stop = stop
	go c.watch(ctx, peerID)
	c.log.Info("clipboard shared", "peer_id", peerID)
}

// watch sends the viewer each new text copied on the host.
func (c *ClipboardService) watch(ctx context.Context, peerID string) {
	t := time.NewTicker(c.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		n := c.board.ChangeCount()
		c.mu.Lock()
		if c.peer != peerID || n == c.seen {
			c.mu.Unlock()
			continue
		}
		c.seen = n
		c.nextID++
		id := c.nextID
		c.mu.Unlock()
		text, ok := c.board.Text()
		if !ok || text == "" || len(text) > MaxClipboard {
			continue // an image, a file, or too much
		}
		for _, m := range ClipboardPieces(id, text, false) {
			if !c.send(peerID, m) {
				break
			}
		}
	}
}

// Handle takes a "clipboard" piece from a viewer. A text whose pieces all
// arrived goes onto the host's clipboard, and is pasted if asked for.
func (c *ClipboardService) Handle(peerID string, data []byte) {
	var m clipboardMsg
	if json.Unmarshal(data, &m) != nil || m.Parts < 1 || m.Parts > MaxClipboard/ClipboardPiece+1 || m.Part < 0 || m.Part >= m.Parts {
		return
	}
	c.mu.Lock()
	if c.peer != peerID {
		c.mu.Unlock()
		return
	}
	if m.Part == 0 || m.ID != c.partsID {
		c.parts, c.partsID = nil, m.ID
	}
	if m.Part != len(c.parts) { // a piece out of order: drop the text
		c.parts = nil
		c.mu.Unlock()
		return
	}
	c.parts = append(c.parts, m.Text)
	if len(c.parts) < m.Parts {
		c.mu.Unlock()
		return
	}
	var text string
	for _, p := range c.parts {
		text += p
	}
	c.parts = nil
	if len(text) > MaxClipboard || !utf8.ValidString(text) {
		c.mu.Unlock()
		return
	}
	// The host's own change is not sent back to the viewer.
	c.seen = c.board.SetText(text)
	c.mu.Unlock()
	if m.Paste && c.paste != nil {
		c.paste(peerID)
	}
}

// ClipboardPieces cuts text into messages of at most ClipboardPiece bytes
// of text each, never splitting a character.
func ClipboardPieces(id uint32, text string, paste bool) [][]byte {
	var pieces []string
	for len(text) > ClipboardPiece {
		cut := ClipboardPiece
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		pieces = append(pieces, text[:cut])
		text = text[cut:]
	}
	pieces = append(pieces, text)
	out := make([][]byte, 0, len(pieces))
	for i, p := range pieces {
		b, _ := json.Marshal(clipboardMsg{Type: "clipboard", ID: id, Part: i, Parts: len(pieces), Text: p, Paste: paste && i == len(pieces)-1})
		out = append(out, b)
	}
	return out
}
