// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"math/big"
	"slices"
	"sync"
	"time"
)

// A private room asks every guest for a PIN before the device streams to
// them. The device enforces it (not the web, not signalhub), and limits the
// guesses so a 6 digit PIN cannot be found by trying.
//
// Every invitation carries its own PIN, good for one person: the first
// browser that uses it gets in and receives a token to come back (a reload,
// a dropped connection) without a PIN; anyone else trying the same PIN is
// told it was used. A PIN nobody used expires after passTTL, and every PIN
// and token dies with the room's session (the gate is per session).
const (
	PinLength       = 6
	pinTriesPerPeer = 5                // wrong PINs before a guest is turned away
	pinRoomFails    = 20               // wrong PINs in pinWindow before the room locks
	pinWindow       = 10 * time.Minute // how far back wrong PINs count
	pinLock         = 10 * time.Minute // how long a locked room refuses every PIN
	passTTL         = 6 * time.Hour    // an invitation nobody used expires
	maxPasses       = 64               // invitations alive at once in a room
)

// NewPIN returns a random 6 digit PIN.
func NewPIN() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		panic(err) // crypto/rand never fails on supported systems
	}
	return fmt.Sprintf("%06d", n.Int64())
}

// ValidPIN reports whether s is 6 digits.
func ValidPIN(s string) bool {
	if len(s) != PinLength {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// newToken returns a random 32 byte token, base64url.
func newToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// PinResult is the device's answer to a PIN, sent back to the guest.
type PinResult struct {
	OK bool `json:"ok"`
	// Token lets this browser come back into the room without a PIN.
	Token string `json:"token,omitempty"`
	// Reason when not OK: "wrong", "used" (someone already came in with
	// this invitation), "blocked" (this guest used its tries) or "locked"
	// (too many wrong PINs in the room: wait RetryAfter seconds).
	Reason     string `json:"reason,omitempty"`
	Left       int    `json:"left,omitempty"`
	RetryAfter int    `json:"retry_after,omitempty"`
	// Owner: the guest came in with the host's own key (never sent).
	Owner bool `json:"-"`
}

// Pass is one invitation's PIN, for the host to share.
type Pass struct {
	Pin       string    `json:"pin"`
	ExpiresAt time.Time `json:"expires_at"`
}

type pass struct {
	pin     string
	expires time.Time
	token   string // set once someone came in with it
}

// PinGate checks the PINs of one room session.
type PinGate struct {
	now func() time.Time

	mu          sync.Mutex
	private     bool
	owner       string // the host's own key (its browsers never need a PIN)
	passes      []*pass
	tries       map[string]int
	fails       []time.Time
	lockedUntil time.Time
}

// NewPinGate returns an open gate (no PIN asked). now may be nil (time.Now).
func NewPinGate(now func() time.Time) *PinGate {
	if now == nil {
		now = time.Now
	}
	return &PinGate{now: now, tries: map[string]int{}, owner: newToken()}
}

// SetPrivate makes guests need an invitation's PIN (or a token).
func (g *PinGate) SetPrivate(on bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.private = on
}

// Required reports whether the room asks for a PIN.
func (g *PinGate) Required() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.private
}

// OwnerKey is the host's key: the device gives it only to its linked
// browsers, which send it instead of a PIN.
func (g *PinGate) OwnerKey() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.owner
}

// Issue makes a new invitation: a PIN no other live invitation has.
func (g *PinGate) Issue() Pass {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	g.dropExpired(now)
	if len(g.passes) >= maxPasses {
		// Forget the oldest invitation nobody used; used ones stay.
		if i := slices.IndexFunc(g.passes, func(p *pass) bool { return p.token == "" }); i >= 0 {
			g.passes = slices.Delete(g.passes, i, i+1)
		}
	}
	pin := NewPIN()
	for slices.ContainsFunc(g.passes, func(p *pass) bool { return p.pin == pin }) {
		pin = NewPIN()
	}
	p := &pass{pin: pin, expires: now.Add(passTTL)}
	g.passes = append(g.passes, p)
	return Pass{Pin: pin, ExpiresAt: p.expires}
}

// dropExpired forgets invitations nobody used in time.
func (g *PinGate) dropExpired(now time.Time) {
	g.passes = slices.DeleteFunc(g.passes, func(p *pass) bool {
		return p.token == "" && !now.Before(p.expires)
	})
}

// Check answers one try of a guest: an invitation's PIN, or a token (the
// owner's key, or the one a guest got when it first came in).
func (g *PinGate) Check(peer, pin, token string) PinResult {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	if !g.private {
		return PinResult{OK: true}
	}
	g.dropExpired(now)
	// The host's key and the tokens of guests already in come first: someone
	// guessing PINs can lock the PIN path, but never lock them out.
	if token != "" {
		if equal(token, g.owner) {
			delete(g.tries, peer)
			return PinResult{OK: true, Owner: true}
		}
		for _, p := range g.passes {
			if p.token != "" && equal(token, p.token) {
				delete(g.tries, peer)
				return PinResult{OK: true, Token: p.token}
			}
		}
	}
	if now.Before(g.lockedUntil) {
		return PinResult{Reason: "locked", RetryAfter: int(g.lockedUntil.Sub(now).Seconds()) + 1}
	}
	if g.tries[peer] >= pinTriesPerPeer {
		return PinResult{Reason: "blocked"}
	}
	if token != "" {
		return g.fail(peer, now, "wrong") // a made-up token
	}
	for _, p := range g.passes {
		if !equal(pin, p.pin) {
			continue
		}
		if p.token != "" {
			return g.fail(peer, now, "used")
		}
		p.token = newToken()
		delete(g.tries, peer)
		return PinResult{OK: true, Token: p.token}
	}
	return g.fail(peer, now, "wrong")
}

// fail counts a wrong try and says what the guest may do next.
func (g *PinGate) fail(peer string, now time.Time, reason string) PinResult {
	g.tries[peer]++
	recent := g.fails[:0]
	for _, t := range g.fails {
		if now.Sub(t) < pinWindow {
			recent = append(recent, t)
		}
	}
	g.fails = append(recent, now)
	if len(g.fails) >= pinRoomFails {
		g.lockedUntil = now.Add(pinLock)
		g.fails = nil
		return PinResult{Reason: "locked", RetryAfter: int(pinLock.Seconds())}
	}
	if left := pinTriesPerPeer - g.tries[peer]; left > 0 {
		return PinResult{Reason: reason, Left: left}
	}
	return PinResult{Reason: "blocked"}
}

// equal compares secrets in constant time.
func equal(a, b string) bool {
	return a != "" && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// Forget drops a guest that left. Its tries stay counted in the room.
func (g *PinGate) Forget(peer string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.tries, peer)
}
