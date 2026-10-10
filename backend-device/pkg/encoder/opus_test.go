// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package encoder

import "testing"

func TestOpusEncode(t *testing.T) {
	enc, err := NewOpus(1, 64000)
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	pkt, err := enc.Encode(make([]int16, OpusFrameSamples))
	if err != nil || len(pkt) == 0 {
		t.Fatalf("encode: %d bytes, %v", len(pkt), err)
	}
	if _, err := enc.Encode(make([]int16, 10)); err == nil {
		t.Fatal("short frame accepted")
	}
	if _, err := NewOpus(3, 64000); err == nil {
		t.Fatal("3 channels accepted")
	}
}

func TestOpusEncodesWithPacketLoss(t *testing.T) {
	o, err := NewOpus(2, 96000)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	pcm := make([]int16, OpusFrameSamples*2)
	for i := range pcm {
		pcm[i] = int16((i * 37) % 2000)
	}
	for _, pct := range []int{0, 12, 150, -5} {
		o.SetPacketLoss(pct)
		if pkt, err := o.Encode(pcm); err != nil || len(pkt) == 0 {
			t.Fatalf("loss %d%%: %v, %d bytes", pct, err, len(pkt))
		}
	}
	if o.PacketLoss() != 0 {
		t.Fatalf("a negative loss left %d%%", o.PacketLoss())
	}
}
