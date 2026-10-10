// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"testing"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
)

type countWriter struct{ n int }

func (c *countWriter) Write(*rtp.Header, []byte, interceptor.Attributes) (int, error) {
	c.n++
	return 0, nil
}

// The first connection that gets video stays clean; the others lose about
// the share asked for, sound included.
func TestSimLossSparesTheFirstViewer(t *testing.T) {
	f := &simLossFactory{pct: 20}
	send := func(id, mime string) int {
		in, _ := f.NewInterceptor(id)
		var w countWriter
		out := in.BindLocalStream(&interceptor.StreamInfo{MimeType: mime}, &w)
		for range 5000 {
			_, _ = out.Write(&rtp.Header{}, make([]byte, 10), nil)
		}
		return w.n
	}
	if n := send("host", "audio/opus"); n != 5000 {
		t.Fatalf("sound before any video lost %d packets", 5000-n)
	}
	if n := send("host", "video/VP8"); n != 5000 {
		t.Fatalf("the first viewer lost %d packets", 5000-n)
	}
	for _, mime := range []string{"video/VP8", "audio/opus"} {
		if n := send("guest", mime); n < 3700 || n > 4300 {
			t.Fatalf("%s: the guest kept %d of 5000 packets, want about 4000", mime, n)
		}
	}
	t.Setenv(simLossEnv, "")
	if simLossFromEnv() != 0 {
		t.Fatal("loss without the variable")
	}
	t.Setenv(simLossEnv, "7.5")
	if simLossFromEnv() != 7.5 {
		t.Fatal("did not read 7.5")
	}
}
