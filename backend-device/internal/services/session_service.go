// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/signalclient"
)

// SessionMeta is the room's public description on signalhub (room meta).
// It never carries anything private: signalhub shows it to whoever joins.
type SessionMeta struct {
	Name string `json:"name"` // the computer's name
	OS   string `json:"os"`   // darwin, windows or linux
}

// Viewers is what the session needs from the stream.
type Viewers interface {
	AddViewer(peerID string, codecs []string) error
	// AddWaiting connects a linked browser that gets nothing until Admit.
	AddWaiting(peerID string, codecs []string) error
	Admit(peerID string) bool
	HandleSignal(from string, payload json.RawMessage) error
	// Hangup sends a last control message and closes the connection.
	Hangup(peerID string, last []byte)
	// SendControl sends a message on the viewer's control channel.
	SendControl(peerID string, msg []byte) bool
	RemoveViewer(peerID string)
	CloseAll()
}

// ConsentTimeout is how long the person at the host has to answer.
const ConsentTimeout = 60 * time.Second

// maxSignalPayload bounds what the host reads from a guest's signal: an
// SDP answer is a few KiB.
const maxSignalPayload = 64 << 10

// guestState is where a guest of the session room is.
type guestState int

const (
	guestJoined   guestState = iota // in the room, has not knocked
	guestKnocking                   // the person at the host is being asked
	guestWaiting                    // a linked browser, connected but proving itself
	guestAccepted                   // in session: gets the stream
	guestDone                       // turned down or session over: ignored until it leaves
)

// door is how a guest came.
type door int

const (
	doorRoom door = iota // join with the session code or link
	doorCode             // claim with the pairing code: asks to be linked
	doorLink             // reach: a browser linked before, without anyone asking
)

type guest struct {
	state  guestState
	door   door
	knock  models.Knock
	cancel context.CancelFunc // stops the consent dialog while knocking
}

// UnattendedPolicy says whether linked browsers may come in.
type UnattendedPolicy interface {
	UnattendedOn() bool
}

// SessionService keeps the invite-only room that remote sessions use open
// on signalhub, and its code on screen, and runs the sessions: a viewer
// that joins knocks, the person at the host accepts or not, and only an
// accepted viewer gets a WebRTC offer. One session at a time.
//
// The room lives as long as the signaling connection: after a reconnect it
// is opened again and gets a new code. NewCode replaces the code on
// demand; the old one stops working at once (signalhub keeps one
// invitation per room). A session already on stays on.
//
// signalhub has no admission step and cannot remove a guest: a guest that
// is turned down stays in the room, ignored, until it leaves.
type SessionService struct {
	opener  *RoomOpener
	status  *StatusService
	meta    json.RawMessage
	log     *slog.Logger
	viewers Viewers

	mu      sync.Mutex
	sender  Sender
	consent Consent
	timeout time.Duration
	roomID  string
	gen     int // bumps on every (re)connect, so a late room_opened is dropped
	guests  map[string]*guest
	active  string // the viewer in session
	perm    models.Permissions
	onPerm  func(peer string, p models.Permissions)
	policy  UnattendedPolicy
}

var _ signalclient.Handler = (*SessionService)(nil)

// NewSessionService builds the service; wire the signaling client with
// SetSender. Until SetConsent, nobody can be asked: every knock is turned
// down.
func NewSessionService(opener *RoomOpener, status *StatusService, meta SessionMeta, viewers Viewers, log *slog.Logger) *SessionService {
	if log == nil {
		log = slog.Default()
	}
	raw, _ := json.Marshal(meta) // a struct of strings always marshals
	return &SessionService{
		opener: opener, status: status, meta: raw, log: log, viewers: viewers,
		consent: NobodyToAsk, timeout: ConsentTimeout, guests: make(map[string]*guest),
	}
}

// SetSender wires the signaling client.
func (s *SessionService) SetSender(snd Sender) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sender = snd
}

