// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package lordshare is a client of LordOS's public screen-sharing API
// (LordOS: docs/SCREEN-SHARING.md, server/src/remote/sharewire.h). It is the
// only place in go-link Desk that knows that API's wire format, so a change
// there is a change here and nowhere else. It holds no go-link types.
//
// The framing is the session protocol's: u32 length (of what follows,
// little endian), u8 type, data. The server is strict: a wrong length or an
// unknown type ends the connection, and every value is checked before it is
// sent here so that this never happens.
package lordshare

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"
)

// Client to system (sharewire.h, proto.h).
const (
	msgPtr      = 11
	msgBtn      = 12
	msgWheel    = 13
	msgKey      = 14
	msgSetClip  = 15
	msgResize   = 17
	msgScroll   = 30
	msgKeyframe = 33
	msgRate     = 34
	msgStart    = 50
	msgQuery    = 51
	msgStop     = 52
	msgAwake    = 53
	msgLock     = 54
	msgText     = 55
	msgHear     = 56
)

// System to client.
const (
	msgSize    = 1
	msgCursor  = 4
	msgShape   = 5
	msgClip    = 6
	msgVFrame  = 40
	msgAFrame  = 41
	msgVInfo   = 43
	msgState   = 60
	msgRefused = 61
	msgEnd     = 62
)

// What a share asks for and what it was granted.
const (
	FlagVideo = 1 << iota
	FlagAudio
	FlagInput
	FlagClipboard
	FlagAll = FlagVideo | FlagAudio | FlagInput | FlagClipboard
)

// Codecs of a share.
const (
	CodecH264 = 1
	CodecVP8  = 2
)

// Limits of the API (sharewire.h, sharewire.c).
const (
	MinHz    = 24
	MaxHz    = 240
	MinKbps  = 300
	MaxKbps  = 20000
	MaxText  = 1024    // SHARE_TEXT, bytes
	MaxClip  = 1 << 20 // a clipboard text, bytes
	maxInput = 40 << 20
	maxKey   = 0x2ff // key codes stay below it
)

// Reason is why a start is refused or a share ended: a closed list, the
// same order as SwReason.
type Reason uint8

// The reasons.
const (
	OK Reason = iota
	NoIdentity
	Denied
	NoPerson
	NoIndicator
	Busy
	Locked
	Away
	RemoteDesktop
	Restricted
	Revoked
	Stopped
	BadRequest
	EndedByApp
)

var reasonNames = []string{"ok", "no_identity", "denied", "no_person", "no_indicator", "busy", "locked", "away", "remote_desktop", "restricted", "revoked", "stopped", "bad_request", "ended_by_app"}

func (r Reason) String() string {
	if int(r) < len(reasonNames) {
		return reasonNames[r]
	}
	return fmt.Sprintf("reason_%d", uint8(r))
}

// frame appends one message to b.
func frame(b []byte, t byte, parts ...[]byte) []byte {
	n := 1
	for _, p := range parts {
		n += len(p)
	}
	b = binary.LittleEndian.AppendUint32(b, uint32(n))
	b = append(b, t)
	for _, p := range parts {
		b = append(b, p...)
	}
	return b
}

// readFrame reads one message into buf (grown if needed).
func readFrame(r io.Reader, buf []byte) (byte, []byte, error) {
	var hd [5]byte
	if _, err := io.ReadFull(r, hd[:]); err != nil {
		return 0, buf, err
	}
	n := binary.LittleEndian.Uint32(hd[:])
	if n == 0 || n > maxInput {
		return 0, buf, fmt.Errorf("lordshare: bad message length %d", n)
	}
	n--
	if uint32(cap(buf)) < n {
		buf = make([]byte, n)
	}
	buf = buf[:n]
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, buf, err
	}
	return hd[4], buf, nil
}

// errClosed: the connection is gone.
var errClosed = errors.New("lordshare: not connected")

// cleanText is s as the server takes it: valid UTF-8 without NUL.
func cleanText(s string) string {
	b := make([]rune, 0, len(s))
	for _, r := range s {
		if r != 0 && r != utf8.RuneError {
			b = append(b, r)
		}
	}
	return string(b)
}

// splitText cuts s at code points into pieces of at most n bytes.
func splitText(s string, n int) []string {
	var out []string
	for len(s) > n {
		i := n
		for i > 0 && !utf8.RuneStart(s[i]) {
			i--
		}
		out = append(out, s[:i])
		s = s[i:]
	}
	if s != "" {
		out = append(out, s)
	}
	return out
}
