// Stage 0 of go-link Desk on LordOS (throwaway): frames arrive already
// encoded from the session, so they skip VideoFrame's per-viewer encoder.

package services

import (
	"fmt"
	"time"
)

// EncodedWant is what the viewers need from an encoded source.
type EncodedWant struct {
	Viewers int
	Codec   string // the first viewer's (CodecH264 or CodecVP8), "" with none
	Kbps    int
	NeedKey bool
}

// EncodedWant reports the current viewers' needs.
func (s *StreamService) EncodedWant() EncodedWant {
	s.mu.Lock()
	defer s.mu.Unlock()
	var w EncodedWant
	for _, v := range s.viewers {
		w.Viewers++
		if w.Codec == "" || int(v.kbps.Load()) < w.Kbps {
			w.Kbps = int(v.kbps.Load())
		}
		if w.Codec == "" {
			w.Codec = v.codec
		}
		if v.keyframe.Load() {
			w.NeedKey = true
		}
	}
	return w
}

// EncodedFrame sends one encoded frame to every viewer on that codec.
func (s *StreamService) EncodedFrame(codec string, key bool, pts time.Duration, data []byte, w, h int) {
	for _, v := range s.viewerList() {
		if v.codec != codec || v.out == nil {
			continue
		}
		if key {
			v.keyframe.Store(false)
		}
		s.mu.Lock()
		s.sizes[v.id] = [2]int{w, h}
		s.mu.Unlock()
		v.out.write(data, pts)
	}
}

// PathInfo describes the ICE pair each viewer is using.
func (s *StreamService) PathInfo() []string {
	var out []string
	for _, v := range s.viewerList() {
		st := v.pc.SCTP()
		if st == nil || st.Transport() == nil || st.Transport().ICETransport() == nil {
			continue
		}
		p, err := st.Transport().ICETransport().GetSelectedCandidatePair()
		if err != nil || p == nil {
			continue
		}
		out = append(out, fmt.Sprintf("local=%s/%s remote=%s/%s", p.Local.Typ, p.Local.Protocol, p.Remote.Typ, p.Remote.Protocol))
	}
	return out
}
