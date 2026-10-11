// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package viewer is go-link Desk's own viewer: the app connects to another
// computer the way the website does, with the same protocol
// (docs/protocol.md). The host on the other side cannot tell them apart.
//
// A Session is one visit: it joins the computer's room with its code,
// links with its pairing code, or comes back to a linked computer; knocks;
// answers the host's offer; and from then on hands the picture, the
// sound, the pointer and the control messages to the window through
// Events, and the window's input, chat, clipboard and files to the host.
package viewer

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/signalclient"
)

// DoorKind is how a session reaches its computer.
type DoorKind int

const (
	// DoorJoin: the session code (9 digits) or invitation the person at
	// the computer shares; they accept the session.
	DoorJoin DoorKind = iota
	// DoorClaim: the computer's pairing code (Settings › Unattended
	// access); the person there accepts and this app is linked.
	DoorClaim
	// DoorReach: a computer linked before; nobody accepts.
	DoorReach
)

// Door is where a session goes.
type Door struct {
	Kind   DoorKind
	Code   string               // join, claim: 9 digits, without spaces
	Invite string               // join: the long invitation instead of the code
	Device *models.RemoteDevice // reach
}

// StatusKind is where a session is, from the knock to its end.
type StatusKind string

const (
	StatusJoining      StatusKind = "joining"
	StatusWaiting      StatusKind = "waiting"  // the person at the computer decides
	StatusPassword     StatusKind = "password" // a linked computer asks its unattended password
	StatusConnecting   StatusKind = "connecting"
	StatusLive         StatusKind = "live"
	StatusReconnecting StatusKind = "reconnecting"
	// The session is over:
	StatusRejected        StatusKind = "rejected"         // Reason: declined, timeout, busy, unavailable, off
	StatusEnded           StatusKind = "ended"            // Reason: host, viewer, network, signal, idle, revoked
	StatusNotFound        StatusKind = "not_found"        // no such code
	StatusRateLimited     StatusKind = "rate_limited"     // too many tries
	StatusUnreachable     StatusKind = "unreachable"      // signalhub did not answer
	StatusUnlinked        StatusKind = "unlinked"         // the computer no longer has this link
	StatusOffline         StatusKind = "offline"          // the linked computer is not on
	StatusPasswordRefused StatusKind = "password_refused" // too many wrong passwords
	StatusUntrusted       StatusKind = "untrusted"        // whoever answered could not prove it is the linked computer
	StatusSelf            StatusKind = "self"             // the code is this computer's own
)

// Status is a session's state.
type Status struct {
	Kind StatusKind
	// Reason: why it was rejected or ended.
	Reason string
	// Password: the last try's error (wrong, blocked, locked), tries left,
	// and whether the computer is checking one.
	PasswordError string
	Left          int
	Checking      bool
	// RetryAfter: password refused while locked, in seconds.
	RetryAfter int
}

// Over reports whether the session can no longer change.
func (s Status) Over() bool {
	switch s.Kind {
	case StatusJoining, StatusWaiting, StatusPassword, StatusConnecting, StatusLive, StatusReconnecting:
		return false
	}
	return true
}

// HostInfo is the computer as its welcome describes it.
type HostInfo struct {
	Name string
	OS   string
}

// Display is one of the computer's screens.
type Display struct {
	ID           int
	Name         string
	Width        int
	Height       int
	PointsWidth  int
	PointsHeight int
	Primary      bool
}

// Sent is what the computer says it sends (stream_stats).
type Sent struct {
	FPS          float64
	Width        int
	Height       int
	Kbps         int
	Codec        string
	TargetKbps   int
	EstimateKbps int
}

// CursorShape is the computer's pointer image. Width and Height are its
// size on screen in the computer's points; HotX and HotY its hot spot in
// the image's pixels.
type CursorShape struct {
	ID     string
	PNG    []byte
	HotX   float64
	HotY   float64
	Width  float64
	Height float64
	Pixels float64
}

// ChatLine is one line of the session's chat.
type ChatLine struct {
	FromHost bool
	Text     string
	At       time.Time
}

// NewLink is a link the computer made for this app (claim): keep it to
// come back without a code. Token is this app's secret.
type NewLink struct {
	DeviceID string
	LinkID   string
	Token    string
}

// Frame is one video frame as it came: an H.264 access unit in Annex B,
// or a VP8 frame.
type Frame struct {
	Codec    string // "h264" or "vp8"
	Data     []byte
	Keyframe bool
}

