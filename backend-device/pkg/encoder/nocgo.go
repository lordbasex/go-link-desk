// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !cgo

package encoder

import (
	"errors"
	"time"
)

// Without cgo there is no libvpx or libopus: a build that streams no
// picture or sound (go-link-desk-terminal) compiles, and asks for none.
// Codecs says this build has libvpx and libopus (cgo).
const Codecs = false

var errNoCgo = errors.New("encoder: built without cgo (no libvpx or libopus)")

// VP8 is a libvpx VP8 encoder; this build has none.
type VP8 struct{}

// NewVP8 fails: this build has no libvpx.
func NewVP8(Config) (*VP8, error) { return nil, errNoCgo }

func (v *VP8) Encode([]byte, bool) ([]byte, bool, error) { return nil, false, errNoCgo }
func (v *VP8) EncodeAt([]byte, time.Duration, bool) ([]byte, bool, error) {
	return nil, false, errNoCgo
}
func (v *VP8) SetBitrate(int) error { return errNoCgo }
func (v *VP8) Close()               {}

// Opus is a libopus encoder; this build has none.
type Opus struct{}

// NewOpus fails: this build has no libopus.
func NewOpus(int, int) (*Opus, error) { return nil, errNoCgo }

func (o *Opus) Encode([]int16) ([]byte, error) { return nil, errNoCgo }
func (o *Opus) SetPacketLoss(int)              {}
func (o *Opus) PacketLoss() int                { return 0 }
func (o *Opus) Close()                         {}
