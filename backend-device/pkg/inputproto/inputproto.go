// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package inputproto reads the viewer's input packets (docs/protocol.md,
// section 5): pointer moves on the unreliable "pointer" channel, and
// buttons, wheel, keys, text and release-all on the reliable "input"
// channel. Big endian; protocol/vectors/input.json is the reference.
package inputproto

import (
	"encoding/binary"
	"errors"
	"unicode/utf8"
)

// Packet types.
const (
	TypeAbs        = 0x01
	TypeRel        = 0x02
	TypeButton     = 0x10
	TypeWheel      = 0x20
	TypeKey        = 0x30
	TypeText       = 0x31
	TypeReleaseAll = 0x3F
)

// Mouse buttons.
const (
	ButtonLeft    = 0
	ButtonMiddle  = 1
	ButtonRight   = 2
	ButtonBack    = 3
	ButtonForward = 4
)

// Modifier bits of a key packet.
const (
	ModShift = 1 << iota
	ModCtrl
	ModAlt
	ModMeta
	ModCaps
)

// Wheel modes.
const (
	WheelPixels = 0
	WheelLines  = 1
)

// MaxText bounds a text packet (an IME commit, a paste typed out in
// pieces).
const MaxText = 1024

// Packet is one decoded packet; the fields its Type uses are set.
type Packet struct {
	Type    byte
	Seq     uint16 // abs, rel
	Display uint8  // abs, button: the display's index in the host's list
	X, Y    uint16 // abs, button: 0..65535 over the display
	DX, DY  int16  // rel (pointer lock), wheel
	Button  uint8
	Down    bool  // button, key
	Repeat  bool  // key: the keyboard's own repeat
	Mode    uint8 // wheel
	Keycode uint16
	Mods    uint8
	Text    string
}

// ErrBadPacket: too short, too long, an unknown type or a bad value.
var ErrBadPacket = errors.New("inputproto: bad packet")

// Decode reads one packet.
func Decode(b []byte) (Packet, error) {
	if len(b) == 0 {
		return Packet{}, ErrBadPacket
	}
	p := Packet{Type: b[0]}
	u16 := func(i int) uint16 { return binary.BigEndian.Uint16(b[i:]) }
	want := map[byte]int{TypeAbs: 8, TypeRel: 7, TypeButton: 8, TypeWheel: 6, TypeKey: 5, TypeReleaseAll: 1}
	if n, ok := want[p.Type]; ok && len(b) != n {
		return Packet{}, ErrBadPacket
	}
	switch p.Type {
	case TypeAbs:
		p.Seq, p.Display, p.X, p.Y = u16(1), b[3], u16(4), u16(6)
	case TypeRel:
		p.Seq, p.DX, p.DY = u16(1), int16(u16(3)), int16(u16(5))
	case TypeButton:
		if b[1] > ButtonForward || b[2] > 1 {
			return Packet{}, ErrBadPacket
		}
		p.Button, p.Down, p.Display, p.X, p.Y = b[1], b[2] == 1, b[3], u16(4), u16(6)
	case TypeWheel:
		if b[5] > WheelLines {
			return Packet{}, ErrBadPacket
		}
		p.DX, p.DY, p.Mode = int16(u16(1)), int16(u16(3)), b[5]
	case TypeKey:
		if b[3] > 2 {
			return Packet{}, ErrBadPacket
		}
		p.Keycode, p.Down, p.Repeat, p.Mods = u16(1), b[3] != 0, b[3] == 2, b[4]
	case TypeText:
		if len(b) < 2 || len(b)-1 > MaxText || !utf8.Valid(b[1:]) {
			return Packet{}, ErrBadPacket
		}
		p.Text = string(b[1:])
	case TypeReleaseAll:
	default:
		return Packet{}, ErrBadPacket
	}
	return p, nil
}

// SeqNewer reports whether a pointer move's seq comes after last, with
// wrap-around: a move older than the last applied is dropped.
func SeqNewer(seq, last uint16) bool {
	return seq != last && seq-last < 0x8000
}
