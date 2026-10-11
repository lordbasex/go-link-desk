// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"sync"
	"time"
)

// The congestion control's estimate alone is not to be trusted while the
// encoder sends far less than it (a small or still picture): seeing no
// congestion, it climbs to its maximum, and a sudden busy screen at that
// rate floods the path (a relay across a continent carries a few Mbps,
// not 30). Browsers bound their own estimate by what they actually send
// (libwebrtc's "application limited" state); sendCap does that for each
// viewer, and adds the loss its browser reports (client_report).
//
// The cap starts at capStartKbps, grows by capGrowth a second while the
// encoder fills most of it and nothing is lost, and drops at once to what
// got through when the viewer reports heavy loss.
const (
	capStartKbps  = 2500
	capGrowth     = 1.15 // per second: 2.5 to 20 Mbps in about 15 s
	capBusy       = 0.6  // the encoder fills at least this much of the cap to grow it
	capHeavyLoss  = 10.0 // % lost: drop to what got through
	capSomeLoss   = 3.0  // % lost: hold
	capAfterDrop  = 3 * time.Second
	capLossFresh  = 5 * time.Second
	capSentWindow = 2 * time.Second
	capGrowEvery  = time.Second
	capDropMargin = 0.85
)

type sentSample struct {
	at    time.Time
	bytes int64
}

// sendCap is one viewer's cap. Safe for concurrent use.
type sendCap struct {
	mu        sync.Mutex
	kbps      float64 // the cap (0: none yet)
	samples   []sentSample
	total     int64 // bytes sent so far
	loss      float64
	lossAt    time.Time
	grewAt    time.Time
	droppedAt time.Time
}

// sent counts bytes the encoder sent.
func (c *sendCap) sent(n int) {
	c.mu.Lock()
	c.total += int64(n)
	c.mu.Unlock()
}

// lost takes the viewer's reported video loss (%).
func (c *sendCap) lost(pct float64, now time.Time) {
	c.mu.Lock()
	c.loss, c.lossAt = pct, now
	c.mu.Unlock()
}

// sentKbps is what was sent over the last window (call under mu).
func (c *sendCap) sentKbps(now time.Time) float64 {
	c.samples = append(c.samples, sentSample{now, c.total})
	for len(c.samples) > 1 && now.Sub(c.samples[1].at) >= capSentWindow {
		c.samples = c.samples[1:]
	}
	first := c.samples[0]
	secs := now.Sub(first.at).Seconds()
	if secs < 0.5 {
		return -1 // not enough to tell yet
	}
	return float64(c.total-first.bytes) * 8 / 1000 / secs
}

// update moves the cap (every tick) and returns it in kbps.
func (c *sendCap) update(now time.Time) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.kbps == 0 {
		c.kbps = capStartKbps
		c.grewAt, c.droppedAt = now, now
	}
	sent := c.sentKbps(now)
	loss := 0.0
	if !c.lossAt.IsZero() && now.Sub(c.lossAt) < capLossFresh {
		loss = c.loss
	}
	switch {
	case loss >= capHeavyLoss && sent > 0:
		through := sent * (1 - loss/100) * capDropMargin
		if through < c.kbps {
			c.kbps = through
			c.droppedAt = now
		}
		c.lossAt = time.Time{} // one report, one drop
	case loss >= capSomeLoss:
		// hold
	case sent >= 0 && sent >= c.kbps*capBusy && now.Sub(c.grewAt) >= capGrowEvery && now.Sub(c.droppedAt) >= capAfterDrop:
		c.kbps *= capGrowth
		c.grewAt = now
	}
	c.kbps = max(c.kbps, float64(minKbps))
	return int(c.kbps)
}

// current is the cap now, without moving it (0: none yet).
func (c *sendCap) current() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return int(c.kbps)
}

// effectiveKbps is what a viewer's path is taken to carry: the congestion
// control's estimate (0: none yet), bounded by the cap (0: none yet).
func effectiveKbps(est, capKbps int) int {
	switch {
	case est <= 0:
		return capKbps
	case capKbps <= 0:
		return est
	}
	return min(est, capKbps)
}
