// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"sync"

	"github.com/pion/interceptor/pkg/cc"
	"github.com/pion/interceptor/pkg/gcc"
	"github.com/pion/webrtc/v4"
)

// Bandwidth estimation: Google Congestion Control on the sending side,
// from the browser's transport-wide feedback (TWCC). It estimates what
// each viewer's path carries, and the encoder follows it, under the
// preset's ceiling. go-link sends at a fixed rate and only steps down on
// loss; a desktop needs more (a 4K screen at 20 Mbps on a LAN, a laptop on
// 4G at 1).
const (
	bweStartBps = 4_000_000
	bweMinBps   = 300_000
	bweMaxBps   = 30_000_000
)

// estimators finds each peer connection's estimator by its id (the cc
// interceptor is created with the API, before any stream exists).
var estimators sync.Map // pc id -> cc.BandwidthEstimator

// newCongestionControl builds the interceptor. It paces nothing: Pion's
// leaky-bucket pacer would hold frames back, and a desktop session wants
// each frame on the wire at once; the encoder's rate is what follows the
// estimate.
func newCongestionControl() (*cc.InterceptorFactory, error) {
	f, err := cc.NewInterceptor(func() (cc.BandwidthEstimator, error) {
		return gcc.NewSendSideBWE(
			gcc.SendSideBWEInitialBitrate(bweStartBps),
			gcc.SendSideBWEMinBitrate(bweMinBps),
			gcc.SendSideBWEMaxBitrate(bweMaxBps),
			gcc.SendSideBWEPacer(gcc.NewNoOpPacer()),
		)
	})
	if err != nil {
		return nil, err
	}
	f.OnNewPeerConnection(func(id string, e cc.BandwidthEstimator) { estimators.Store(id, e) })
	return f, nil
}

// estimateKbps is a connection's current estimate (0: none yet).
func estimateKbps(pc *webrtc.PeerConnection) int {
	if pc == nil {
		return 0
	}
	if e, ok := estimators.Load(pc.ID()); ok {
		return e.(cc.BandwidthEstimator).GetTargetBitrate() / 1000
	}
	return 0
}

// forgetEstimate drops a closed connection's estimator.
func forgetEstimate(pc *webrtc.PeerConnection) {
	if pc != nil {
		estimators.Delete(pc.ID())
	}
}
