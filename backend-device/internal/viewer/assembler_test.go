// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package viewer

import (
	"bytes"
	"testing"
	"time"

	"github.com/pion/rtp"
)

// h264Frame packetizes one access unit: an IDR or a P slice, in a FU-A
// split into n packets.
func h264Frame(seq uint16, ts uint32, idr bool, n int) []*rtp.Packet {
	nal := byte(0x41) // P slice
	if idr {
		nal = 0x65
	}
	var out []*rtp.Packet
	for i := 0; i < n; i++ {
		fuHeader := nal & 0x1f
		if i == 0 {
			fuHeader |= 0x80
		}
		if i == n-1 {
			fuHeader |= 0x40
		}
		payload := []byte{nal&0xe0 | 28, fuHeader, byte(i), byte(i), byte(i)}
		out = append(out, &rtp.Packet{Header: rtp.Header{SequenceNumber: seq + uint16(i), Timestamp: ts, Marker: i == n-1}, Payload: payload})
	}
	return out
}

func pushAll(a *assembler, pkts []*rtp.Packet, now time.Time) []Frame {
	var out []Frame
	for _, p := range pkts {
		f, _ := a.push(p, now)
		out = append(out, f...)
	}
	return out
}

func TestFramesComeWholeAndInOrder(t *testing.T) {
	a := newAssembler("h264")
	now := time.Now()
	key := h264Frame(100, 1000, true, 3)
	// Out of order inside the frame: nothing until the last one.
	if f := pushAll(a, []*rtp.Packet{key[0], key[2]}, now); len(f) != 0 {
		t.Fatal("frame with a gap")
	}
	f := pushAll(a, []*rtp.Packet{key[1]}, now)
	if len(f) != 1 || !f[0].Keyframe {
		t.Fatalf("%+v", f)
	}
	want := []byte{0, 0, 0, 1, 0x65, 0, 0, 0, 1, 1, 1, 2, 2, 2}
	if !bytes.Equal(f[0].Data, want) {
		t.Fatalf("data %x, want %x", f[0].Data, want)
	}
	if f := pushAll(a, h264Frame(103, 4000, false, 2), now); len(f) != 1 || f[0].Keyframe {
		t.Fatalf("%+v", f)
	}
}

func TestFramesBeforeTheFirstKeyframeAreDropped(t *testing.T) {
	a := newAssembler("h264")
	now := time.Now()
	if f := pushAll(a, h264Frame(1, 1, false, 2), now); len(f) != 0 {
		t.Fatal("a P frame before any keyframe")
	}
	if !a.wantKeyframe() {
		t.Fatal("no keyframe asked for")
	}
	if f := pushAll(a, h264Frame(3, 2, true, 2), now); len(f) != 1 {
		t.Fatal("keyframe lost")
	}
}

func TestALostPacketIsGivenUpForAKeyframe(t *testing.T) {
	a := newAssembler("h264")
	now := time.Now()
	pushAll(a, h264Frame(10, 1, true, 2), now)
	p := h264Frame(12, 2, false, 3)
	pushAll(a, []*rtp.Packet{p[0], p[2]}, now) // p[1] never comes
	next := h264Frame(15, 3, false, 2)
	if f := pushAll(a, next, now); len(f) != 0 {
		t.Fatal("a frame after a gap, before giving up")
	}
	// Later, the gap is given up: P frames are dropped until a keyframe.
	later := now.Add(assembleWait + time.Millisecond)
	f, lost := a.push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 17, Timestamp: 4, Marker: true}, Payload: []byte{0x41, 1}}, later)
	if len(f) != 0 || lost == 0 || !a.wantKeyframe() {
		t.Fatalf("frames %d lost %d", len(f), lost)
	}
	if f := pushAll(a, h264Frame(18, 5, true, 2), later); len(f) != 1 || !f[0].Keyframe {
		t.Fatalf("%+v", f)
	}
}

func TestSequenceNumbersWrap(t *testing.T) {
	a := newAssembler("h264")
	now := time.Now()
	if f := pushAll(a, h264Frame(65534, 1, true, 4), now); len(f) != 1 {
		t.Fatalf("%d frames across the wrap", len(f))
	}
	if !seqBefore(65535, 0) || seqBefore(0, 65535) {
		t.Fatal("seqBefore")
	}
}

func TestVP8Keyframes(t *testing.T) {
	a := newAssembler("vp8")
	// VP8 payload descriptor: S=1, PID=0; then the frame's first byte
	// (bit 0 clear: a key frame).
	f, _ := a.push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5, Timestamp: 1, Marker: true}, Payload: []byte{0x10, 0x00, 0x9d}}, time.Now())
	if len(f) != 1 || !f[0].Keyframe || !bytes.Equal(f[0].Data, []byte{0x00, 0x9d}) {
		t.Fatalf("%+v", f)
	}
}

func TestASteadyStreamIsNeverGivenUp(t *testing.T) {
	a := newAssembler("h264")
	now := time.Now()
	seq := uint16(0)
	// Each frame's first packet arrives with the last of the one before,
	// for longer than the wait: never a gap.
	pkts := h264Frame(seq, 0, true, 2)
	a.push(pkts[0], now)
	frames := 0
	for i := 1; i < 100; i++ {
		now = now.Add(20 * time.Millisecond)
		next := h264Frame(seq+2, uint32(i), false, 2)
		f, lost := a.push(next[0], now)
		f2, lost2 := a.push(pkts[1], now)
		if lost+lost2 > 0 {
			t.Fatalf("frame %d given up", i)
		}
		frames += len(f) + len(f2)
		seq += 2
		pkts = next
	}
	if frames != 99 {
		t.Fatalf("%d frames", frames)
	}
}
