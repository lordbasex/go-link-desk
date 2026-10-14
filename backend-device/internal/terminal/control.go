// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !windows

package terminal

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
)

// Control is the running service's local door for its own commands
// (go-link-desk-terminal status, password, link, unlink): a Unix socket
// next to device.json that only this user can open (the folder is 0700,
// the socket 0600, and on Linux the caller's user is checked too). Nothing
// on it ever reaches the network. Root may use it too: root can do
// anything on the computer already.
//
// It is also the service's Consent: a computer without a screen has
// nobody to accept a viewer, except the person who runs
// "go-link-desk-terminal link" and answers in that terminal. Only a
// browser that came with the pairing code to be linked is asked; anyone
// else is turned down.
type Control struct {
	Path string
	Log  *slog.Logger
	// Status describes the service; Code is the pairing code now ("" none).
	Status      func() Status
	Code        func() string
	SetPassword func(password string) error
	Unlink      func(id string) bool // "*": every one
	// Reset forgets every link and the password, and ends the session.
	Reset func() error

	mu     sync.Mutex
	linker *linker // the "link" command waiting, if any
}

// Status is what "go-link-desk-terminal status" shows.
type Status struct {
	Version    string     `json:"version"`
	DeviceID   string     `json:"device_id"`
	Signal     string     `json:"signal"`
	Name       string     `json:"name"`
	User       string     `json:"user"` // whose shell sessions get
	WebURL     string     `json:"web_url"`
	Unattended bool       `json:"unattended"` // a password is set
	Session    string     `json:"session,omitempty"`
	Links      []LinkInfo `json:"links"`
}

// LinkInfo is a linked browser, as the status lists it.
type LinkInfo struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Created  time.Time `json:"created"`
	LastSeen time.Time `json:"last_seen"`
}

// Request is a command's one line to the service.
type Request struct {
	Cmd      string `json:"cmd"` // status, password, link, unlink, reset
	Password string `json:"password,omitempty"`
	ID       string `json:"id,omitempty"`
}

// Reply is a line back. A link command gets several: the code (again when
// it changes), the question, and how it ended.
type Reply struct {
	OK     bool    `json:"ok,omitempty"`
	Error  string  `json:"error,omitempty"`
	Status *Status `json:"status,omitempty"`
	Code   string  `json:"code,omitempty"`
	Ask    *Asking `json:"ask,omitempty"`
	Linked string  `json:"linked,omitempty"` // the browser linked
}

// Asking is a browser that wants to be linked.
type Asking struct {
	Name    string `json:"name"`
	Browser string `json:"browser"`
}

// Answer is the link command's yes or no.
type Answer struct {
	Answer bool `json:"answer"`
}

type linker struct {
	asks chan linkAsk
}

type linkAsk struct {
	req   services.ConsentRequest
	reply chan bool
}

// LinkPermissions are a terminal link's: the computer is its owner's, so
// they see and type in its terminals, copy and paste, and may save
// screenshots and recordings.
var LinkPermissions = models.Permissions{View: true, Control: true, Clipboard: true, Record: true}

// Serve listens until ctx ends.
func (c *Control) Serve(ctx context.Context) error {
	_ = os.Remove(c.Path) // a socket left by a service that stopped
	l, err := net.Listen("unix", c.Path)
	if err != nil {
		return err
	}
	if err := os.Chmod(c.Path, 0o600); err != nil {
		_ = l.Close()
		return err
	}
	go func() {
		<-ctx.Done()
		_ = l.Close()
		_ = os.Remove(c.Path)
	}()
	for {
		conn, err := l.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go c.handle(ctx, conn.(*net.UnixConn))
	}
}

