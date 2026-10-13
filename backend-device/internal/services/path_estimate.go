// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"sync"
	"time"
)

// GCC (congestion.go) lowers its estimate to 85% of what it received when
// it sees the delay grow, and raises it to at most 1.5 times that. While
// the encoder sends far less than the path carries (a still or simple
// picture, a GPU encoder that keeps under its target), one late burst on
// Wi-Fi brings the estimate down to what that quiet picture needed; the
// ladder then makes the picture smaller, the encoder sends even less, and
// the estimate cannot climb back. Seen on a LAN: 1080p at 6 Mbps down to
// 640x360 at 300 kbps in 11 seconds, for good, without a packet lost.
//
// pathEstimate is GCC's estimate with that undone. GCC under it counts
// when the viewer reports loss, or when the encoder was sending most of
// it and GCC's figure comes from that traffic (at least half of it: one
// left low by a quiet spell is still climbing back, 8% a second); else
// it holds. An estimate that has held below probeUnderKbps with no loss
// is tried probeStep higher every probeEvery, as browsers probe with
// padding: a path that cannot carry it says so as the encoder fills it.
const (
	pathBusy       = 0.6 // of the estimate: the encoder fills it
	pathMeasured   = 0.5 // of what was sent: GCC's figure comes from it
	pathLoss       = capSomeLoss
	probeEvery     = 30 * time.Second
	probeStep      = 1.5
	probeUnderKbps = capStartKbps
)

// pathEstimate is one viewer's. Safe for concurrent use.
type pathEstimate struct {
	mu     sync.Mutex
	kbps   int // what the path is taken to carry (0: no estimate yet)
	heldAt time.Time
}

// update takes GCC's estimate, what the encoder sent over the last
// moments (-1: not known yet) and the viewer's reported loss (%), and
// returns the estimate.
func (p *pathEstimate) update(gcc, sentKbps int, loss float64, now time.Time) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if gcc <= 0 {
		return p.kbps
	}
	if p.kbps == 0 {
		p.kbps, p.heldAt = gcc, now
		return p.kbps
	}
	sent := float64(sentKbps)
	measured := sentKbps < 0 || sent >= pathBusy*float64(p.kbps) && float64(gcc) >= pathMeasured*sent
	switch {
	case gcc > p.kbps:
		p.kbps = gcc
		p.heldAt = now
	case gcc < p.kbps && (measured || loss >= pathLoss):
		p.kbps = gcc
		p.heldAt = now
	}
	if p.kbps < probeUnderKbps && loss < 1 && now.Sub(p.heldAt) >= probeEvery {
		p.kbps = min(int(float64(p.kbps)*probeStep), probeUnderKbps)
		p.heldAt = now
	}
	return p.kbps
}

// current is the estimate now, without moving it (0: none yet).
func (p *pathEstimate) current() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.kbps
}
