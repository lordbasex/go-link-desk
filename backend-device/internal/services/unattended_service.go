// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
)

// PasswordTimeout is how long a linked browser has to type the password.
const PasswordTimeout = 2 * time.Minute

// Messages of the password step, on the control channel.
const (
	msgPasswordRequired = "password_required"
	msgPassword         = "password"
	msgPasswordResult   = "password_result"
)

// UnattendedConfig wires UnattendedService.
type UnattendedConfig struct {
	Links    *LinkService
	Session  *SessionService
	Settings *SettingsService
	// Send writes on a browser's control channel (gated or not).
	Send   func(peerID string, msg []byte) bool
	Gate   *PasswordGate // nil: a new one
	Logger *slog.Logger
}

// UnattendedService lets linked browsers in without anyone accepting: a
// browser that came back with reach is connected gated, proves the
// device first (auth_challenge), then shows its token (auth), then types
// the password if one is set, and only then does the session start, with
// its link's permissions. A browser that came with the pairing code was
// accepted by the person at the host; it gets its token, and the
// permissions accepted become its link's.
type UnattendedService struct {
	cfg  UnattendedConfig
	gate *PasswordGate

	mu       sync.Mutex
	awaiting map[string]*awaiting // browsers asked for the password
}

type awaiting struct {
	link  models.Link
	timer *time.Timer
}

// NewUnattendedService builds the service and hooks it to the links.
func NewUnattendedService(cfg UnattendedConfig) *UnattendedService {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	gate := cfg.Gate
	if gate == nil {
		gate = NewPasswordGate(nil)
	}
	u := &UnattendedService{cfg: cfg, gate: gate, awaiting: map[string]*awaiting{}}
	cfg.Links.OnAuthenticated(u.authenticated)
	return u
}

// HandleWaiting reads a gated browser's control messages: the link's
// proofs, and the password.
func (u *UnattendedService) HandleWaiting(peerID string, data []byte) {
	var head struct {
		Type     string `json:"type"`
		Password string `json:"password"`
	}
	if json.Unmarshal(data, &head) != nil {
		return
	}
	if head.Type == msgPassword {
		u.password(peerID, head.Password)
		return
	}
	u.cfg.Links.HandleMessage(peerID, data)
}

// HandleControl reads the link messages of a browser in session: one that
// came with the pairing code asks for its token (auth), and a linked one
// may unlink itself. Other viewers' are ignored.
func (u *UnattendedService) HandleControl(peerID string, data []byte) {
	if u.cfg.Links.Trusted(peerID) {
		u.cfg.Links.HandleMessage(peerID, data)
	}
}

// authenticated runs when a browser proved its link, or got one.
func (u *UnattendedService) authenticated(peerID string, link models.Link, created bool) {
	if created {
		// The person at the host accepted it a moment ago: its link keeps
		// what they allowed, and its browser's name.
		peer, k, perm := u.cfg.Session.Active()
		if peer != peerID {
			perm = models.ViewOnly
		}
		u.cfg.Links.SetLink(link.ID, func(l *models.Link) {
			l.Name = k.Browser
			l.Permissions = perm
		})
		return
	}
	if !u.cfg.Settings.UnattendedOn() {
		u.cfg.Session.Drop(peerID, models.EndRevoked)
		return
	}
	if u.cfg.Settings.Get().Unattended.PasswordHash == "" {
		u.admit(peerID, link)
		return
	}
	u.mu.Lock()
	if old := u.awaiting[peerID]; old != nil {
		old.timer.Stop()
	}
	u.awaiting[peerID] = &awaiting{link: link, timer: time.AfterFunc(PasswordTimeout, func() {
		u.cfg.Logger.Warn("linked browser did not type the password in time", "peer_id", peerID)
		u.forget(peerID)
		u.cfg.Session.Drop(peerID, models.EndRevoked)
	})}
	u.mu.Unlock()
	u.reply(peerID, map[string]string{"type": msgPasswordRequired})
}

// password checks the password of a browser that was asked for it.
func (u *UnattendedService) password(peerID, password string) {
	u.mu.Lock()
	a := u.awaiting[peerID]
	u.mu.Unlock()
	if a == nil {
		return // not asked: a browser that has not proved its link yet
	}
	hash := u.cfg.Settings.Get().Unattended.PasswordHash
	r := u.gate.Check(peerID, func() bool { return hash == "" || CheckPassword(hash, password) })
	u.reply(peerID, struct {
		Type string `json:"type"`
		PinResult
	}{msgPasswordResult, r})
	switch {
	case r.OK:
		u.forget(peerID)
		u.admit(peerID, a.link)
	case r.Reason == "blocked" || r.Reason == "locked":
		u.cfg.Logger.Warn("linked browser turned away: wrong password", "peer_id", peerID, "reason", r.Reason)
		u.forget(peerID)
		time.AfterFunc(500*time.Millisecond, func() { u.cfg.Session.Drop(peerID, models.EndRevoked) })
	default:
		u.cfg.Logger.Warn("wrong unattended password", "peer_id", peerID, "left", r.Left)
	}
}

func (u *UnattendedService) admit(peerID string, link models.Link) {
	if err := u.cfg.Session.Admit(peerID, link.Permissions); err != nil {
		u.cfg.Logger.Info("linked browser not admitted", "peer_id", peerID, "err", err)
	}
}

// PairedByCode, Reached and Forget make the service the pairing's
// LinkAuth: they pass on to the links, and Forget also drops what is kept
// about a browser that left.
func (u *UnattendedService) PairedByCode(peerID string) { u.cfg.Links.PairedByCode(peerID) }

// Reached starts the clock for a browser that came back with reach.
func (u *UnattendedService) Reached(peerID string) { u.cfg.Links.Reached(peerID) }

// Forget drops what is kept about a browser that left.
func (u *UnattendedService) Forget(peerID string) {
	u.cfg.Links.Forget(peerID)
	u.forget(peerID)
	u.gate.Forget(peerID)
}

var _ LinkAuth = (*UnattendedService)(nil)

func (u *UnattendedService) forget(peerID string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if a := u.awaiting[peerID]; a != nil {
		a.timer.Stop()
		delete(u.awaiting, peerID)
	}
}

func (u *UnattendedService) reply(peerID string, msg any) {
	if b, err := json.Marshal(msg); err == nil && u.cfg.Send != nil {
		u.cfg.Send(peerID, b)
	}
}
