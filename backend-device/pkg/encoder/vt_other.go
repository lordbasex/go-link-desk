// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !darwin

package encoder

import (
	"errors"
	"time"
	"unsafe"
)

// VideoToolbox is macOS's: elsewhere there is no system H.264 encoder yet
// (Media Foundation on Windows comes in a later phase).
type vtEncoder struct{ hardware int }

func newVT(Config, int, func([]byte, bool, time.Duration)) (*vtEncoder, error) {
	return nil, errors.New("h264: VideoToolbox is only on macOS")
}

func (v *vtEncoder) write([]byte, bool) error { return errors.New("h264: no VideoToolbox") }
func (v *vtEncoder) writePixelBuffer(unsafe.Pointer, time.Duration, bool) error {
	return errors.New("h264: no VideoToolbox")
}
func (v *vtEncoder) setBitrate(int) {}
func (v *vtEncoder) close()         {}

const hasVT = false
