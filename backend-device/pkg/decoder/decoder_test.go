// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package decoder

import (
	"bytes"
	"testing"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/encoder"
)

func TestSplitAnnexBAndAVCC(t *testing.T) {
	au := []byte{0, 0, 0, 1, 0x67, 1, 2, 0, 0, 1, 0x68, 3, 0, 0, 0, 1, 0x65, 4, 5, 6}
	nals := splitAnnexB(au)
	if len(nals) != 3 || !bytes.Equal(nals[0], []byte{0x67, 1, 2}) || !bytes.Equal(nals[1], []byte{0x68, 3}) || !bytes.Equal(nals[2], []byte{0x65, 4, 5, 6}) {
		t.Fatalf("nals %x", nals)
	}
	p := parseAccessUnit(au)
	if !p.key || !bytes.Equal(p.sps, []byte{0x67, 1, 2}) || !bytes.Equal(p.avcc, []byte{0, 0, 0, 4, 0x65, 4, 5, 6}) {
		t.Fatalf("%+v", p)
	}
}

func TestOpusRoundTrip(t *testing.T) {
	enc, err := encoder.NewOpus(1, 32000)
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	dec, err := NewOpus(1)
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()
	pcm := make([]int16, encoder.OpusFrameSamples)
	for i := range pcm {
		pcm[i] = int16((i % 48) * 400) // a 1 kHz sawtooth
	}
	var loud bool
	for range 5 {
		pkt, err := enc.Encode(pcm)
		if err != nil {
			t.Fatal(err)
		}
		out, err := dec.Decode(pkt)
		if err != nil || len(out) != encoder.OpusFrameSamples {
			t.Fatalf("%d samples, %v", len(out), err)
		}
		for _, v := range out {
			loud = loud || v > 4000
		}
	}
	if !loud {
		t.Fatal("the decoded sound is silent")
	}
	if out, err := dec.Decode(nil); err != nil || len(out) != encoder.OpusFrameSamples {
		t.Fatalf("concealment: %d samples, %v", len(out), err)
	}
}
