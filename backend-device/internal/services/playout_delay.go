// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"strings"
	"sync"
	"sync/atomic"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// Playout delay: the device tells each browser how long it may hold the
// game's video before showing it (the playout-delay RTP header extension,
// minimum and maximum in 10 ms steps), as cloud gaming does. Left alone,
// Chrome sizes that wait from how unevenly frames arrive and almost never
// lowers it: in a two-hour room with no freeze it grew from 8 ms to 79 ms,
// so the controls got slower the longer people played. Every viewer starts
// at 0 (show each frame as soon as it is decoded); a viewer whose browser
// reports trouble gets a longer maximum, one step at a time, and goes back
// down after a clean spell (playoutLadder).

// playoutDelayURI is the header extension Chrome reads.
const playoutDelayURI = "http://www.webrtc.org/experiments/rtp-hdrext/playout-delay"

// playoutSteps are the maximum waits a viewer moves between, in ms.
var playoutSteps = []int{0, 40, 80, 160}

// playoutDelays finds each peer connection's interceptor by its id.
var playoutDelays = &playoutFactory{}

type playoutFactory struct {
	byPC sync.Map // pc id -> *playoutInterceptor
}

// NewInterceptor is called once for every peer connection.
func (f *playoutFactory) NewInterceptor(id string) (interceptor.Interceptor, error) {
	i := &playoutInterceptor{id: id, f: f}
	f.byPC.Store(id, i)
	return i, nil
}

// forPC returns the interceptor of a peer connection, or nil.
func (f *playoutFactory) forPC(pc *webrtc.PeerConnection) *playoutInterceptor {
	if pc == nil {
		return nil
	}
	if i, ok := f.byPC.Load(pc.ID()); ok {
		return i.(*playoutInterceptor)
	}
	return nil
}

// playoutInterceptor stamps the playout delay on one connection's video.
type playoutInterceptor struct {
	interceptor.NoOp
	id    string
	f     *playoutFactory
	maxMs atomic.Int32
}

// SetMax sets the longest wait the browser may add, in ms (0: none).
func (i *playoutInterceptor) SetMax(ms int) { i.maxMs.Store(int32(ms)) }

// Max is the current longest wait, in ms.
func (i *playoutInterceptor) Max() int { return int(i.maxMs.Load()) }

func (i *playoutInterceptor) BindLocalStream(info *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {
	if !strings.HasPrefix(strings.ToLower(info.MimeType), "video/") {
		return writer
	}
	id := 0
	for _, e := range info.RTPHeaderExtensions {
		if e.URI == playoutDelayURI {
			id = e.ID
		}
	}
	if id == 0 {
		return writer // the browser did not accept the extension
	}
	return interceptor.RTPWriterFunc(func(h *rtp.Header, payload []byte, a interceptor.Attributes) (int, error) {
		ext, err := (&rtp.PlayoutDelayExtension{MinDelay: 0, MaxDelay: uint16(i.maxMs.Load() / 10)}).Marshal()
		if err != nil {
			return writer.Write(h, payload, a)
		}
		out := *h
		out.Extensions = append([]rtp.Extension(nil), h.Extensions...)
		if err := out.SetExtension(uint8(id), ext); err != nil {
			return writer.Write(h, payload, a)
		}
		return writer.Write(&out, payload, a)
	})
}

func (i *playoutInterceptor) Close() error {
	i.f.byPC.Delete(i.id)
	return nil
}

// playoutLadder moves one viewer between playoutSteps from what its browser
// reports every two seconds: any trouble climbs a step at once, and 15
// clean reports in a row (30 s) step back down.
type playoutLadder struct {
	step  int
	clean int
}

// playoutCleanReports is how many clean reports bring the wait down a step.
const playoutCleanReports = 15

// note takes one client report and says whether the step changed and why.
func (l *playoutLadder) note(m map[string]float64) (changed bool, why string) {
	if m["hidden"] == 1 {
		return false, "" // a hidden tab measures nothing useful
	}
	switch {
	case m["freeze_ms"] > 0:
		why = "a freeze"
	case m["video_loss_pct"] >= 2:
		why = "lost video packets"
	case m["jitter_ms"] >= 15:
		why = "uneven packet arrival"
	}
	if why != "" {
		l.clean = 0
		if l.step < len(playoutSteps)-1 {
			l.step++
			return true, why
		}
		return false, ""
	}
	l.clean++
	if l.clean >= playoutCleanReports && l.step > 0 {
		l.step--
		l.clean = 0
		return true, "a clean spell"
	}
	return false, ""
}

// maxMs is the wait of the current step.
func (l *playoutLadder) maxMs() int { return playoutSteps[l.step] }
