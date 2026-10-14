// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !windows

package terminal

import (
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/pion/webrtc/v4"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
)

// Channel is the DataChannel the terminal travels on: the shell's output
// to the viewer, what the viewer types to the shell, as bytes.
const Channel = "terminal"

const (
	defaultCols, defaultRows = 80, 24
	maxSize                  = 1000 // columns or rows a viewer may ask for
	readChunk                = 32 << 10
	// The channel's buffer: past high the shell's output waits until it
	// drains to low, so a flood (cat of a big file) never fills memory.
	highWater = 4 << 20
	lowWater  = 1 << 20
	// What the shell writes before the viewer's channel opens.
	maxPending = 1 << 20
)

// Service gives each viewer in session a shell of its own: started when
// the session lets the viewer see (Allow), typed into only with Control,
// and hung up when the session ends. The shell ending ends the session.
type Service struct {
	shell  ShellConfig
	send   func(peerID string, msg []byte) bool // the control channel
	log    *slog.Logger
	onExit func(peerID string)
	start  func(ShellConfig, uint16, uint16) (shellIO, error)
	typed  func(peerID string)

	mu      sync.Mutex
	peers   map[string]*peer
	allowed map[string]models.Permissions // the session's, which may come before the channel
}

// shellIO is what Service needs of a Shell (a test's fake has it too).
type shellIO interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Resize(cols, rows uint16) error
	Done() <-chan struct{}
	ExitCode() int
	Close()
}

type peer struct {
	dc      *webrtc.DataChannel
	open    bool
	control bool
	sh      shellIO
	pending [][]byte
	queued  int
	cols    uint16
	rows    uint16
	drained chan struct{} // the channel's buffer went under lowWater
	gone    chan struct{} // closed with the channel or the session
	goneSet sync.Once
}

func (p *peer) leave() { p.goneSet.Do(func() { close(p.gone) }) }

// NewService builds the service; send delivers a control message, onExit
// is told when a viewer's shell ended (end its session then).
func NewService(shell ShellConfig, send func(peerID string, msg []byte) bool, onExit func(peerID string), log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		shell: shell, send: send, log: log, onExit: onExit, peers: make(map[string]*peer), allowed: make(map[string]models.Permissions),
		start: func(c ShellConfig, cols, rows uint16) (shellIO, error) { return StartShell(c, cols, rows) },
	}
}

// OnTyped is told whenever a viewer types (the idle timer).
func (t *Service) OnTyped(fn func(peerID string)) { t.typed = fn }

// Channel takes a viewer's terminal channel (StreamService.OnChannel),
// made before the viewer is let in: nothing reads it until Allow.
func (t *Service) Channel(peerID string, dc *webrtc.DataChannel) {
	if dc.Label() != Channel {
		return
	}
	p := &peer{dc: dc, cols: defaultCols, rows: defaultRows, drained: make(chan struct{}, 1), gone: make(chan struct{})}
	t.mu.Lock()
	if old := t.peers[peerID]; old != nil && old.sh != nil {
		old.sh.Close() // an earlier connection of the same viewer
	}
	t.peers[peerID] = p
	perm, ok := t.allowed[peerID]
	t.mu.Unlock()
	if ok {
		t.apply(peerID, p, perm)
	}
	dc.SetBufferedAmountLowThreshold(lowWater)
	dc.OnBufferedAmountLow(func() {
		select {
		case p.drained <- struct{}{}:
		default:
		}
	})
	dc.OnOpen(func() {
		t.mu.Lock()
		p.open = true
		pending := p.pending
		p.pending, p.queued = nil, 0
		t.mu.Unlock()
		for _, b := range pending {
			_ = dc.Send(b)
		}
	})
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if msg.IsString {
			return
		}
		t.mu.Lock()
		sh, ok := p.sh, p.control
		t.mu.Unlock()
		if sh != nil && ok {
			_, _ = sh.Write(msg.Data)
			if t.typed != nil {
				t.typed(peerID)
			}
		}
	})
	dc.OnClose(func() {
		p.leave()
		t.mu.Lock()
		if t.peers[peerID] == p && p.sh == nil {
			delete(t.peers, peerID) // never let in
		}
		t.mu.Unlock()
	})
}

