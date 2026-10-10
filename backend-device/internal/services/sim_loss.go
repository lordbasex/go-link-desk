// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
)

// Simulated loss, for tests only: GOLINK_SIM_LOSS=<percent> makes the
// device drop that share of the RTP packets it sends (video and sound) to
// every connection but the first one that gets video, so a test can watch a
// clean player (the host, who comes in first) next to a lossy one in the
// same room. It sits closest to the network, after the retransmission
// buffer, so resends make up for lost packets as on a real network. Off
// unless the variable is set; never set it on a real device.

// simLossEnv names the variable.
const simLossEnv = "GOLINK_SIM_LOSS"

// simLossFromEnv reads the percentage, or 0 when unset or wrong.
func simLossFromEnv() float64 {
	v, err := strconv.ParseFloat(os.Getenv(simLossEnv), 64)
	if err != nil || v <= 0 || v >= 100 {
		return 0
	}
	return v
}

type simLossFactory struct {
	pct   float64
	mu    sync.Mutex
	first string // the id of the first connection that bound video
}

func (f *simLossFactory) NewInterceptor(id string) (interceptor.Interceptor, error) {
	return &simLossInterceptor{f: f, id: id}, nil
}

type simLossInterceptor struct {
	interceptor.NoOp
	f  *simLossFactory
	id string
}

// lossy says whether this connection's packets are dropped: every one but
// the first that bound a video stream.
func (i *simLossInterceptor) lossy(video bool) bool {
	i.f.mu.Lock()
	defer i.f.mu.Unlock()
	if i.f.first == "" && video {
		i.f.first = i.id
	}
	return i.f.first != "" && i.f.first != i.id
}

func (i *simLossInterceptor) BindLocalStream(info *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {
	if !i.lossy(strings.HasPrefix(strings.ToLower(info.MimeType), "video/")) {
		return writer
	}
	return interceptor.RTPWriterFunc(func(h *rtp.Header, payload []byte, a interceptor.Attributes) (int, error) {
		if rand.Float64()*100 < i.f.pct {
			return h.MarshalSize() + len(payload), nil // dropped on its way out
		}
		return writer.Write(h, payload, a)
	})
}