// Events are what the window learns. Every field is optional; they are
// called from the session's goroutines, never two of the same at once
// except Video and Audio, which have their own.
type Events struct {
	Status      func(Status)
	Host        func(HostInfo)
	Permissions func(models.Permissions)
	Displays    func(displays []Display, current int)
	Sent        func(Sent)
	Notice      func(code string)
	Video       func(Frame)
	// Audio is the computer's sound: 16-bit mono at 48 kHz.
	Audio     func(samples []int16)
	Cursor    func(CursorShape)
	CursorPos func(x, y float64, visible bool)
	// Clipboard: something new was copied on the computer.
	Clipboard func(text string)
	Chat      func(ChatLine)
	// Typing: the person at the computer is writing a message (on), or stopped.
	Typing func(on bool)
	Linked func(NewLink)
	// Unlinked: the computer no longer has this link: forget it.
	Unlinked func()
	// Files: the files channel is open (with the files permission).
	Files func(*Files)
}

// Options configure a Session.
type Options struct {
	Door Door
	// Name and Agent are what the person at the computer sees ("Ana",
	// "go-link Desk 1.2 on macOS").
	Name  string
	Agent string
	// Codecs this side decodes: "h264-high", "h264", "vp8".
	Codecs []string
	// SignalURL is the signaling server (default models.DefaultSignalURL).
	SignalURL string
	// OwnDeviceID is this computer's: a door to it is refused (StatusSelf).
	OwnDeviceID string
	Events      Events
	Logger      *slog.Logger
}

// Transport carries the session's signaling: one signalhub connection of
// its own, as a browser tab has (leaving signalhub is how a viewer leaves
// the room). Tests replace it.
type Transport interface {
	Send(env signalclient.Envelope) error
	Close()
}

// Session is one visit to a computer.
type Session struct {
	opts Options
	log  *slog.Logger

	mu        sync.Mutex
	status    Status
	transport Transport
	ice       []signalclient.ICEServer
	host      string // the computer's peer id
	asked     bool   // the door's request went out
	peer      *peer
	perms     models.Permissions
	hasPerms  bool
	backlog   []json.RawMessage // the host's signals before the peer exists
	nonce     string            // reach: what the computer must prove
	proven    bool              // reach: the proof checked out
	authed    bool              // reach: the computer took the token (auth_ok)
	early     *models.Permissions
	connected bool // the peer connection is up
	clipSeq   uint32
	clipIn    clipboardAssembler
	shapes    map[string]CursorShape
	cancel    context.CancelFunc
	// pendingControl waits for the peer and its control channel.
	pendingControl [][]byte
	isHidden       bool
}

// New builds a session; Start (or Dial) runs it.
func New(opts Options) *Session {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.SignalURL == "" {
		opts.SignalURL = models.DefaultSignalURL
	}
	if len(opts.Codecs) == 0 {
		opts.Codecs = []string{"vp8"}
	}
	return &Session{opts: opts, log: opts.Logger, status: Status{Kind: StatusJoining}, shapes: map[string]CursorShape{}}
}

// Status returns where the session is.
func (s *Session) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Permissions returns what the computer lets this viewer do now.
func (s *Session) Permissions() models.Permissions {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.perms
}

// Dial connects to signalhub with a connection of the session's own and
// runs the session until it ends or ctx does. It returns at once.
func (s *Session) Dial(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	h := &dialHandler{s: s}
	c := signalclient.New(signalclient.Config{URL: s.opts.SignalURL, Logger: s.log}, h)
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()
	s.Start(&clientTransport{c: c, cancel: cancel})
	go func() {
		_ = c.Run(ctx)
	}()
	// signalhub must answer in time.
	go func() {
		select {
		case <-ctx.Done():
		case <-time.After(20 * time.Second):
			if s.Status().Kind == StatusJoining && !h.hello() {
				s.finish(Status{Kind: StatusUnreachable})
			}
		}
	}()
}

// Start uses t for signaling; Connected and Handle feed it.
func (s *Session) Start(t Transport) {
	s.mu.Lock()
	s.transport = t
	s.mu.Unlock()
	s.emitStatus()
}

// clientTransport is a signalclient.Client used once: a dropped connection
// is not tried again (the room membership went with it).
type clientTransport struct {
	c      *signalclient.Client
	cancel context.CancelFunc
}

func (t *clientTransport) Send(env signalclient.Envelope) error { return t.c.Send(env) }
func (t *clientTransport) Close()                               { t.cancel() }

type dialHandler struct {
	s  *Session
	mu sync.Mutex
	ok bool
}

func (h *dialHandler) hello() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.ok
}

func (h *dialHandler) OnConnect(hello signalclient.Envelope) {
	h.mu.Lock()
	h.ok = true
	h.mu.Unlock()
	h.s.Connected(hello.ICEServers)
}

func (h *dialHandler) OnMessage(env signalclient.Envelope) { h.s.Handle(env) }

