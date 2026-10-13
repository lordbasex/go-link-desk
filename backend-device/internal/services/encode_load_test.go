// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"io"
	"log/slog"
	"testing"
	"time"

	screencap "github.com/lordbasex/go-link-desk/backend-device/pkg/capture"
)

// feedLoad runs loadStep over the shares, one a second, from rung.
func feedLoad(rung int, shares ...float64) int {
	hot, cool := 0, 0
	for _, s := range shares {
		rung, hot, cool = loadStep(rung, s, hot, cool, loadUpAfter)
	}
	return rung
}

func TestAnOverloadedEncoderStepsDown(t *testing.T) {
	top := len(loadLadder)
	if got := feedLoad(top, 0.9, 0.95); got != top {
		t.Errorf("two hot seconds: rung %d, want %d (no change yet)", got, top)
	}
	if got := feedLoad(top, 0.9, 0.95, 0.85); got != top-1 {
		t.Errorf("three hot seconds: rung %d, want %d", got, top-1)
	}
	// A cooler second in between starts the count again.
	if got := feedLoad(top, 0.9, 0.9, 0.6, 0.9, 0.9); got != top {
		t.Errorf("hot seconds not in a row: rung %d, want %d", got, top)
	}
	// It goes no lower than the lowest rung.
	hot := make([]float64, 40)
	for i := range hot {
		hot[i] = 1
	}
	if got := feedLoad(top, hot...); got != 0 {
		t.Errorf("a long overload: rung %d, want 0", got)
	}
}

func TestAnEncoderWithRoomClimbsOneRungAtATime(t *testing.T) {
	cool := make([]float64, loadUpAfter)
	for i := range cool {
		cool[i] = 0.2
	}
	if got := feedLoad(0, cool[:loadUpAfter-1]...); got != 0 {
		t.Errorf("not long enough: rung %d, want 0", got)
	}
	if got := feedLoad(0, cool...); got != 1 {
		t.Errorf("after %d cool seconds: rung %d, want 1", loadUpAfter, got)
	}
	// A middling share neither climbs nor drops.
	mid := make([]float64, 60)
	for i := range mid {
		mid[i] = 0.6
	}
	if got := feedLoad(1, mid...); got != 1 {
		t.Errorf("a middling load: rung %d, want 1", got)
	}
}

func TestTheLoadRungLimitsTheCapture(t *testing.T) {
	s, err := NewStreamService(StreamConfig{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}, NewICEStore())
	if err != nil {
		t.Fatal(err)
	}
	s.displays = []screencap.Display{{ID: 1, Width: 1920, Height: 1080, PointsWidth: 1920, PointsHeight: 1080}}
	s.want = VideoWant{Preset: PresetQuality}
	s.rung = len(bandwidthLadder)
	if c := s.Capture(); c.Width != 1920 || c.FPS != 60 {
		t.Fatalf("no limit: %dx%d at %d, want 1920 wide at 60", c.Width, c.Height, c.FPS)
	}
	s.loadRung = 1 // 1280 wide, 20 fps
	if c := s.Capture(); c.Width != 1280 || c.Height != 720 || c.FPS != 20 {
		t.Errorf("load rung 1: %dx%d at %d, want 1280x720 at 20", c.Width, c.Height, c.FPS)
	}
	// The tighter of the two ladders wins.
	s.rung = 0 // 640 wide, 15 fps
	if c := s.Capture(); c.Width != 640 || c.FPS != 15 {
		t.Errorf("both ladders: %dx%d at %d, want 640 wide at 15", c.Width, c.Height, c.FPS)
	}
}

func TestNoteEncodeMovesTheRungOncePerSecond(t *testing.T) {
	s, err := NewStreamService(StreamConfig{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}, NewICEStore())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(0, 0)
	changed := false
	// Encoding 90% of every second, for four seconds.
	for i := 0; i < 40; i++ {
		now = now.Add(100 * time.Millisecond)
		changed = s.noteEncode(90*time.Millisecond, now) || changed
	}
	if !changed || s.loadRung != len(loadLadder)-1 {
		t.Errorf("after four busy seconds: rung %d (changed %v), want %d", s.loadRung, changed, len(loadLadder)-1)
	}
}