// SetConsent sets who decides on knocks: the window's dialog, or a test's.
func (s *SessionService) SetConsent(c Consent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c == nil {
		c = NobodyToAsk
	}
	s.consent = c
}

// SetUnattended sets who says whether linked browsers may come in
// (without it, they never do).
func (s *SessionService) SetUnattended(p UnattendedPolicy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.policy = p
}

func (s *SessionService) unattendedOn() bool {
	s.mu.Lock()
	p := s.policy
	s.mu.Unlock()
	return p != nil && p.UnattendedOn()
}

// AddLink takes a browser that came with the pairing code (byCode) or
// with reach: it knocks like a room guest. It implements Links.
func (s *SessionService) AddLink(peerID string, byCode bool) {
	d := doorLink
	if byCode {
		d = doorCode
	}
	s.mu.Lock()
	s.guests[peerID] = &guest{door: d}
	s.mu.Unlock()
}

// setTimeout changes the time to answer (tests).
func (s *SessionService) setTimeout(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.timeout = d
}

// OnConnect opens the session room. The pairing service registers first
// (it comes first in the dispatcher), and signalhub handles one
// connection's messages in order, so the room belongs to the registered
// owner.
func (s *SessionService) OnConnect(signalclient.Envelope) {
	s.mu.Lock()
	s.gen++
	gen := s.gen
	s.roomID = ""
	s.mu.Unlock()
	s.opener.Open(false, true, s.meta, func(roomID string, err error) {
		s.roomOpened(gen, roomID, err)
	})
}

func (s *SessionService) roomOpened(gen int, roomID string, err error) {
	if err != nil {
		s.log.Warn("cannot open the session room", "err", err)
		return
	}
	s.mu.Lock()
	if gen != s.gen {
		s.mu.Unlock()
		return
	}
	s.roomID = roomID
	s.mu.Unlock()
	s.status.SetSession(&models.SessionStatus{RoomID: roomID})
	s.log.Info("session room open", "room_id", roomID)
	s.NewCode()
}

// NewCode asks signalhub for a new invitation; the previous code and link
// stop working. It does nothing while the room is not open.
func (s *SessionService) NewCode() {
	s.mu.Lock()
	snd, roomID := s.sender, s.roomID
	s.mu.Unlock()
	if snd == nil || roomID == "" {
		return
	}
	if err := snd.Send(signalclient.Envelope{Type: signalclient.TypeInviteCreate, RoomID: roomID}); err != nil {
		s.log.Warn("cannot ask for a session code", "err", err)
	}
}

// OnMessage handles the room's invitation and its guests.
func (s *SessionService) OnMessage(env signalclient.Envelope) {
	switch env.Type {
	case signalclient.TypeInviteCreated:
		s.mu.Lock()
		roomID := s.roomID
		s.mu.Unlock()
		if roomID == "" || env.RoomID != roomID {
			return
		}
		s.status.SetSession(&models.SessionStatus{RoomID: roomID, Code: env.Code, Invite: env.Invite})
		s.log.Info("session code", "code", FormatCode(env.Code))
	case signalclient.TypePeerJoined:
		s.mu.Lock()
		mine := s.roomID != "" && env.RoomID == s.roomID
		if mine {
			s.guests[env.Remote] = &guest{}
		}
		s.mu.Unlock()
		if mine {
			s.log.Info("viewer joined the session room", "peer_id", env.Remote)
		}
	case signalclient.TypeSignal:
		s.onSignal(env.From, env.Payload)
	case signalclient.TypePeerLeft:
		s.onLeft(env.From)
	}
}

