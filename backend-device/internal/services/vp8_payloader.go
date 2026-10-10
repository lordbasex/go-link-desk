// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"strings"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// vp8Payloader cuts VP8 frames into RTP payloads (RFC 7741) with a 15-bit
// PictureID on every frame, as browsers send it. Pion's own payloader
// leaves the PictureID out of the frame whose ID is 0 and writes IDs under
// 128 in 7 bits: every 32768 frames (9 min 6 s at 60 fps) the receiver met
// a frame with no PictureID in a stream that had one, lost track of the
// frame references and dropped every frame until the next keyframe (found
// in the room telemetry: a freeze every 546 s, with no lost packets).
type vp8Payloader struct {
	pictureID uint16
}

// vp8HeaderLen is the payload descriptor: X, I and a 15-bit PictureID.
const vp8HeaderLen = 4

// Payload fragments one encoded frame to fit mtu.
func (p *vp8Payloader) Payload(mtu uint16, payload []byte) [][]byte {
	room := int(mtu) - vp8HeaderLen
	if room <= 0 || len(payload) == 0 {
		return nil
	}
	var out [][]byte
	for i := 0; i < len(payload); i += room {
		n := min(room, len(payload)-i)
		b := make([]byte, vp8HeaderLen+n)
		b[0] = 0x80 // X: extended control bits follow
		if i == 0 {
			b[0] |= 0x10 // S: the start of a frame (partition 0)
		}
		b[1] = 0x80                             // I: a PictureID follows
		b[2] = 0x80 | byte(p.pictureID>>8&0x7F) // M: it is 15 bits long
		b[3] = byte(p.pictureID)                // its low byte
		copy(b[vp8HeaderLen:], payload[i:i+n])
		out = append(out, b)
	}
	p.pictureID = (p.pictureID + 1) & 0x7FFF
	return out
}

// newVideoTrack makes a video track; VP8 ones use vp8Payloader.
func newVideoTrack(c webrtc.RTPCodecCapability) (*webrtc.TrackLocalStaticSample, error) {
	if !strings.EqualFold(c.MimeType, webrtc.MimeTypeVP8) {
		return webrtc.NewTrackLocalStaticSample(c, "video", "go-link")
	}
	return webrtc.NewTrackLocalStaticSample(c, "video", "go-link", webrtc.WithPayloader(func(webrtc.RTPCodecCapability) (rtp.Payloader, error) {
		return &vp8Payloader{}, nil
	}))
}
