// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"bytes"
	"testing"

	"github.com/pion/rtp/codecs"
)

// Every frame carries a 15-bit PictureID, 0 included, and the IDs wrap
// from 32767 to 0 (Pion's payloader dropped the ID of frame 0).
func TestVP8PayloaderPictureIDAlwaysPresent(t *testing.T) {
	p := &vp8Payloader{}
	frame := bytes.Repeat([]byte{0xAB}, 3000)
	for n := 0; n < 32768+300; n++ {
		want := uint16(n & 0x7FFF)
		payloads := p.Payload(1200, frame)
		if len(payloads) != 3 {
			t.Fatalf("frame %d: %d payloads", n, len(payloads))
		}
		var got []byte
		for i, b := range payloads {
			var d codecs.VP8Packet
			body, err := d.Unmarshal(b)
			if err != nil {
				t.Fatal(err)
			}
			if d.I != 1 || d.PictureID != want {
				t.Fatalf("frame %d packet %d: I=%d PictureID=%d, want %d", n, i, d.I, d.PictureID, want)
			}
			if (d.S == 1) != (i == 0) {
				t.Fatalf("frame %d packet %d: S=%d", n, i, d.S)
			}
			if len(b) > 1200 {
				t.Fatalf("packet of %d bytes", len(b))
			}
			got = append(got, body...)
		}
		if !bytes.Equal(got, frame) {
			t.Fatalf("frame %d does not rebuild", n)
		}
	}
}