// onSignal reads a guest's signal: a knock while it waits at the door, the
// WebRTC answer and candidates once it is in, nothing otherwise.
func (s *SessionService) onSignal(from string, payload json.RawMessage) {
	if len(payload) > maxSignalPayload {
		return
	}
	var head struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(payload, &head) != nil {
		return
	}
	s.mu.Lock()
	g := s.guests[from]
	state, d := guestDone, doorRoom
	if g != nil {
		state, d = g.state, g.door
	}
	s.mu.Unlock()
	switch {
	case state == guestJoined && head.Kind == "knock":
		var k models.Knock
		if json.Unmarshal(payload, &k) != nil {
			return
		}
		switch {
		case d != doorRoom && !s.unattendedOn():
			s.turnDown(from, models.RejectOff)
		case d == doorLink:
			s.wait(from, k.Clean())
		default:
			s.knock(from, k.Clean())
		}
	case (state == guestAccepted || state == guestWaiting) && (head.Kind == "answer" || head.Kind == "candidate"):
		if err := s.viewers.HandleSignal(from, payload); err != nil {
			s.log.Warn("bad signal", "peer_id", from, "err", err)
		}
	default:
		// Not a guest of the room, or out of turn: a guest that was not
		// accepted gets nothing, whatever it sends.
		s.log.Debug("signal ignored", "peer_id", from, "kind", head.Kind)
	}
}

// turnDown says no to a guest that knocked; it is ignored afterwards.
func (s *SessionService) turnDown(peer, reason string) {
	s.mu.Lock()
	if g := s.guests[peer]; g != nil {
		g.state = guestDone
	}
	s.mu.Unlock()
	s.reject(peer, reason)
}

// wait connects a linked browser that knocked, gated: it must prove its
// link (and the password, if there is one) before Admit lets it in.
func (s *SessionService) wait(peer string, k models.Knock) {
	s.mu.Lock()
	g := s.guests[peer]
	if g == nil || s.active != "" {
		s.mu.Unlock()
		s.turnDown(peer, models.RejectBusy)
		return
	}
	g.state, g.knock = guestWaiting, k
	s.mu.Unlock()
	s.log.Info("linked browser knocks", "peer_id", peer, "browser", k.Browser)
	if err := s.viewers.AddWaiting(peer, k.Codecs); err != nil {
		s.log.Error("cannot connect the linked browser", "peer_id", peer, "err", err)
		s.turnDown(peer, models.RejectUnavailable)
	}
}

// Admit starts the session of a linked browser that proved itself, with
// its link's permissions and nobody asked. It fails if the browser is not
// waiting, or another session is on.
func (s *SessionService) Admit(peer string, perm models.Permissions) error {
	s.mu.Lock()
	g := s.guests[peer]
	switch {
	case g == nil || g.state != guestWaiting:
		s.mu.Unlock()
		return errors.New("session: not a waiting browser")
	case s.active != "":
		g.state = guestDone
		s.mu.Unlock()
		s.reject(peer, models.RejectBusy)
		s.viewers.RemoveViewer(peer)
		return errors.New("session: busy")
	}
	perm.View = true
	g.state = guestAccepted
	s.active = peer
	s.perm = perm
	k := g.knock
	s.mu.Unlock()
	s.signal(peer, struct {
		Kind        string             `json:"kind"`
		Permissions models.Permissions `json:"permissions"`
	}{"accepted", perm})
	s.status.SetActive(&models.ActiveSession{PeerID: peer, Name: k.Name, Browser: k.Browser, Since: time.Now(), Permissions: perm, Unattended: true})
	s.log.Info("unattended session started", "peer_id", peer, "name", k.Name, "control", perm.Control)
	s.permissionsChanged(peer, perm)
	s.viewers.Admit(peer)
	return nil
}

// Drop ends whatever a guest has: its session, or its gated connection.
func (s *SessionService) Drop(peer, reason string) {
	s.mu.Lock()
	active := s.active == peer
	g := s.guests[peer]
	waiting := g != nil && g.state == guestWaiting
	if waiting {
		g.state = guestDone
	}
	s.mu.Unlock()
	switch {
	case active:
		s.End(reason)
	case waiting:
		s.viewers.RemoveViewer(peer)
	}
}