func (h *dialHandler) OnDisconnect(error) {
	// Before the room: signalhub could not be reached; after it: the
	// computer is gone with it.
	if h.s.Status().Kind == StatusJoining && !h.hello() {
		h.s.finish(Status{Kind: StatusUnreachable})
		return
	}
	h.s.finish(Status{Kind: StatusEnded, Reason: models.EndSignal})
}

// Connected runs when signalhub says hello: ask through the door.
func (s *Session) Connected(ice []signalclient.ICEServer) {
	s.mu.Lock()
	s.ice = ice
	if s.asked || s.status.Over() {
		s.mu.Unlock()
		return
	}
	s.asked = true
	d := s.opts.Door
	env := signalclient.Envelope{App: models.App}
	switch d.Kind {
	case DoorJoin:
		env.Type = signalclient.TypeJoin
		env.Code, env.Invite = d.Code, d.Invite
	case DoorClaim:
		env.Type = signalclient.TypeClaim
		env.Code = d.Code
	case DoorReach:
		env.Type = signalclient.TypeReach
		if d.Device != nil {
			env.DeviceID = d.Device.DeviceID
		}
	}
	t := s.transport
	s.mu.Unlock()
	if d.Kind == DoorReach && (d.Device == nil || d.Device.DeviceID == s.opts.OwnDeviceID) {
		s.finish(Status{Kind: StatusSelf})
		return
	}
	if err := t.Send(env); err != nil {
		s.finish(Status{Kind: StatusUnreachable})
	}
}

// Handle takes one message from signalhub.
func (s *Session) Handle(env signalclient.Envelope) {
	switch env.Type {
	case signalclient.TypeJoined, signalclient.TypePaired:
		s.inRoom(env.Remote)
	case signalclient.TypeError:
		s.mu.Lock()
		joining := s.status.Kind == StatusJoining
		s.mu.Unlock()
		if !joining {
			s.log.Warn("signal error", "error", env.Error)
			return
		}
		switch {
		case env.Error == "rate limit exceeded":
			s.finish(Status{Kind: StatusRateLimited})
		case env.Error == "device not connected":
			s.finish(Status{Kind: StatusOffline})
		default:
			s.finish(Status{Kind: StatusNotFound})
		}
	case signalclient.TypeSignal:
		s.mu.Lock()
		from := s.host
		s.mu.Unlock()
		if from == "" || env.From != from {
			return
		}
		s.onSignal(env.Payload)
	case signalclient.TypePeerLeft:
		s.mu.Lock()
		host := s.host
		s.mu.Unlock()
		if host != "" && env.From == host {
			s.finish(Status{Kind: StatusEnded, Reason: models.EndSignal})
		}
	}
}

// inRoom: signalhub put this viewer with the computer. Knock.
func (s *Session) inRoom(host string) {
	s.mu.Lock()
	if s.host != "" || s.status.Over() || host == "" {
		s.mu.Unlock()
		return
	}
	s.host = host
	s.status = Status{Kind: StatusWaiting}
	reach := s.opts.Door.Kind == DoorReach
	s.mu.Unlock()
	s.emitStatus()
	knock, _ := json.Marshal(models.Knock{
		Name:    truncate(strings.TrimSpace(s.opts.Name), models.MaxKnockName),
		Browser: truncate(s.opts.Agent, models.MaxKnockBrowser),
		Codecs:  s.opts.Codecs,
	})
	// The knock's kind goes in with its fields.
	var m map[string]any
	_ = json.Unmarshal(knock, &m)
	m["kind"] = "knock"
	s.sendSignal(m)
	// A linked computer proves itself first: the token goes only to the
	// computer it was linked to (sent when the control channel opens).
	if reach {
		nonce := randomNonce()
		s.mu.Lock()
		s.nonce = nonce
		s.mu.Unlock()
		s.sendControl(map[string]any{"type": "auth_challenge", "link_id": s.opts.Door.Device.LinkID, "nonce": nonce})
	}
}

