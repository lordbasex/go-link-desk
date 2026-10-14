// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !cgo

package decoder

// Without cgo there is no libvpx or libopus to decode with.

// VP8 decodes a VP8 stream; this build has no libvpx.
type VP8 struct{}

// NewVP8 fails: this build has no libvpx.
func NewVP8(int) (*VP8, error) { return nil, ErrUnsupported }

func (v *VP8) Decode([]byte) (*I420, error) { return nil, ErrUnsupported }
func (v *VP8) Close()                       {}

// Opus decodes Opus; this build has no libopus.
type Opus struct{}

// NewOpus fails: this build has no libopus.
func NewOpus(int) (*Opus, error) { return nil, ErrUnsupported }

func (o *Opus) Decode([]byte) ([]int16, error) { return nil, ErrUnsupported }
func (o *Opus) Close()                         {}
