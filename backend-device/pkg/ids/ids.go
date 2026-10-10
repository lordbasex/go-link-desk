// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package ids generates random identifiers.
package ids

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
)

// New returns 128 random bits encoded as 32 hex characters.
func New() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("ids: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// NewUUIDv4 returns a random RFC 9562 version 4 UUID.
func NewUUIDv4() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("ids: crypto/rand failed: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 9562 variant
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ValidUUID reports whether s has the canonical textual UUID format.
func ValidUUID(s string) bool {
	return uuidPattern.MatchString(s)
}
