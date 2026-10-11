// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !darwin

package voice

// Supported reports whether this build has call sound.
const Supported = false

// Engine is the call sound (never on this system yet).
type Engine struct{}

// Start is not supported here yet.
func Start(bool, func([]int16)) (*Engine, error) { return nil, ErrUnsupported }

func (e *Engine) Play([]int16) {}
func (e *Engine) Mute(bool)    {}
func (e *Engine) Stop()        {}

// HasMicAccess: no microphone here.
func HasMicAccess() bool { return false }

// RequestMicAccess: no microphone here.
func RequestMicAccess() bool { return false }
