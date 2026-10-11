// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package voice is the host's side of a call's sound: the microphone, with
// the system's echo cancellation, and the speaker for the other side's
// voice. On macOS both run in one AVAudioEngine with voice processing, so
// what the speaker plays is taken out of what the microphone hears.
// Samples are 16-bit mono at Rate.
package voice

import "errors"

// Rate is the sound's sample rate: what Opus and WebRTC use.
const Rate = 48000

// ErrUnsupported: no voice on this system in this build.
var ErrUnsupported = errors.New("voice: not supported on this system")
