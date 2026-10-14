// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"net/netip"
	"testing"

	"github.com/pion/webrtc/v4"
)

func TestPortAnnouncer(t *testing.T) {
	mid := "0"
	idx := uint16(0)
	host := webrtc.ICECandidateInit{
		Candidate:     "candidate:1234 1 udp 2130706431 10.0.0.5 50000 typ host generation 0",
		SDPMid:        &mid,
		SDPMLineIndex: &idx,
	}
	ap := netip.MustParseAddrPort
	a := &portAnnouncer{}
	// Before the host candidate: kept for later. Its own address, an IPv6
	// one and a repeated one give nothing.
	if got := a.at(announced([]string{"192.168.1.50", "not-an-ip", "10.0.0.5"}, 50000)...); got != nil {
		t.Fatalf("before the host candidate: %+v", got)
	}
	got := a.host(host)
	if len(got) != 1 || got[0].Candidate != "candidate:4000000001 1 udp 2130706175 192.168.1.50 50000 typ host generation 0" || *got[0].SDPMid != "0" {
		t.Fatalf("pending: %+v", got)
	}
	// The public address STUN saw, on the fixed port, and the router's
	// mapping on another port.
	got = a.at(ap("203.0.113.50:50000"), ap("[2001:db8::1]:50000"), ap("192.168.1.50:50000"), ap("203.0.113.50:50002"))
	if len(got) != 2 ||
		got[0].Candidate != "candidate:4000000002 1 udp 2130705919 203.0.113.50 50000 typ host generation 0" ||
		got[1].Candidate != "candidate:4000000003 1 udp 2130705663 203.0.113.50 50002 typ host generation 0" {
		t.Fatalf("later: %+v", got)
	}
	if a.host(host) != nil {
		t.Fatal("a second host candidate")
	}
	if (&portAnnouncer{}).host(webrtc.ICECandidateInit{Candidate: "garbage"}) != nil {
		t.Fatal("a malformed candidate yields nothing")
	}
}
