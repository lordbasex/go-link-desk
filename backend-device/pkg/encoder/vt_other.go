// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !darwin

package encoder

import "errors"

// VideoToolbox is macOS's: elsewhere the encoder "videotoolbox" is not there.
type vtEncoder struct{}

func newVT(cfg Config, gop int, onFrame func([]byte)) (*vtEncoder, error) {
	return nil, errors.New("h264: VideoToolbox is only on macOS")
}

func (v *vtEncoder) write(i420 []byte, key bool) error { return errors.New("h264: no VideoToolbox") }
func (v *vtEncoder) close()                            {}

const hasVT = false
