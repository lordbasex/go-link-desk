// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package encoder

/*
#cgo pkg-config: opus
#include <opus.h>
#include <stdlib.h>

static OpusEncoder *opus_new(int rate, int channels, int bitrate, int *err) {
	OpusEncoder *e = opus_encoder_create(rate, channels, OPUS_APPLICATION_AUDIO, err);
	if (e && *err == OPUS_OK) {
		opus_encoder_ctl(e, OPUS_SET_BITRATE(bitrate));
		opus_encoder_ctl(e, OPUS_SET_COMPLEXITY(5));
	}
	return e;
}

static void opus_set_loss(OpusEncoder *e, int pct) {
	opus_encoder_ctl(e, OPUS_SET_PACKET_LOSS_PERC(pct));
	opus_encoder_ctl(e, OPUS_SET_INBAND_FEC(pct > 0 ? 1 : 0));
}
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// OpusSampleRate is the only rate WebRTC uses for Opus.
const OpusSampleRate = 48000

// OpusFrameSamples is one 20 ms frame per channel at 48 kHz.
const OpusFrameSamples = OpusSampleRate / 50

// Opus is a libopus encoder. It is not safe for concurrent use.
type Opus struct {
	enc      *C.OpusEncoder
	channels int
	out      []byte
	lossPct  int
}

// NewOpus opens an encoder for 1 or 2 channels at 48 kHz.
func NewOpus(channels, bitrate int) (*Opus, error) {
	if channels != 1 && channels != 2 {
		return nil, fmt.Errorf("encoder: opus channels must be 1 or 2, got %d", channels)
	}
	var cerr C.int
	e := C.opus_new(OpusSampleRate, C.int(channels), C.int(bitrate), &cerr)
	if e == nil || cerr != C.OPUS_OK {
		return nil, fmt.Errorf("encoder: opus init failed (%d)", int(cerr))
	}
	return &Opus{enc: e, channels: channels, out: make([]byte, 4000)}, nil
}

// Encode compresses one 20 ms frame of interleaved 16-bit samples
// (OpusFrameSamples * channels values).
func (o *Opus) Encode(pcm []int16) ([]byte, error) {
	if o.enc == nil {
		return nil, errors.New("encoder: closed")
	}
	if len(pcm) != OpusFrameSamples*o.channels {
		return nil, fmt.Errorf("encoder: opus frame has %d samples, want %d", len(pcm), OpusFrameSamples*o.channels)
	}
	n := C.opus_encode(o.enc, (*C.opus_int16)(unsafe.Pointer(&pcm[0])), C.int(OpusFrameSamples),
		(*C.uchar)(unsafe.Pointer(&o.out[0])), C.opus_int32(len(o.out)))
	if n < 0 {
		return nil, fmt.Errorf("encoder: opus encode failed (%d)", int(n))
	}
	return append([]byte(nil), o.out[:n]...), nil
}

// SetPacketLoss tells the encoder how many packets in a hundred the
// listeners lose (0-100). Above 0 it turns in-band FEC on (redundancy in
// SILK frames) and, for music in CELT frames, leans less on the previous
// frame, so one lost packet hurts fewer of the ones after it. Call it from
// the goroutine that encodes.
func (o *Opus) SetPacketLoss(pct int) {
	pct = max(0, min(100, pct))
	if o.enc == nil || pct == o.lossPct {
		return
	}
	o.lossPct = pct
	C.opus_set_loss(o.enc, C.int(pct))
}

// PacketLoss is the last value given to SetPacketLoss.
func (o *Opus) PacketLoss() int { return o.lossPct }

// Close releases the encoder.
func (o *Opus) Close() {
	if o.enc != nil {
		C.opus_encoder_destroy(o.enc)
		o.enc = nil
	}
}
