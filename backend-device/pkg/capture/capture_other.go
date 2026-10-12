// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !darwin && !linux

package capture

import "unsafe"

// Displays has nothing to list here yet.
func Displays() ([]Display, error) { return nil, ErrUnsupported }

// Stream is a running capture (never on this system yet).
type Stream struct{}

// Start is not supported here yet.
func Start(Config, func(Frame), func(error)) (*Stream, error) { return nil, ErrUnsupported }

func (s *Stream) Update(Config) error   { return ErrUnsupported }
func (s *Stream) Config() Config        { return Config{} }
func (s *Stream) Stop()                 {}
func (s *Stream) OnAudio(func([]int16)) {}

func Retain(unsafe.Pointer)                         {}
func Release(unsafe.Pointer)                        {}
func ToI420(unsafe.Pointer, []byte, int, int) error { return ErrUnsupported }
func HasAccess() bool                               { return false }
func RequestAccess() bool                           { return false }

// Supported reports whether this build captures the screen.
const Supported = false

// Problem is why the screen cannot be captured now ("" when it can).
func Problem() string { return "" }