// Allow follows the session's permissions (SessionService.OnPermissions):
// seeing starts the viewer's shell, Control lets it type, and nothing
// (the session ended) hangs the shell up. They may come before the
// viewer's channel does; the shell starts when both are there.
func (t *Service) Allow(peerID string, perm models.Permissions) {
	t.mu.Lock()
	if !perm.View {
		delete(t.allowed, peerID)
		p := t.peers[peerID]
		delete(t.peers, peerID)
		t.mu.Unlock()
		if p == nil {
			return
		}
		p.leave()
		t.mu.Lock()
		sh := p.sh
		t.mu.Unlock()
		if sh != nil {
			sh.Close()
			t.log.Info("terminal closed", "peer_id", peerID)
		}
		return
	}
	t.allowed[peerID] = perm
	p := t.peers[peerID]
	t.mu.Unlock()
	if p != nil {
		t.apply(peerID, p, perm)
	}
}

// apply gives a viewer's channel its permissions, starting its shell once.
func (t *Service) apply(peerID string, p *peer, perm models.Permissions) {
	t.mu.Lock()
	p.control = perm.Control
	if p.sh != nil || t.peers[peerID] != p {
		t.mu.Unlock()
		return
	}
	sh, err := t.start(t.shell, p.cols, p.rows)
	if err != nil {
		t.mu.Unlock()
		t.log.Error("cannot start the shell", "shell", t.shell.Path, "err", err)
		t.exited(peerID, -1)
		return
	}
	p.sh = sh
	t.mu.Unlock()
	t.log.Info("terminal opened", "peer_id", peerID, "shell", t.shell.Path, "user", t.shell.User, "may_type", perm.Control)
	go t.pump(peerID, p, sh)
}

// pump carries the shell's output to the viewer until the shell ends.
func (t *Service) pump(peerID string, p *peer, sh shellIO) {
	buf := make([]byte, readChunk)
	for {
		n, err := sh.Read(buf)
		if n > 0 {
			t.out(p, append([]byte(nil), buf[:n]...))
		}
		if err != nil {
			break
		}
	}
	<-sh.Done()
	t.mu.Lock()
	mine := t.peers[peerID] == p
	if mine {
		delete(t.peers, peerID)
		delete(t.allowed, peerID)
	}
	t.mu.Unlock()
	if mine { // the shell ended by itself (exit), not by Allow
		p.leave()
		sh.Close()
		t.exited(peerID, sh.ExitCode())
	}
}

// out sends b, or keeps it until the channel opens; a full channel makes
// the shell wait.
func (t *Service) out(p *peer, b []byte) {
	t.mu.Lock()
	if !p.open {
		p.pending = append(p.pending, b)
		p.queued += len(b)
		for p.queued > maxPending && len(p.pending) > 1 {
			p.queued -= len(p.pending[0])
			p.pending = p.pending[1:]
		}
		t.mu.Unlock()
		return
	}
	t.mu.Unlock()
	if p.dc.BufferedAmount() > highWater {
		select {
		case <-p.drained:
		case <-p.gone:
			return
		}
	}
	_ = p.dc.Send(b)
}

// exited tells the viewer its shell ended, and ends the session.
func (t *Service) exited(peerID string, code int) {
	t.log.Info("the shell ended", "peer_id", peerID, "code", code)
	msg, _ := json.Marshal(struct {
		Type string `json:"type"`
		Code int    `json:"code"`
	}{"terminal_exit", code})
	t.send(peerID, msg)
	if t.onExit != nil {
		t.onExit(peerID)
	}
}

// HandleSize takes the viewer's "terminal_size" {cols, rows}: the size of
// its terminal, at the start and on every change.
func (t *Service) HandleSize(peerID string, data []byte) {
	var m struct {
		Cols int `json:"cols"`
		Rows int `json:"rows"`
	}
	if json.Unmarshal(data, &m) != nil || m.Cols < 1 || m.Rows < 1 || m.Cols > maxSize || m.Rows > maxSize {
		return
	}
	t.mu.Lock()
	p := t.peers[peerID]
	if p == nil {
		t.mu.Unlock()
		return
	}
	p.cols, p.rows = uint16(m.Cols), uint16(m.Rows)
	sh := p.sh
	t.mu.Unlock()
	if sh != nil {
		_ = sh.Resize(uint16(m.Cols), uint16(m.Rows))
	}
}

// Active says whether a viewer has a shell open now.
func (t *Service) Active(peerID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	p := t.peers[peerID]
	return p != nil && p.sh != nil
}
