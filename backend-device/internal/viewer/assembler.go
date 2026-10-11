// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package viewer

import (
	"time"

	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
)

// How long a missing packet is waited for (NACK resends it) before the
// frame is given up, and how many packets wait at most.
const (
	assembleWait = 250 * time.Millisecond
	assembleMax  = 2048
)

// assembler puts RTP packets back into frames, in order. A frame is the
// packets from the one after the last frame's marker up to the next
// marker, with no gap. Unlike Pion's sample builder it hands a frame over
// as soon as its last packet arrives, not when the next frame starts: a
// still screen sends nothing after its last frame.
//
// A gap that is not filled in time drops the frames until a keyframe, and
// asks for one.
type assembler struct {
	codec   string
	pkts    map[uint16]*rtp.Packet
	next    uint16 // the first packet of the next frame
	started bool
	// keyWait: frames are dropped until a keyframe (at the start, and
	// after a loss); askKey: a keyframe should be asked for.
	keyWait bool
	askKey  bool
	gapAt   time.Time // since when the next packet is missing
}

func newAssembler(codec string) *assembler {
	return &assembler{codec: codec, pkts: map[uint16]*rtp.Packet{}, keyWait: true}
}

// wantKeyframe reports (once) that a keyframe should be asked for.
func (a *assembler) wantKeyframe() bool {
	k := a.askKey
	a.askKey = false
	return k
}

// push takes a packet and returns the frames it completes, and how many
// packets were given up.
func (a *assembler) push(p *rtp.Packet, now time.Time) (frames []Frame, lost int) {
	if !a.started {
		a.started = true
		a.next = p.SequenceNumber
	}
	if seqBefore(p.SequenceNumber, a.next) {
		return nil, 0 // late or a duplicate
	}
	if len(p.Payload) == 0 && !p.Marker {
		return nil, 0 // padding
	}
	a.pkts[p.SequenceNumber] = p
	progressed := false
	for {
		f, ok := a.take()
		if !ok {
			break
		}
		progressed = true
		if f != nil {
			frames = append(frames, *f)
		}
	}
	if len(a.pkts) == 0 {
		a.gapAt = time.Time{}
		return frames, 0
	}
	// Packets wait for one that is missing (or still on its way): time it
	// from the last frame taken.
	if a.gapAt.IsZero() || progressed {
		a.gapAt = now
	}
	if now.Sub(a.gapAt) < assembleWait && len(a.pkts) < assembleMax {
		return frames, 0
	}
	// Give it up: go on from the oldest packet that starts a frame of its
	// own (the one after a marker, or the first of a newer timestamp),
	// and drop frames until a keyframe.
	lost = a.skip()
	a.keyWait, a.askKey, a.gapAt = true, true, time.Time{}
	for {
		f, ok := a.take()
		if !ok {
			break
		}
		if f != nil {
			frames = append(frames, *f)
		}
	}
	return frames, lost
}

// take builds the frame that starts at next, if all of it is here. A
// frame dropped while waiting for a keyframe comes back as nil, ok.
func (a *assembler) take() (*Frame, bool) {
	var seqs []uint16
	seq := a.next
	for {
		p, ok := a.pkts[seq]
		if !ok {
			return nil, false
		}
		seqs = append(seqs, seq)
		if p.Marker {
			break
		}
		seq++
		if len(seqs) > assembleMax {
			return nil, false
		}
	}
	data, ok := a.depacketize(seqs)
	for _, s := range seqs {
		delete(a.pkts, s)
	}
	a.next = seq + 1
	if !ok {
		a.keyWait, a.askKey = true, true
		return nil, true
	}
	key := isKeyframe(a.codec, data)
	if a.keyWait && !key {
		a.askKey = true
		return nil, true
	}
	a.keyWait = false
	return &Frame{Codec: a.codec, Data: data, Keyframe: key}, true
}

// skip moves next past the missing packets: to the oldest waiting packet
// that follows a marker or has a newer timestamp than the stuck frame's.
// It returns how many sequence numbers were given up.
func (a *assembler) skip() int {
	var oldest uint16
	first := true
	for s := range a.pkts {
		if first || seqBefore(s, oldest) {
			oldest, first = s, false
		}
	}
	if first {
		return 0
	}
	stuck := a.pkts[oldest].Timestamp
	start := oldest
	found := false
	for i := 0; i < assembleMax; i++ {
		s := oldest + uint16(i)
		p, ok := a.pkts[s]
		if !ok {
			continue
		}
		if p.Timestamp != stuck {
			start, found = s, true
			break
		}
		if p.Marker {
			start, found = s+1, true
			break
		}
	}
	if !found {
		start = oldest
	}
	lost := int(start - a.next)
	for s := range a.pkts {
		if seqBefore(s, start) {
			delete(a.pkts, s)
		}
	}
	a.next = start
	return lost
}

// depacketize joins the frame's payloads: Annex B for H.264, the frame
// for VP8.
func (a *assembler) depacketize(seqs []uint16) ([]byte, bool) {
	var out []byte
	switch a.codec {
	case "h264":
		d := &codecs.H264Packet{}
		for _, s := range seqs {
			b, err := d.Unmarshal(a.pkts[s].Payload)
			if err != nil {
				return nil, false
			}
			out = append(out, b...)
		}
	default:
		d := &codecs.VP8Packet{}
		for i, s := range seqs {
			b, err := d.Unmarshal(a.pkts[s].Payload)
			if err != nil {
				return nil, false
			}
			if i == 0 && d.S != 1 {
				return nil, false // not the frame's first partition
			}
			out = append(out, b...)
		}
	}
	return out, len(out) > 0
}

// isKeyframe: an H.264 access unit with an IDR slice, or a VP8 key frame.
func isKeyframe(codec string, data []byte) bool {
	if codec != "h264" {
		return len(data) > 0 && data[0]&1 == 0
	}
	for i := 0; i+3 < len(data); i++ {
		if data[i] != 0 || data[i+1] != 0 {
			continue
		}
		var nal byte
		switch {
		case data[i+2] == 1:
			nal = data[i+3]
		case data[i+2] == 0 && i+4 < len(data) && data[i+3] == 1:
			nal = data[i+4]
		default:
			continue
		}
		if nal&0x1f == 5 {
			return true
		}
	}
	return false
}

// seqBefore reports whether a comes before b, with wrap-around.
func seqBefore(a, b uint16) bool {
	return a != b && b-a < 0x8000
}