// onSignal takes the computer's decision, offer or candidate.
func (s *Session) onSignal(payload json.RawMessage) {
	var head struct {
		Kind        string             `json:"kind"`
		Permissions models.Permissions `json:"permissions"`
		Reason      string             `json:"reason"`
	}
	if json.Unmarshal(payload, &head) != nil {
		return
	}
	switch head.Kind {
	case "accepted":
		s.mu.Lock()
		// A linked computer lets this app in only once it proved itself
		// and took the token; its acceptance may overtake auth_ok (they
		// travel different ways), so it waits for it.
		if s.opts.Door.Kind == DoorReach && !s.authed {
			p := head.Permissions
			s.early = &p
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
		s.admitted(head.Permissions)
	case "rejected":
		s.finish(Status{Kind: StatusRejected, Reason: rejectReason(head.Reason)})
	case "offer", "candidate":
		s.mu.Lock()
		p := s.peer
		if p == nil && head.Kind == "offer" {
			var err error
			p, err = newPeer(s, s.ice)
			if err != nil {
				s.mu.Unlock()
				s.log.Error("viewer peer", "err", err)
				s.finish(Status{Kind: StatusEnded, Reason: models.EndNetwork})
				return
			}
			s.peer = p
		}
		if p == nil {
			if len(s.backlog) < 50 {
				s.backlog = append(s.backlog, payload)
			}
			s.mu.Unlock()
			return
		}
		backlog := s.backlog
		s.backlog = nil
		s.mu.Unlock()
		p.signal(payload)
		for _, b := range backlog {
			p.signal(b)
		}
	}
}

// admitted: the computer let this viewer in.
func (s *Session) admitted(p models.Permissions) {
	s.mu.Lock()
	if s.status.Over() {
		s.mu.Unlock()
		return
	}
	p.View = true
	s.perms, s.hasPerms = p, true
	if s.connected {
		s.status = Status{Kind: StatusLive}
	} else {
		s.status = Status{Kind: StatusConnecting}
	}
	claim := s.opts.Door.Kind == DoorClaim
	s.mu.Unlock()
	s.emitStatus()
	if fn := s.opts.Events.Permissions; fn != nil {
		fn(p)
	}
	// Accepted with the pairing code: now ask for the link's token.
	if claim {
		s.sendControl(map[string]any{"type": "auth"})
	}
}

// connState follows the peer connection.
func (s *Session) connState(state string) {
	s.mu.Lock()
	s.connected = state == "connected"
	kind := s.status.Kind
	if s.status.Over() {
		s.mu.Unlock()
		return
	}
	if kind == StatusWaiting || kind == StatusPassword || kind == StatusJoining {
		s.mu.Unlock()
		if state == "failed" {
			s.finish(Status{Kind: StatusEnded, Reason: models.EndNetwork})
		}
		return
	}
	switch state {
	case "connected":
		s.status = Status{Kind: StatusLive}
	case "reconnecting":
		s.status = Status{Kind: StatusReconnecting}
	case "failed":
		s.mu.Unlock()
		s.finish(Status{Kind: StatusEnded, Reason: models.EndNetwork})
		return
	default:
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	s.emitStatus()
}

// HangUp leaves: the computer is told, and the session ends.
func (s *Session) HangUp() {
	if s.Status().Over() {
		return
	}
	s.sendControl(map[string]any{"type": "session_end"})
	s.finish(Status{Kind: StatusEnded, Reason: models.EndViewer})
}

// finish ends the visit: the connection closes and the viewer leaves the
// room (its signalhub connection closes).
func (s *Session) finish(st Status) {
	s.mu.Lock()
	if s.status.Over() {
		s.mu.Unlock()
		return
	}
	s.status = st
	p, t := s.peer, s.transport
	s.mu.Unlock()
	if p != nil {
		p.close()
	}
	if t != nil {
		t.Close()
	}
	s.emitStatus()
}

func (s *Session) emitStatus() {
	if fn := s.opts.Events.Status; fn != nil {
		fn(s.Status())
	}
}

func (s *Session) sendSignal(v any) {
	s.mu.Lock()
	t, host := s.transport, s.host
	s.mu.Unlock()
	if t == nil || host == "" {
		return
	}
	payload, err := json.Marshal(v)
	if err != nil {
		return
	}
	_ = t.Send(signalclient.Envelope{Type: signalclient.TypeSignal, To: host, Payload: payload})
}

// sendControl sends a message on the control channel, or keeps it (up to
// 20) until the channel opens.
func (s *Session) sendControl(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	s.mu.Lock()
	p := s.peer
	if p == nil {
		if len(s.pendingControl) < 20 {
			s.pendingControl = append(s.pendingControl, b)
		}
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	p.sendControl(b)
}

// takePending hands the messages sent before the peer existed to it.
func (s *Session) takePending() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.pendingControl
	s.pendingControl = nil
	return out
}

var errClosed = errors.New("viewer: session over")

func rejectReason(r string) string {
	switch r {
	case models.RejectDeclined, models.RejectTimeout, models.RejectBusy, models.RejectUnavailable, models.RejectOff:
		return r
	}
	return models.RejectDeclined
}

func endReason(r string) string {
	switch r {
	case models.EndHost, models.EndViewer, models.EndNetwork, models.EndSignal, models.EndIdle, models.EndRevoked:
		return r
	}
	return models.EndHost
}

// truncate keeps at most n characters.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
