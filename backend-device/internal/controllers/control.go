// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package controllers routes the messages a viewer sends on its control
// channel to the services that handle them. go-link keeps this routing in
// one big switch in its main.go; here each message type registers its own
// handler, and an unknown type is dropped.
package controllers

import (
	"log/slog"
	"sync"
)

// Handler takes one control message (the whole JSON object) from a
// viewer.
type Handler func(peerID string, data []byte)

// Control is the router of viewers' control messages.
type Control struct {
	log      *slog.Logger
	mu       sync.RWMutex
	handlers map[string]Handler
}

// NewControl builds an empty router.
func NewControl(log *slog.Logger) *Control {
	if log == nil {
		log = slog.Default()
	}
	return &Control{log: log, handlers: make(map[string]Handler)}
}

// Handle registers the handler of one message type, replacing any other.
func (c *Control) Handle(kind string, h Handler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers[kind] = h
}

// Dispatch hands a message to its type's handler. Types nobody handles
// (a newer website, or a viewer up to something) are dropped.
func (c *Control) Dispatch(peerID, kind string, data []byte) {
	c.mu.RLock()
	h := c.handlers[kind]
	c.mu.RUnlock()
	if h == nil {
		c.log.Debug("control message ignored", "peer_id", peerID, "type", kind)
		return
	}
	h(peerID, data)
}

// Session is what the session messages need.
type Session interface {
	ViewerHungUp(peerID string)
}

// SessionRoutes registers the messages that drive the session itself:
// session_end, the viewer hanging up.
func SessionRoutes(c *Control, s Session) {
	c.Handle("session_end", func(peerID string, _ []byte) { s.ViewerHungUp(peerID) })
}
