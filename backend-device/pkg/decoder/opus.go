// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package decoder

/*
#cgo pkg-config: opus
#include <opus.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// Opus decodes Opus to 16-bit samples at 48 kHz. It is not safe for
// concurrent use.
type Opus struct {
	dec      *C.OpusDecoder
	channels int
	pcm      []int16
}

// NewOpus opens a decoder for 1 or 2 channels.
func NewOpus(channels int) (*Opus, error) {
	if channels != 1 && channels != 2 {
		return nil, fmt.Errorf("decoder: opus channels must be 1 or 2, got %d", channels)
	}
	var cerr C.int
	d := C.opus_decoder_create(OpusRate, C.int(channels), &cerr)
	if d == nil || cerr != C.OPUS_OK {
		return nil, fmt.Errorf("decoder: opus init failed (%d)", int(cerr))
	}
	// The longest packet: 120 ms.
	return &Opus{dec: d, channels: channels, pcm: make([]int16, OpusRate*120/1000*channels)}, nil
}

// Decode turns one packet into samples (interleaved if stereo), valid
// until the next call. A nil packet conceals one lost 20 ms frame.
func (o *Opus) Decode(packet []byte) ([]int16, error) {
	if o.dec == nil {
		return nil, errors.New("decoder: closed")
	}
	var p *C.uchar
	if len(packet) > 0 {
		p = (*C.uchar)(unsafe.Pointer(&packet[0]))
	}
	frames := len(o.pcm) / o.channels
	if packet == nil {
		frames = OpusRate / 50
	}
	n := C.opus_decode(o.dec, p, C.opus_int32(len(packet)), (*C.opus_int16)(unsafe.Pointer(&o.pcm[0])), C.int(frames), 0)
	if n < 0 {
		return nil, fmt.Errorf("decoder: opus decode failed (%d)", int(n))
	}
	return o.pcm[:int(n)*o.channels], nil
}

// Close frees the decoder.
func (o *Opus) Close() {
	if o.dec != nil {
		C.opus_decoder_destroy(o.dec)
		o.dec = nil
	}
}
