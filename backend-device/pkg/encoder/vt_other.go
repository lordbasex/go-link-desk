// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build (!darwin && !windows && !linux) || !cgo

package encoder

import (
	"errors"
	"time"
	"unsafe"
)

// VideoToolbox is macOS's, Media Foundation Windows' and VA-API Linux's:
// elsewhere there is no system H.264 encoder (sessions use VP8).
type sysEncoder struct {
	hardware int
	name     string
}

func newSys(Config, int, func([]byte, bool, time.Duration)) (*sysEncoder, error) {
	return nil, errors.New("h264: VideoToolbox is only on macOS")
}

func (v *sysEncoder) write([]byte, bool) error { return errors.New("h264: no VideoToolbox") }
func (v *sysEncoder) writeAt([]byte, time.Duration, bool) error {
	return errors.New("h264: no system encoder")
}
func (v *sysEncoder) writePixelBuffer(unsafe.Pointer, time.Duration, bool) error {
	return errors.New("h264: no VideoToolbox")
}
func (v *sysEncoder) setBitrate(int) {}
func (v *sysEncoder) close()         {}

const (
	hasSys       = false
	pixelBuffers = false
	probeOutput  = false
)
