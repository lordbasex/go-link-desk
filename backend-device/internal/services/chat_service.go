// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
)

// ChatMessage is one line of the session's chat.
type ChatMessage struct {
	FromViewer bool // false: the person at the computer wrote it
	Text       string
	At         time.Time
}

// ChatService is the session's chat: "chat" messages {text} both ways on
// the control channel, with the viewer in session. What the viewer writes
// is cleaned (printable text, bounded) before anyone sees it.
type ChatService struct {
	send   func(peerID string, msg []byte) bool
	logger *slog.Logger

	mu        sync.Mutex
	peer      string // the viewer in session, "" for none
	lines     []ChatMessage
	onMessage func(ChatMessage)
}

// maxChatLog bounds the messages a session keeps.
const maxChatLog = 200

// NewChatService builds the service; send delivers a control message.
func NewChatService(send func(peerID string, msg []byte) bool, log *slog.Logger) *ChatService {
	if log == nil {
		log = slog.Default()
	}
	return &ChatService{send: send, logger: log}
}

// OnMessage is told of every message, the host's own included.
func (c *ChatService) OnMessage(fn func(ChatMessage)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onMessage = fn
}

// SetPeer starts a session's chat with a viewer, or ends it ("" ). A new
// session starts with an empty chat.
func (c *ChatService) SetPeer(peerID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.peer != peerID {
		c.lines = nil
	}
	c.peer = peerID
}

// Messages is the session's chat so far.
func (c *ChatService) Messages() []ChatMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]ChatMessage(nil), c.lines...)
}

// Send writes to the viewer in session; false if there is none or the
// text is empty.
func (c *ChatService) Send(text string) bool {
	text = models.CleanChat(text)
	c.mu.Lock()
	peer := c.peer
	c.mu.Unlock()
	if peer == "" || text == "" {
		return false
	}
	b, _ := json.Marshal(struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}{"chat", text})
	if !c.send(peer, b) {
		return false
	}
	c.add(ChatMessage{Text: text, At: time.Now()})
	return true
}

// Handle takes a "chat" message from a viewer.
func (c *ChatService) Handle(peerID string, data []byte) {
	var m struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(data, &m) != nil {
		return
	}
	text := models.CleanChat(m.Text)
	c.mu.Lock()
	ok := peerID == c.peer && peerID != ""
	c.mu.Unlock()
	if ok && text != "" {
		// The log has its length, never its words.
		c.logger.Info("chat message from the viewer", "peer_id", peerID, "chars", len([]rune(text)))
		c.add(ChatMessage{FromViewer: true, Text: text, At: time.Now()})
	}
}

func (c *ChatService) add(m ChatMessage) {
	c.mu.Lock()
	c.lines = append(c.lines, m)
	if len(c.lines) > maxChatLog {
		c.lines = c.lines[len(c.lines)-maxChatLog:]
	}
	fn := c.onMessage
	c.mu.Unlock()
	if fn != nil {
		fn(m)
	}
}
