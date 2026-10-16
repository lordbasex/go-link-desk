// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// The unattended password is kept as an argon2id hash (RFC 9106) in the
// PHC string format. The parameters are OWASP's minimum for argon2id:
// 19 MiB, two passes, one lane.
const (
	argonMemory  = 19 * 1024 // KiB
	argonTime    = 2
	argonThreads = 1
	argonKeyLen  = 32
	argonSaltLen = 16
)

// Limits of the password itself, in characters.
const (
	MinPassword = 8
	MaxPassword = 128
)

// ErrPasswordLength: the password is too short or too long.
var ErrPasswordLength = fmt.Errorf("the password must have %d to %d characters", MinPassword, MaxPassword)

// HashPassword returns the argon2id hash of password, with a random salt.
func HashPassword(password string) (string, error) {
	if n := utf8.RuneCountInString(password); n < MinPassword || n > MaxPassword {
		return "", ErrPasswordLength
	}
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// CheckPassword reports whether password matches hash. A hash it cannot
// read never matches.
func CheckPassword(hash, password string) bool {
	if len(password) > 4*MaxPassword {
		return false
	}
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return false
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	// A hash someone edited by hand must not make every check cost a
	// gigabyte or an hour.
	if m == 0 || m > 256*1024 || t == 0 || t > 10 || p == 0 || p > 8 {
		return false
	}
	b64 := base64.RawStdEncoding
	salt, err1 := b64.DecodeString(parts[4])
	want, err2 := b64.DecodeString(parts[5])
	if errors.Join(err1, err2) != nil || len(salt) < 8 || len(want) < 16 || len(want) > 64 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// PasswordGate limits the guesses at the unattended password with the
// PIN gate's numbers: a browser gets pinTriesPerPeer tries, and
// pinRoomFails wrong passwords within pinWindow lock every try for
// pinLock. Only browsers that already proved a link token get here.
type PasswordGate struct {
	now func() time.Time

	mu          sync.Mutex
	tries       map[string]int
	fails       []time.Time
	pending     int // guesses admitted and still hashing (reserved slots)
	lockedUntil time.Time
}

// NewPasswordGate builds the gate; now may be nil (time.Now).
func NewPasswordGate(now func() time.Time) *PasswordGate {
	if now == nil {
		now = time.Now
	}
	return &PasswordGate{now: now, tries: map[string]int{}}
}

// Check answers one try of peer: ok runs only when the gate lets it try,
// so a locked gate costs no hashing.
//
// The attempt is reserved under the lock before ok() (the argon2 hashing)
// runs without it: the per-peer and room counters are charged up front and
// rolled back only if the guess turns out right. So the room-wide cap is
// enforced against guesses *started*, not finished - a burst fired at once
// from many peerIDs can no longer all pass the check while each sits in
// argon2 and slip the room past its limit.
func (g *PasswordGate) Check(peer string, ok func() bool) PinResult {
	g.mu.Lock()
	now := g.now()
	if now.Before(g.lockedUntil) {
		left := int(g.lockedUntil.Sub(now).Seconds()) + 1
		g.mu.Unlock()
		return PinResult{Reason: "locked", RetryAfter: left}
	}
	if g.tries[peer] >= pinTriesPerPeer {
		g.mu.Unlock()
		return PinResult{Reason: "blocked"}
	}
	g.trimFails(now)
	// Count guesses already in flight too: if the committed and reserved
	// wrong guesses would reach the room cap, lock now rather than letting
	// the burst through.
	if len(g.fails)+g.pending >= pinRoomFails {
		g.lockedUntil = now.Add(pinLock)
		g.mu.Unlock()
		return PinResult{Reason: "locked", RetryAfter: int(pinLock.Seconds())}
	}
	g.tries[peer]++
	g.pending++
	g.mu.Unlock()

	right := ok()

	g.mu.Lock()
	defer g.mu.Unlock()
	g.pending--
	if right {
		delete(g.tries, peer)
		return PinResult{OK: true}
	}
	now = g.now()
	g.trimFails(now)
	g.fails = append(g.fails, now)
	if len(g.fails) >= pinRoomFails {
		g.lockedUntil = now.Add(pinLock)
		g.fails = nil
		return PinResult{Reason: "locked", RetryAfter: int(pinLock.Seconds())}
	}
	if left := pinTriesPerPeer - g.tries[peer]; left > 0 {
		return PinResult{Reason: "wrong", Left: left}
	}
	return PinResult{Reason: "blocked"}
}

// trimFails drops wrong guesses older than the window. Call it holding mu.
func (g *PasswordGate) trimFails(now time.Time) {
	recent := g.fails[:0]
	for _, t := range g.fails {
		if now.Sub(t) < pinWindow {
			recent = append(recent, t)
		}
	}
	g.fails = recent
}

// Forget drops a browser that left. Its wrong tries stay counted.
func (g *PasswordGate) Forget(peer string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.tries, peer)
}
