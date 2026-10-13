// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import "time"

// The encode load ladder: what a computer too slow for the picture still
// encodes in time. Every second the stream measures the share of it spent
// encoding. When encoding takes most of the time, frames come late, and
// libvpx, out of time, goes to its fastest, coarsest setting: the picture
// blurs and its frames grow far past the bitrate (a 1.8 GHz Xeon core sent
// 22 Mbps for 8 asked at 1280x800), which the congestion control then
// takes for a full network and drops to 640x400. Stepping down the frame
// rate and then the size before that keeps the picture sharp and the
// bitrate where it was asked. A computer with room to spare climbs back,
// one rung at a time, slower each time a climb did not hold.
var loadLadder = []struct {
	maxWidth, fps int
}{
	{960, 15},
	{1280, 20},
	{1600, 24},
	{0, 30}, // the full size, at 30 frames a second
}

const (
	// loadHigh is the share of the time spent encoding that is too much:
	// past it, frames start to come late.
	loadHigh = 0.8
	// loadLow is the share that leaves room for the next rung up.
	loadLow = 0.45
	// loadDownAfter and loadUpAfter are the seconds in a row it takes.
	loadDownAfter = 3
	loadUpAfter   = 15
	// loadUpMax is the longest wait to climb, after climbs that failed.
	loadUpMax = 120
)

// encodeLoad is the stream's watch (VideoFrame's goroutine only).
type encodeLoad struct {
	busy    time.Duration // encoding since the second started
	start   time.Time     // when the second started
	hot     int           // seconds in a row over loadHigh
	cool    int           // seconds in a row under loadLow
	upAfter int           // seconds of cool it takes to climb now
	upAt    time.Time     // the last climb
}

// loadStep moves the rung (len(loadLadder): no limit) by one second's
// share: down at once after loadDownAfter hot seconds, up one after
// upAfter cool ones. It returns the new rung and the updated counts.
func loadStep(rung int, share float64, hot, cool, upAfter int) (int, int, int) {
	switch {
	case share >= loadHigh:
		hot, cool = hot+1, 0
	case share <= loadLow:
		hot, cool = 0, cool+1
	default:
		hot, cool = 0, 0
	}
	if hot >= loadDownAfter && rung > 0 {
		return rung - 1, 0, 0
	}
	if cool >= upAfter && rung < len(loadLadder) {
		return rung + 1, 0, 0
	}
	return rung, hot, cool
}

// noteEncode adds one frame's encoding time and, once a second, moves the
// load rung. It reports whether the rung changed.
func (s *StreamService) noteEncode(took time.Duration, now time.Time) bool {
	l := &s.load
	if l.start.IsZero() {
		l.start = now
	}
	l.busy += took
	elapsed := now.Sub(l.start)
	if elapsed < time.Second {
		return false
	}
	share := min(float64(l.busy)/float64(elapsed), 1)
	l.busy, l.start = 0, now
	if l.upAfter == 0 {
		l.upAfter = loadUpAfter
	}
	s.mu.Lock()
	before := s.loadRung
	after, hot, cool := loadStep(before, share, l.hot, l.cool, l.upAfter)
	s.loadRung = after
	s.mu.Unlock()
	l.hot, l.cool = hot, cool
	switch {
	case after > before:
		l.upAt = now
	case after < before && !l.upAt.IsZero() && now.Sub(l.upAt) < 30*time.Second:
		// The last climb did not hold: wait longer before the next.
		l.upAfter = min(l.upAfter*2, loadUpMax)
	}
	if after == before {
		return false
	}
	s.log.Info("encode load", "share", round1(share*100), "rung", after)
	return true
}
