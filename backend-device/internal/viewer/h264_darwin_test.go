// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

package viewer

import (
	"testing"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
)

// On a Mac the host encodes H.264 with VideoToolbox for a viewer that
// takes it: frames come as whole access units, the first an IDR.
func TestAMacSendsH264ToAViewerThatTakesIt(t *testing.T) {
	_, hub, rec := startSession(t, Door{Kind: DoorJoin, Code: "915355636"}, models.Permissions{}, func(h *fakeHub) {
		h.codecs = []string{"h264-high", "h264"}
		h.accept()
	})
	_ = hub
	waitFor(t, "frames", func() bool { rec.mu.Lock(); defer rec.mu.Unlock(); return len(rec.frames) >= 10 })
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if f := rec.frames[0]; f.Codec != "h264" || !f.Keyframe {
		t.Fatalf("first frame: %s, key %v", f.Codec, f.Keyframe)
	}
	for _, f := range rec.frames {
		if len(f.Data) < 4 || f.Data[0] != 0 || f.Data[1] != 0 {
			t.Fatal("a frame is not Annex B")
		}
	}
}
