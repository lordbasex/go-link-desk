// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package decoder decodes what the other side of a call sends: H.264
// video (the system's decoder, VideoToolbox, on macOS) and Opus sound
// (libopus).
package decoder

import (
	"errors"
	"image"
)

// ErrUnsupported: no video decoder on this system in this build.
var ErrUnsupported = errors.New("decoder: not supported on this system")

// ErrNeedKeyframe: the decoder waits for a keyframe with its parameter
// sets (ask the sender for one).
var ErrNeedKeyframe = errors.New("decoder: waiting for a keyframe")

// OpusRate is the only rate WebRTC uses for Opus.
const OpusRate = 48000

// ErrCorrupt: the frame decoded from a missing or damaged one before it;
// the picture is wrong until the next keyframe.
var ErrCorrupt = errors.New("decoder: vp8 frame refers to a lost one")

// I420 is a decoded picture: its three planes, each with its own row
// length (stride). The slices stay valid only until the next Decode.
type I420 struct {
	Width, Height     int
	Y, U, V           []byte
	StrideY, StrideUV int
}

// NAL unit types of H.264 the decoder looks at.
const (
	nalIDR = 5
	nalSPS = 7
	nalPPS = 8
)

// splitAnnexB returns the NAL units of an Annex B access unit (start codes
// 00 00 01 or 00 00 00 01 between them), without their start codes.
func splitAnnexB(b []byte) [][]byte {
	var nals [][]byte
	start := -1
	for i := 0; i+2 < len(b); {
		if b[i] == 0 && b[i+1] == 0 && b[i+2] == 1 {
			if start >= 0 {
				end := i
				if end > start && b[end-1] == 0 {
					end-- // the 4-byte start code's leading zero
				}
				nals = append(nals, b[start:end])
			}
			i += 3
			start = i
			continue
		}
		i++
	}
	if start >= 0 && start < len(b) {
		nals = append(nals, b[start:])
	}
	out := nals[:0]
	for _, n := range nals {
		if len(n) > 0 {
			out = append(out, n)
		}
	}
	return out
}

// accessUnit is one H.264 access unit ready for the decoder: its
// parameter sets, if it carries them, and its other NAL units as AVCC
// (each prefixed with its 4-byte length), what VideoToolbox takes.
type accessUnit struct {
	sps, pps []byte
	avcc     []byte
	key      bool
}

func parseAccessUnit(annexB []byte) accessUnit {
	var au accessUnit
	for _, n := range splitAnnexB(annexB) {
		switch n[0] & 0x1F {
		case nalSPS:
			au.sps = n
		case nalPPS:
			au.pps = n
		default:
			if n[0]&0x1F == nalIDR {
				au.key = true
			}
			l := len(n)
			au.avcc = append(au.avcc, byte(l>>24), byte(l>>16), byte(l>>8), byte(l))
			au.avcc = append(au.avcc, n...)
		}
	}
	return au
}

// Picture is a decoded frame, RGBA.
type Picture = *image.RGBA
