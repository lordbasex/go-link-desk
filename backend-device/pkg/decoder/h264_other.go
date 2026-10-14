// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !darwin || !cgo

package decoder

import "time"

// H264 is not available on this system yet.
type H264 struct{}

// NewH264 returns a decoder that decodes nothing.
func NewH264(func(Picture)) *H264 { return &H264{} }

// Decode reports that this system has no decoder.
func (d *H264) Decode([]byte, time.Duration) error { return ErrUnsupported }

// Close does nothing.
func (d *H264) Close() {}

// HasH264 reports whether this build decodes H.264.
const HasH264 = false
