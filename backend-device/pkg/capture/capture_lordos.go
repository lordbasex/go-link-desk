// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux && headless && lordos

package capture

import (
	"unsafe"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/lordshare"
)

// On LordOS frames arrive already encoded, through
// services.EncodedSource: nothing raw passes here. This file answers what
// the rest asks about the screen: one display, the session's.

// Displays is the session's one picture.
func Displays() ([]Display, error) {
	w, h := 1280, 800
	if c := lordshare.Shared(); c != nil {
		if st, ok := c.State(); ok && st.W > 0 {
			w, h = st.W, st.H
		}
	}
	return []Display{{ID: 1, Name: "LordOS", Width: w, Height: h, PointsWidth: w, PointsHeight: h, Primary: true}}, nil
}

// Stream never runs here: see EncodedSource.
type Stream struct{}

// Start is not how frames arrive here.
func Start(Config, func(Frame), func(error)) (*Stream, error) { return nil, ErrUnsupported }

func (s *Stream) Update(Config) error   { return ErrUnsupported }
func (s *Stream) Config() Config        { return Config{} }
func (s *Stream) Stop()                 {}
func (s *Stream) OnAudio(func([]int16)) {}

func Retain(unsafe.Pointer)                         {}
func Release(unsafe.Pointer)                        {}
func ToI420(unsafe.Pointer, []byte, int, int) error { return ErrUnsupported }

// HasAccess reports the system's screen-share permission.
func HasAccess() bool { return granted(lordshare.FlagVideo) }

// RequestAccess: the system asks by itself on the first share.
func RequestAccess() bool { return HasAccess() }

// Supported: this build shows the screen.
const Supported = true

// Problem is why the screen cannot be captured now ("" when it can).
func Problem() string { return "" }

// RemoteControl reports the system's remote-input permission.
func RemoteControl() bool { return granted(lordshare.FlagInput) }

// AsksInputApart: LordOS asks for the two permissions in its one prompt.
func AsksInputApart() bool { return false }

func granted(flag int) bool {
	c := lordshare.Shared()
	if c == nil {
		return false
	}
	st, ok := c.State()
	return ok && st.Granted&flag != 0
}
