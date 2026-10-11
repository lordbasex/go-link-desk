// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !darwin

package camera

// Supported reports whether this build captures the camera.
const Supported = false

// Stream is a running camera (never on this system yet).
type Stream struct{}

// Start is not supported here yet.
func Start(Config, func(Frame)) (*Stream, error) { return nil, ErrUnsupported }

// Stop does nothing.
func (s *Stream) Stop() {}

// HasAccess: no camera here.
func HasAccess() bool { return false }

// RequestAccess: no camera here.
func RequestAccess() bool { return false }
