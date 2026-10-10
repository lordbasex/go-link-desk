// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package signalclient

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ErrNotConnected is returned by Send while there is no live connection.
var ErrNotConnected = errors.New("signalclient: not connected")

// Handler receives connection events. All methods are called from the
// client's read goroutine, one at a time, so a Handler does not need its
// own locking for these calls. Send may be called from inside them.
type Handler interface {
	// OnConnect runs after the server's hello arrives.
	OnConnect(hello Envelope)
	// OnMessage runs for every message after hello.
	OnMessage(env Envelope)
	// OnDisconnect runs when a connection attempt fails or a live
	// connection drops. The client then waits and reconnects.
	OnDisconnect(err error)
}

// Config configures a Client. Zero values get defaults.
type Config struct {
	URL        string      // ws:// or wss:// endpoint, e.g. wss://signal.example/ws
	Header     http.Header // extra handshake headers
	MinBackoff time.Duration
	MaxBackoff time.Duration
	PongWait   time.Duration // max silence before the link is considered dead
	WriteWait  time.Duration
	Logger     *slog.Logger
}

// Client keeps a signalhub connection alive.
type Client struct {
	cfg Config
	h   Handler

	mu   sync.Mutex // guards conn and serializes writes
	conn *websocket.Conn
}

// New builds a Client. Call Run to start it.
func New(cfg Config, h Handler) *Client {
	if cfg.MinBackoff <= 0 {
		cfg.MinBackoff = time.Second
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 30 * time.Second
	}
	if cfg.PongWait <= 0 {
		// The server pings every 54s; allow a margin for slow links.
		cfg.PongWait = 75 * time.Second
	}
	if cfg.WriteWait <= 0 {
		cfg.WriteWait = 10 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Client{cfg: cfg, h: h}
}

// CheckURL validates a signalhub URL. It reports whether the link is
// protected: wss:// always is; ws:// is accepted only because loopback
// development servers have no TLS.
func CheckURL(raw string) (encrypted bool, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return false, fmt.Errorf("signalclient: bad URL: %w", err)
	}
	switch u.Scheme {
	case "wss":
		encrypted = true
	case "ws":
	default:
		return false, fmt.Errorf("signalclient: URL scheme must be ws or wss, got %q", u.Scheme)
	}
	if u.Host == "" {
		return false, fmt.Errorf("signalclient: URL %q has no host", raw)
	}
	return encrypted, nil
}

// endpoint adds ?v=1 unless the URL already names a version.
func endpoint(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("signalclient: bad URL: %w", err)
	}
	if u.Scheme != "ws" && u.Scheme != "wss" {
		return "", fmt.Errorf("signalclient: URL scheme must be ws or wss, got %q", u.Scheme)
	}
	q := u.Query()
	if q.Get("v") == "" {
		q.Set("v", ProtocolVersion)
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

// Run connects and reconnects until ctx is cancelled. It only returns an
// error for a bad configuration or when ctx ends.
func (c *Client) Run(ctx context.Context) error {
	target, err := endpoint(c.cfg.URL)
	if err != nil {
		return err
	}
	backoff := c.cfg.MinBackoff
	for {
		connected, err := c.session(ctx, target)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		c.h.OnDisconnect(err)
		if connected {
			backoff = c.cfg.MinBackoff
		}
		// Full jitter: many devices restarting together do not all
		// reconnect at the same instant.
		wait := time.Duration(rand.Int64N(int64(backoff))) + c.cfg.MinBackoff/2
		c.cfg.Logger.Debug("signal reconnect scheduled", "in", wait, "err", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
		backoff = min(backoff*2, c.cfg.MaxBackoff)
	}
}

// session runs one connection. connected reports whether hello arrived.
func (c *Client) session(ctx context.Context, target string) (connected bool, err error) {
	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second, Proxy: http.ProxyFromEnvironment}
	conn, _, err := dialer.DialContext(ctx, target, c.cfg.Header)
	if err != nil {
		return false, err
	}
	defer conn.Close()

	// Close the socket when ctx ends so the blocking read returns.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	conn.SetReadLimit(64 << 10)
	_ = conn.SetReadDeadline(time.Now().Add(c.cfg.PongWait))
	conn.SetPingHandler(func(data string) error {
		_ = conn.SetReadDeadline(time.Now().Add(c.cfg.PongWait))
		return conn.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(c.cfg.WriteWait))
	})

	var hello Envelope
	if err := conn.ReadJSON(&hello); err != nil {
		return false, err
	}
	if hello.Type != TypeHello {
		return false, fmt.Errorf("signalclient: expected hello, got %q", hello.Type)
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.conn = nil
		c.mu.Unlock()
	}()

	c.h.OnConnect(hello)
	for {
		var env Envelope
		if err := conn.ReadJSON(&env); err != nil {
			return true, err
		}
		_ = conn.SetReadDeadline(time.Now().Add(c.cfg.PongWait))
		c.h.OnMessage(env)
	}
}

// Send writes one message. It is safe for concurrent use.
func (c *Client) Send(env Envelope) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return ErrNotConnected
	}
	_ = c.conn.SetWriteDeadline(time.Now().Add(c.cfg.WriteWait))
	return c.conn.WriteJSON(env)
}