func (c *Control) handle(ctx context.Context, conn *net.UnixConn) {
	defer conn.Close()
	if !sameUser(conn) {
		c.Log.Warn("terminal: a command from another user was refused")
		return
	}
	r := bufio.NewReader(conn)
	enc := json.NewEncoder(conn)
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	line, err := r.ReadBytes('\n')
	if err != nil {
		return
	}
	_ = conn.SetReadDeadline(time.Time{})
	var req Request
	if json.Unmarshal(line, &req) != nil {
		_ = enc.Encode(Reply{Error: "not a request"})
		return
	}
	switch req.Cmd {
	case "status":
		st := c.Status()
		_ = enc.Encode(Reply{OK: true, Status: &st})
	case "password":
		if err := c.SetPassword(req.Password); err != nil {
			_ = enc.Encode(Reply{Error: err.Error()})
			return
		}
		c.Log.Info("terminal: the unattended password was set")
		_ = enc.Encode(Reply{OK: true})
	case "unlink":
		if !c.Unlink(req.ID) {
			_ = enc.Encode(Reply{Error: "no linked browser " + req.ID})
			return
		}
		_ = enc.Encode(Reply{OK: true})
	case "reset":
		if err := c.Reset(); err != nil {
			_ = enc.Encode(Reply{Error: err.Error()})
			return
		}
		c.Log.Info("terminal: reset: every link and the password were forgotten")
		_ = enc.Encode(Reply{OK: true})
	case "link":
		c.link(ctx, r, enc)
	default:
		_ = enc.Encode(Reply{Error: "unknown command " + req.Cmd})
	}
}

// link shows the pairing code until a browser asks, asks, and answers.
func (c *Control) link(ctx context.Context, r *bufio.Reader, enc *json.Encoder) {
	if !c.Status().Unattended {
		_ = enc.Encode(Reply{Error: "set a password first: go-link-desk-terminal password"})
		return
	}
	lk := &linker{asks: make(chan linkAsk)}
	c.mu.Lock()
	if c.linker != nil {
		c.mu.Unlock()
		_ = enc.Encode(Reply{Error: "another go-link-desk-terminal link is waiting"})
		return
	}
	c.linker = lk
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.linker = nil
		c.mu.Unlock()
	}()
	// The command's answers; its connection closing is a no.
	answers := make(chan bool)
	gone := make(chan struct{})
	go func() {
		defer close(gone)
		for {
			line, err := r.ReadBytes('\n')
			if err != nil {
				return
			}
			var a Answer
			if json.Unmarshal(line, &a) == nil {
				select {
				case answers <- a.Answer:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	shown := ""
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		if code := c.Code(); code != shown {
			shown = code
			if enc.Encode(Reply{Code: code}) != nil {
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-gone:
			return
		case <-tick.C:
		case ask := <-lk.asks:
			k := ask.req.Knock
			if enc.Encode(Reply{Ask: &Asking{Name: k.Name, Browser: k.Browser}}) != nil {
				ask.reply <- false
				return
			}
			var yes bool
			select {
			case yes = <-answers:
			case <-gone:
			case <-ctx.Done():
			case <-time.After(services.ConsentTimeout):
			}
			ask.reply <- yes
			if yes {
				name := k.Browser
				if k.Name != "" {
					name = k.Name + " (" + k.Browser + ")"
				}
				_ = enc.Encode(Reply{OK: true, Linked: name})
				return
			}
		}
	}
}

// Ask is the service's consent: only a browser asking to be linked, and
// only while "link" waits in a terminal to answer.
func (c *Control) Ask(ctx context.Context, req services.ConsentRequest) (models.Permissions, error) {
	if !req.Link {
		return models.Permissions{}, services.ErrUnavailable
	}
	c.mu.Lock()
	lk := c.linker
	c.mu.Unlock()
	if lk == nil {
		return models.Permissions{}, services.ErrUnavailable
	}
	ask := linkAsk{req: req, reply: make(chan bool, 1)}
	select {
	case lk.asks <- ask:
	case <-ctx.Done():
		return models.Permissions{}, ctx.Err()
	}
	select {
	case yes := <-ask.reply:
		if !yes {
			return models.Permissions{}, services.ErrDeclined
		}
		c.Log.Info("terminal: a browser was linked", "name", req.Knock.Name, "browser", req.Knock.Browser)
		return LinkPermissions, nil
	case <-ctx.Done():
		return models.Permissions{}, ctx.Err()
	}
}

// Dial opens the running service's socket, for a command.
func Dial(path string) (net.Conn, error) {
	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		var ne *net.OpError
		if errors.As(err, &ne) {
			return nil, errors.New("go-link-desk-terminal is not running: start it (systemctl --user start go-link-desk-terminal)")
		}
		return nil, err
	}
	return conn, nil
}
