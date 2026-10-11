// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"testing"
	"time"
)

// feed sends kbps for d, ticking every 500 ms, and returns the last cap.
func feed(c *sendCap, t0 *time.Time, kbps float64, d time.Duration) int {
	var got int
	for step := time.Duration(0); step < d; step += 500 * time.Millisecond {
		c.sent(int(kbps * 1000 / 8 / 2))
		*t0 = t0.Add(500 * time.Millisecond)
		got = c.update(*t0)
	}
	return got
}

func TestTheCapGrowsOnlyWhileTheEncoderFillsIt(t *testing.T) {
	c := &sendCap{}
	now := time.Unix(0, 0)
	c.update(now)
	// A still screen sends little: no reason to believe in more.
	if got := feed(c, &now, 100, 20*time.Second); got != capStartKbps {
		t.Fatalf("cap %d after a still screen", got)
	}
	// A busy screen fills it: it grows, by steps, not at once.
	got := feed(c, &now, 1e6, 3*time.Second)
	if got <= capStartKbps || got > capStartKbps*2 {
		t.Fatalf("cap %d after 3 s busy", got)
	}
	if got := feed(c, &now, 1e6, 20*time.Second); got < 20000 {
		t.Fatalf("cap %d after 23 s busy on a clean path", got)
	}
}

func TestHeavyLossDropsTheCapToWhatGotThrough(t *testing.T) {
	c := &sendCap{}
	now := time.Unix(0, 0)
	c.update(now)
	feed(c, &now, 1e6, 20*time.Second) // grown well past 10 Mbps
	feed(c, &now, 5000, 2*time.Second) // then 5 Mbps go out...
	c.lost(40, now)                    // ...and 40% of it is lost
	got := feed(c, &now, 5000, 500*time.Millisecond)
	// 5000 kbps sent, 40% lost: about 2550 got through.
	if got > 3000 || got < 2000 {
		t.Fatalf("cap %d after 40%% loss at 5 Mbps", got)
	}
	// It holds a while before growing again.
	if again := feed(c, &now, 1e6, 2*time.Second); again != got {
		t.Fatalf("cap grew from %d to %d right after a drop", got, again)
	}
	// Some loss: hold.
	c.lost(5, now)
	if held := feed(c, &now, 1e6, time.Second); held != got {
		t.Fatalf("cap moved from %d to %d with 5%% loss", got, held)
	}
}