// Active is the session on, if any.
func (s *SessionService) Active() (peer string, k models.Knock, perm models.Permissions) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == "" {
		return "", models.Knock{}, models.Permissions{}
	}
	if g := s.guests[s.active]; g != nil {
		k = g.knock
	}
	return s.active, k, s.perm
}

// knock asks the person at the host about a viewer, one at a time.
func (s *SessionService) knock(peer string, k models.Knock) {
	s.mu.Lock()
	g := s.guests[peer]
	busy := s.active != ""
	for id, other := range s.guests {
		if id != peer && other.state == guestKnocking {
			busy = true
		}
	}
	if g == nil || busy {
		if g != nil {
			g.state = guestDone
		}
		s.mu.Unlock()
		s.reject(peer, models.RejectBusy)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	g.state, g.cancel, g.knock = guestKnocking, cancel, k
	consent := s.consent
	link := g.door == doorCode
	s.mu.Unlock()
	s.log.Info("viewer knocks", "peer_id", peer, "name", k.Name, "browser", k.Browser, "codecs", k.Codecs, "link", link)
	go func() {
		defer cancel()
		perm, err := consent.Ask(ctx, ConsentRequest{PeerID: peer, Knock: k, Link: link})
		if err == nil && ctx.Err() != nil {
			err = ctx.Err() // an answer after the time ran out does not count
		}
		s.decided(peer, k, perm, err)
	}()
}

// decided applies the answer to a knock.
func (s *SessionService) decided(peer string, k models.Knock, perm models.Permissions, err error) {
	s.mu.Lock()
	g := s.guests[peer]
	if g == nil || g.state != guestKnocking {
		s.mu.Unlock() // it left while being asked
		return
	}
	g.cancel = nil
	if err != nil {
		g.state = guestDone
		s.mu.Unlock()
		reason := models.RejectDeclined
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			reason = models.RejectTimeout
		case errors.Is(err, ErrUnavailable):
			reason = models.RejectUnavailable
		}
		s.reject(peer, reason)
		return
	}
	perm.View = true // a session always shows the screen
	g.state = guestAccepted
	s.active = peer
	s.perm = perm
	s.mu.Unlock()

	s.signal(peer, struct {
		Kind        string             `json:"kind"`
		Permissions models.Permissions `json:"permissions"`
	}{"accepted", perm})
	s.status.SetActive(&models.ActiveSession{PeerID: peer, Name: k.Name, Browser: k.Browser, Since: time.Now(), Permissions: perm})
	s.log.Info("session started", "peer_id", peer, "name", k.Name, "control", perm.Control)
	s.permissionsChanged(peer, perm)
	if err := s.viewers.AddViewer(peer, k.Codecs); err != nil {
		s.log.Error("cannot start the stream", "peer_id", peer, "err", err)
		s.End(models.EndNetwork)
	}
}

func (s *SessionService) reject(peer, reason string) {
	s.log.Info("viewer turned down", "peer_id", peer, "reason", reason)
	s.signal(peer, struct {
		Kind   string `json:"kind"`
		Reason string `json:"reason"`
	}{"rejected", reason})
}

// signal sends a payload to one guest.
func (s *SessionService) signal(to string, payload any) {
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	s.mu.Lock()
	snd := s.sender
	s.mu.Unlock()
	if snd == nil {
		return
	}
	if err := snd.Send(signalclient.Envelope{Type: signalclient.TypeSignal, To: to, Payload: b}); err != nil {
		s.log.Warn("signal not sent", "to", to, "err", err)
	}
}

// onLeft forgets a guest that left signalhub's session: its question
// closes, or its session ends.
func (s *SessionService) onLeft(peer string) {
	s.mu.Lock()
	g := s.guests[peer]
	delete(s.guests, peer)
	wasActive := s.active == peer
	if wasActive {
		s.active = ""
	}
	s.mu.Unlock()
	if g == nil {
		return
	}
	if g.cancel != nil {
		g.cancel()
	}
	if g.state == guestWaiting {
		s.viewers.RemoveViewer(peer)
	}
	if wasActive {
		s.viewers.RemoveViewer(peer)
		s.ended(peer, models.EndViewer)
	}
}

