// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
)

// ConsentPrompt is a request as the app shows it.
type ConsentPrompt struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Browser  string `json:"browser"`
	Link     bool   `json:"link"`
	Deadline int64  `json:"deadline"` // unix milliseconds
	Warning  bool   `json:"warning"`
}

// SocketConsent asks through an app on the other end of a socket: there
// is no window here. The app gone means nobody can accept, and that is a
// refusal, never a default: Ask fails as NobodyToAsk does.
type SocketConsent struct {
	// Show puts the prompt on the app (nil: it closes).
	Show func(*ConsentPrompt)
	// Ready, when set, runs before anyone is bothered: an error refuses
	// the visitor at once (the session is taken by another door, say).
	Ready func() error
	// Warned reports whether the scam warning was shown once already, and
	// SetWarned keeps that it has been.
	Warned    func() bool
	SetWarned func()

	mu    sync.Mutex
	seq   int
	open  *pendingConsent
	gone  bool
	goneC chan struct{}
}

type pendingConsent struct {
	id     string
	answer chan consentAnswer
}

type consentAnswer struct {
	accept bool
	perm   models.Permissions
}

func (c *SocketConsent) goneCh() chan struct{} {
	if c.goneC == nil {
		c.goneC = make(chan struct{})
	}
	return c.goneC
}

// Gone says the app left: whoever waits is turned down, and so is whoever
// comes after.
func (c *SocketConsent) Gone() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.gone {
		c.gone = true
		close(c.goneCh())
	}
}

// Ask shows the request and waits for the app, ctx, or the app leaving.
func (c *SocketConsent) Ask(ctx context.Context, req ConsentRequest) (models.Permissions, error) {
	if c.Ready != nil {
		if err := c.Ready(); err != nil {
			return models.Permissions{}, err
		}
	}
	c.mu.Lock()
	if c.gone || c.Show == nil {
		c.mu.Unlock()
		return models.Permissions{}, ErrUnavailable
	}
	gone := c.goneCh()
	c.seq++
	p := &pendingConsent{id: "c" + strconv.Itoa(c.seq), answer: make(chan consentAnswer, 1)}
	c.open = p
	c.mu.Unlock()
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(ConsentTimeout)
	}
	warning := c.Warned != nil && !c.Warned()
	c.Show(&ConsentPrompt{ID: p.id, Name: req.Knock.Name, Browser: req.Knock.Browser, Link: req.Link, Deadline: deadline.UnixMilli(), Warning: warning})
	if warning && c.SetWarned != nil {
		c.SetWarned()
	}
	defer func() {
		c.mu.Lock()
		if c.open == p {
			c.open = nil
		}
		c.mu.Unlock()
		c.Show(nil)
	}()
	select {
	case a := <-p.answer:
		if !a.accept {
			return models.Permissions{}, ErrDeclined
		}
		return a.perm, nil
	case <-gone:
		return models.Permissions{}, ErrUnavailable
	case <-ctx.Done():
		return models.Permissions{}, ctx.Err()
	}
}

// Answer takes the app's answer to the request id; false if nothing waits
// for it (it ran out, or it's an old one).
func (c *SocketConsent) Answer(id string, accept bool, perm models.Permissions) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.open == nil || c.open.id != id {
		return false
	}
	select {
	case c.open.answer <- consentAnswer{accept, perm}:
		return true
	default:
		return false
	}
}

// SocketCallConsent asks about a call through the app.
type SocketCallConsent struct {
	// Show puts the question on the app (name "": it closes).
	Show func(name string, video bool)

	mu   sync.Mutex
	open chan CallAnswer
	gone chan struct{}
	dead bool
}

// Gone turns down any call that asks now or later.
func (c *SocketCallConsent) Gone() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gone == nil {
		c.gone = make(chan struct{})
	}
	if !c.dead {
		c.dead = true
		close(c.gone)
	}
}

// AskCall waits for the app, ctx or the app leaving.
func (c *SocketCallConsent) AskCall(ctx context.Context, name string, video bool) (CallAnswer, error) {
	c.mu.Lock()
	if c.gone == nil {
		c.gone = make(chan struct{})
	}
	if c.dead {
		c.mu.Unlock()
		return CallAnswer{}, ErrUnavailable
	}
	gone := c.gone
	ch := make(chan CallAnswer, 1)
	c.open = ch
	c.mu.Unlock()
	c.Show(name, video)
	defer func() {
		c.mu.Lock()
		if c.open == ch {
			c.open = nil
		}
		c.mu.Unlock()
		c.Show("", false)
	}()
	select {
	case a := <-ch:
		return a, nil
	case <-gone:
		return CallAnswer{}, ErrUnavailable
	case <-ctx.Done():
		return CallAnswer{}, ctx.Err()
	}
}

// Answer takes the app's answer; false if nothing asks.
func (c *SocketCallConsent) Answer(a CallAnswer) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.open == nil {
		return false
	}
	select {
	case c.open <- a:
		return true
	default:
		return false
	}
}
