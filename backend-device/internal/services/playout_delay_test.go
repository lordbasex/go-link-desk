// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"testing"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
)

func TestPlayoutLadderClimbsAtOnceAndComesDownSlowly(t *testing.T) {
	var l playoutLadder
	clean := map[string]float64{"jitter_ms": 2, "video_loss_pct": 0}
	if changed, _ := l.note(clean); changed || l.maxMs() != 0 {
		t.Fatalf("a clean start moved to %d ms", l.maxMs())
	}
	for _, bad := range []map[string]float64{{"freeze_ms": 300}, {"video_loss_pct": 3}, {"jitter_ms": 20}} {
		if changed, why := l.note(bad); !changed || why == "" {
			t.Fatalf("%v did not climb", bad)
		}
	}
	if l.maxMs() != 160 {
		t.Fatalf("three troubles: %d ms, want 160", l.maxMs())
	}
	if changed, _ := l.note(map[string]float64{"freeze_ms": 300}); changed || l.maxMs() != 160 {
		t.Fatal("climbed past the top step")
	}
	// A hidden tab neither climbs nor counts as clean.
	for range 40 {
		l.note(map[string]float64{"hidden": 1, "jitter_ms": 50})
	}
	if l.maxMs() != 160 {
		t.Fatalf("a hidden tab moved the wait to %d ms", l.maxMs())
	}
	for i := 1; i < playoutCleanReports; i++ {
		if changed, _ := l.note(clean); changed {
			t.Fatalf("came down after %d clean reports", i)
		}
	}
	if changed, _ := l.note(clean); !changed || l.maxMs() != 80 {
		t.Fatalf("after 30 s clean: %d ms, want 80", l.maxMs())
	}
}

type capture struct{ h *rtp.Header }

func (c *capture) Write(h *rtp.Header, _ []byte, _ interceptor.Attributes) (int, error) {
	c.h = h
	return 0, nil
}

func TestPlayoutInterceptorStampsVideoOnly(t *testing.T) {
	f := &playoutFactory{}
	in, _ := f.NewInterceptor("pc1")
	pi := in.(*playoutInterceptor)
	exts := []interceptor.RTPHeaderExtension{{URI: playoutDelayURI, ID: 5}}

	var got capture
	w := pi.BindLocalStream(&interceptor.StreamInfo{MimeType: "video/VP8", RTPHeaderExtensions: exts}, &got)
	pi.SetMax(80)
	orig := &rtp.Header{Version: 2}
	if _, err := w.Write(orig, nil, nil); err != nil {
		t.Fatal(err)
	}
	var d rtp.PlayoutDelayExtension
	if err := d.Unmarshal(got.h.GetExtension(5)); err != nil {
		t.Fatal(err)
	}
	if d.MinDelay != 0 || d.MaxDelay != 8 {
		t.Fatalf("min %d max %d (10 ms units), want 0 and 8", d.MinDelay, d.MaxDelay)
	}
	if orig.Extension {
		t.Fatal("the caller's header was changed")
	}

	got = capture{}
	w = pi.BindLocalStream(&interceptor.StreamInfo{MimeType: "audio/opus", RTPHeaderExtensions: exts}, &got)
	_, _ = w.Write(&rtp.Header{Version: 2}, nil, nil)
	if got.h.Extension {
		t.Fatal("audio got the playout delay")
	}
	got = capture{}
	w = pi.BindLocalStream(&interceptor.StreamInfo{MimeType: "video/VP8"}, &got)
	_, _ = w.Write(&rtp.Header{Version: 2}, nil, nil)
	if got.h.Extension {
		t.Fatal("stamped an extension the browser did not accept")
	}
	_ = pi.Close()
	if _, ok := f.byPC.Load("pc1"); ok {
		t.Fatal("a closed connection stays in the map")
	}
}