// End stops the session on, if any: the viewer is told why and its
// connection closes. The viewer stays in the room, ignored, until it
// leaves.
func (s *SessionService) End(reason string) {
	s.mu.Lock()
	peer := s.active
	s.active = ""
	if g := s.guests[peer]; g != nil {
		g.state = guestDone
	}
	s.mu.Unlock()
	if peer == "" {
		return
	}
	last, _ := json.Marshal(struct {
		Type   string `json:"type"`
		Reason string `json:"reason"`
	}{"session_end", reason})
	s.viewers.Hangup(peer, last)
	s.ended(peer, reason)
}

// ViewerHungUp ends the session of a viewer that said it is leaving
// (session_end on its control channel).
func (s *SessionService) ViewerHungUp(peer string) {
	if !s.release(peer) {
		return
	}
	s.viewers.RemoveViewer(peer)
	s.ended(peer, models.EndViewer)
}

// StreamClosed ends the session of a viewer whose connection the stream
// gave up on.
func (s *SessionService) StreamClosed(peer, reason string) {
	if s.release(peer) {
		s.ended(peer, reason)
	}
}

// release takes peer out of session; false when it was not in one.
func (s *SessionService) release(peer string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if peer == "" || s.active != peer {
		return false
	}
	s.active = ""
	if g := s.guests[peer]; g != nil {
		g.state = guestDone
	}
	return true
}

func (s *SessionService) ended(peer, reason string) {
	s.permissionsChanged(peer, models.Permissions{})
	s.status.SetActive(nil)
	s.log.Info("session ended", "peer_id", peer, "reason", reason)
}

// OnDisconnect forgets the room, its guests and the session: they all
// went with the connection.
func (s *SessionService) OnDisconnect(error) {
	s.mu.Lock()
	s.roomID = ""
	active := s.active
	s.active = ""
	for _, g := range s.guests {
		if g.cancel != nil {
			g.cancel()
		}
	}
	s.guests = make(map[string]*guest)
	s.mu.Unlock()
	s.viewers.CloseAll()
	if active != "" {
		s.ended(active, models.EndSignal)
	}
	s.status.SetSession(nil)
}

// OnPermissions is told what the viewer in session may do: when the
// session starts, when the person at the host changes it, and (nothing)
// when it ends.
func (s *SessionService) OnPermissions(fn func(peer string, p models.Permissions)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onPerm = fn
}

func (s *SessionService) permissionsChanged(peer string, p models.Permissions) {
	s.mu.Lock()
	fn := s.onPerm
	s.mu.Unlock()
	if fn != nil {
		fn(peer, p)
	}
}

// SetPermissions changes what the viewer in session may do, from the
// session panel; the viewer is told at once (control "permissions").
func (s *SessionService) SetPermissions(p models.Permissions) {
	p.View = true
	s.mu.Lock()
	peer := s.active
	if peer == "" || p == s.perm {
		s.mu.Unlock()
		return
	}
	s.perm = p
	s.mu.Unlock()
	msg, _ := json.Marshal(struct {
		Type        string             `json:"type"`
		Permissions models.Permissions `json:"permissions"`
	}{"permissions", p})
	s.viewers.SendControl(peer, msg)
	if a := s.status.Snapshot().Session; a != nil && a.Active != nil && a.Active.PeerID == peer {
		next := *a.Active
		next.Permissions = p
		s.status.SetActive(&next)
	}
	s.log.Info("permissions changed", "peer_id", peer, "control", p.Control)
	s.permissionsChanged(peer, p)
}

// FormatCode groups a 9-digit code as people read it: "915 355 636".
// Anything else is returned unchanged.
func FormatCode(code string) string {
	if len(code) != 9 {
		return code
	}
	return code[:3] + " " + code[3:6] + " " + code[6:]
}
