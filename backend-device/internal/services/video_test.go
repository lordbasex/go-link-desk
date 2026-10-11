// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"testing"

	screencap "github.com/lordbasex/go-link-desk/backend-device/pkg/capture"
)

func TestCaptureSize(t *testing.T) {
	retina := screencap.Display{Width: 2560, Height: 1600, PointsWidth: 1440, PointsHeight: 900}
	plain := screencap.Display{Width: 1920, Height: 1080, PointsWidth: 1920, PointsHeight: 1080}
	big := screencap.Display{Width: 5120, Height: 2880, PointsWidth: 2560, PointsHeight: 1440}
	for _, tc := range []struct {
		name string
		d    screencap.Display
		want VideoWant
		w, h int
	}{
		{"balanced, no window yet: the points", retina, VideoWant{Preset: PresetBalanced}, 1440, 900},
		{"balanced keeps the pixels for a window a little smaller", plain, VideoWant{Preset: PresetBalanced, Width: 1920, Height: 1027}, 1920, 1080},
		{"balanced fits a much smaller window", plain, VideoWant{Preset: PresetBalanced, Width: 1280, Height: 800}, 1280, 720},
		{"balanced caps the width", big, VideoWant{Preset: PresetBalanced}, 1920, 1080},
		{"retina sends the pixels", retina, VideoWant{Preset: PresetBalanced, Retina: true}, 1920, 1200},
		{"quality: the pixels, whatever the window", plain, VideoWant{Preset: PresetQuality, Width: 1000, Height: 600}, 1920, 1080},
		{"quality on Retina: every pixel", retina, VideoWant{Preset: PresetQuality}, 2560, 1600},
		{"speed caps lower", plain, VideoWant{Preset: PresetSpeed}, 1280, 720},
		{"a tiny window keeps it readable", plain, VideoWant{Preset: PresetBalanced, Width: 200, Height: 100}, 568, 320},
		{"an odd size becomes even", screencap.Display{Width: 1366, Height: 769, PointsWidth: 1366, PointsHeight: 769}, VideoWant{}, 1366, 768},
	} {
		if w, h := captureSize(tc.d, tc.want); w != tc.w || h != tc.h {
			t.Errorf("%s: %dx%d, want %dx%d", tc.name, w, h, tc.w, tc.h)
		}
	}
}

func TestPickCodec(t *testing.T) {
	for _, tc := range []struct {
		browser []string
		h264    bool
		want    string
	}{
		{[]string{"vp8", "h264", "h264-high"}, true, CodecH264High},
		{[]string{"vp8", "h264"}, true, CodecH264},
		{[]string{"vp8", "h264-high"}, false, CodecVP8},
		{nil, true, CodecVP8},
	} {
		if got := pickCodec(tc.browser, tc.h264); got != tc.want {
			t.Errorf("pickCodec(%v, %v) = %s, want %s", tc.browser, tc.h264, got, tc.want)
		}
	}
}

func TestTheBandwidthLadder(t *testing.T) {
	none := len(bandwidthLadder)
	for _, tc := range []struct {
		est, from, want int
	}{
		{0, none, none},     // no estimate yet
		{20000, none, none}, // plenty
		{2000, none, 3},     // 30 fps
		{1000, none, 2},     // 1280 wide
		{300, none, 0},      // 640 wide, straight down
		{450, 0, 0},         // just over a rung: not up yet
		{600, 0, 1},         // a third over 400: up one
		{30000, 0, none},    // all the way back up
		{3000, 3, 3},        // 2500 * 4/3 = 3333: stays
	} {
		if got := ladderRung(tc.est, tc.from); got != tc.want {
			t.Errorf("ladderRung(%d, %d) = %d, want %d", tc.est, tc.from, got, tc.want)
		}
	}
}
