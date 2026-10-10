// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/signalclient"
)

// SessionMeta is the room's public description on signalhub (room meta).
// It never carries anything private: signalhub shows it to whoever joins.
type SessionMeta struct {
	Name string `json:"name"` // the computer's name
	OS   string `json:"os"`   // darwin, windows or linux
}

// SessionService keeps the invite-only room that remote sessions use open
// on signalhub, and its code on screen. The room lives as long as the
// signaling connection: after a reconnect it is opened again and gets a new
// code. NewCode replaces the code on demand; the old one stops working at
// once (signalhub keeps one invitation per room).
//
// Viewers that join are handled in later phases (knock, consent, WebRTC);
// for now they are only logged.
type SessionService struct {
	opener *RoomOpener
	status *StatusService
	meta   json.RawMessage
	log    *slog.Logger

	mu     sync.Mutex
	sender Sender
	roomID string
	gen    int // bumps on every (re)connect, so a late room_opened is dropped
}

var _ signalclient.Handler = (*SessionService)(nil)

// NewSessionService builds the service; wire the signaling client with
// SetSender.
func NewSessionService(opener *RoomOpener, status *StatusService, meta SessionMeta, log *slog.Logger) *SessionService {
	if log == nil {
		log = slog.Default()
	}
	raw, _ := json.Marshal(meta) // a struct of strings always marshals
	return &SessionService{opener: opener, status: status, meta: raw, log: log}
}

// SetSender wires the signaling client.
func (s *SessionService) SetSender(snd Sender) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sender = snd
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

// OnMessage records the invitation and notes viewers coming and going.
func (s *SessionService) OnMessage(env signalclient.Envelope) {
	s.mu.Lock()
	roomID := s.roomID
	s.mu.Unlock()
	if roomID == "" || env.RoomID != roomID {
		return
	}
	switch env.Type {
	case signalclient.TypeInviteCreated:
		s.status.SetSession(&models.SessionStatus{RoomID: roomID, Code: env.Code, Invite: env.Invite})
		s.log.Info("session code", "code", FormatCode(env.Code))
	case signalclient.TypePeerJoined:
		s.log.Info("viewer joined the session room", "peer_id", env.Remote)
	}
}

// OnDisconnect forgets the room: it closed with the connection.
func (s *SessionService) OnDisconnect(error) {
	s.mu.Lock()
	s.roomID = ""
	s.mu.Unlock()
	s.status.SetSession(nil)
}

// FormatCode groups a 9-digit code as people read it: "915 355 636".
// Anything else is returned unchanged.
func FormatCode(code string) string {
	if len(code) != 9 {
		return code
	}
	return code[:3] + " " + code[3:6] + " " + code[6:]
}
