// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"testing"

	"github.com/pion/webrtc/v4"
)

func TestAnnouncedCandidates(t *testing.T) {
	mid := "0"
	idx := uint16(0)
	host := webrtc.ICECandidateInit{
		Candidate:     "candidate:1234 1 udp 2130706431 10.0.0.5 50000 typ host generation 0",
		SDPMid:        &mid,
		SDPMLineIndex: &idx,
	}
	got := announcedCandidates(host, []string{"192.168.1.50", "not-an-ip", "10.0.0.5", "203.0.113.50"})
	if len(got) != 2 {
		t.Fatalf("got %d candidates: %+v", len(got), got)
	}
	if got[0].Candidate != "candidate:4000000000 1 udp 2130706175 192.168.1.50 50000 typ host generation 0" {
		t.Fatalf("first = %q", got[0].Candidate)
	}
	if got[1].Candidate != "candidate:4000000003 1 udp 2130705407 203.0.113.50 50000 typ host generation 0" || *got[1].SDPMid != "0" {
		t.Fatalf("second = %q", got[1].Candidate)
	}
	if announcedCandidates(webrtc.ICECandidateInit{Candidate: "garbage"}, []string{"1.2.3.4"}) != nil {
		t.Fatal("a malformed candidate yields nothing")
	}
}
