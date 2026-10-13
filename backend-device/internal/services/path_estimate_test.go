// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"testing"
	"time"
)

// The session seen on a LAN: GCC at 7 Mbps, the encoder sending 2.5 on a
// simpler scene, and GCC dropping to 85% of that and on down to its floor
// while the picture shrinks. The estimate holds.
func TestPathEstimateIgnoresDropsWhileTheEncoderSendsLittle(t *testing.T) {
	var p pathEstimate
	now := time.Unix(0, 0)
	p.update(7000, 6000, 0, now)
	for i, gcc := range []int{2123, 866, 642, 369, 300} {
		now = now.Add(time.Second)
		sent := []int{2500, 900, 500, 120, 100}[i]
		if got := p.update(gcc, sent, 0, now); got != 7000 {
			t.Fatalf("GCC at %d with %d kbps sent: %d, want 7000 held", gcc, sent, got)
		}
	}
}

// A drop while the encoder fills the estimate is the path's: it counts.
func TestPathEstimateFollowsDropsWhileTheEncoderFillsIt(t *testing.T) {
	var p pathEstimate
	now := time.Unix(0, 0)
	p.update(4000, 3600, 0, now)
	if got := p.update(2500, 3000, 0, now.Add(time.Second)); got != 2500 {
		t.Fatalf("%d, want 2500", got)
	}
}

// Loss makes a drop count even while the encoder sends little.
func TestPathEstimateFollowsDropsWithLoss(t *testing.T) {
	var p pathEstimate
	now := time.Unix(0, 0)
	p.update(4000, 3600, 0, now)
	if got := p.update(1000, 300, 5, now.Add(time.Second)); got != 1000 {
		t.Fatalf("%d, want 1000", got)
	}
}

// GCC above the estimate raises it.
func TestPathEstimateFollowsRises(t *testing.T) {
	var p pathEstimate
	now := time.Unix(0, 0)
	p.update(4000, 3600, 0, now)
	p.update(1000, 3600, 0, now.Add(time.Second))
	if got := p.update(5000, 3600, 0, now.Add(2*time.Second)); got != 5000 {
		t.Fatalf("%d, want 5000", got)
	}
}

// A low estimate with no loss is tried higher every probeEvery, up to
// probeUnderKbps, and a drop while the encoder fills it brings it back.
func TestPathEstimateProbesUpAfterAWhile(t *testing.T) {
	var p pathEstimate
	now := time.Unix(0, 0)
	p.update(300, 280, 0, now)
	if got := p.update(300, 100, 0, now.Add(probeEvery-time.Second)); got != 300 {
		t.Fatalf("before probeEvery: %d", got)
	}
	now = now.Add(probeEvery)
	if got := p.update(300, 100, 0, now); got != 450 {
		t.Fatalf("after probeEvery: %d, want 450", got)
	}
	for i := 0; i < 10; i++ {
		now = now.Add(probeEvery)
		p.update(300, 100, 0, now)
	}
	if got := p.current(); got != probeUnderKbps {
		t.Fatalf("after many probes: %d, want %d", got, probeUnderKbps)
	}
	// The path could not: GCC drops while the encoder fills the estimate.
	if got := p.update(2000, 3000, 0, now.Add(time.Second)); got != 2000 {
		t.Fatalf("a real drop after probing: %d, want 2000", got)
	}
}

// GCC left low by a quiet spell, still climbing back while the encoder
// fills the estimate, is not a drop; once its figure comes from that
// traffic, it is.
func TestPathEstimateWaitsForGCCToMeasureTheTraffic(t *testing.T) {
	var p pathEstimate
	now := time.Unix(0, 0)
	p.update(7000, 6000, 0, now)
	p.update(300, 100, 0, now.Add(time.Second)) // a quiet spell
	if got := p.update(400, 6000, 0, now.Add(2*time.Second)); got != 7000 {
		t.Fatalf("GCC at 400 climbing back under 6 Mbps: %d, want 7000", got)
	}
	// Congestion at 6 Mbps: GCC takes 85% of what arrived.
	if got := p.update(5100, 6000, 0, now.Add(3*time.Second)); got != 5100 {
		t.Fatalf("GCC at 5100 under 6 Mbps: %d, want 5100", got)
	}
}

// No probing with loss.
func TestPathEstimateDoesNotProbeWithLoss(t *testing.T) {
	var p pathEstimate
	now := time.Unix(0, 0)
	p.update(500, 400, 0, now)
	if got := p.update(500, 400, 2, now.Add(probeEvery)); got != 500 {
		t.Fatalf("%d, want 500", got)
	}
}
