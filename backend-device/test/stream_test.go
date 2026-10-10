// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package test holds the host's integration tests: the real services
// against a browser played by Pion.
package test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/signalclient"
)

// browserPeer plays the web's role with Pion: it answers the host's
// offers (the first one and ICE restarts) and relays candidates back.
type browserPeer struct {
	id     string
	t      *testing.T
	pc     *webrtc.PeerConnection
	stream *services.StreamService

	mu     sync.Mutex
	queue  []webrtc.ICECandidateInit
	remote bool
	offers int
}

// relay is the Sender given to the stream: it delivers straight to the
// browser, standing in for signalhub.
type relay struct{ b *browserPeer }

func (r relay) Send(env signalclient.Envelope) error {
	go r.b.onSignal(env.Payload)
	return nil
}

func (b *browserPeer) onSignal(payload json.RawMessage) {
	var sig services.RTCSignal
	if err := json.Unmarshal(payload, &sig); err != nil {
		b.t.Error(err)
		return
	}
	switch sig.Kind {
	case "offer":
		b.mu.Lock()
		b.offers++
		b.mu.Unlock()
		if err := b.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: sig.SDP}); err != nil {
			b.t.Error(err)
			return
		}
		answer, err := b.pc.CreateAnswer(nil)
		if err != nil {
			b.t.Error(err)
			return
		}
		_ = b.pc.SetLocalDescription(answer)
		out, _ := json.Marshal(services.RTCSignal{Kind: "answer", SDP: answer.SDP})
		if err := b.stream.HandleSignal(b.id, out); err != nil {
			b.t.Error(err)
		}
		b.mu.Lock()
		b.remote = true
		queued := b.queue
		b.queue = nil
		b.mu.Unlock()
		for _, c := range queued {
			_ = b.pc.AddICECandidate(c)
		}
	case "candidate":
		b.mu.Lock()
		if !b.remote {
			b.queue = append(b.queue, *sig.Candidate)
			b.mu.Unlock()
			return
		}
		b.mu.Unlock()
		_ = b.pc.AddICECandidate(*sig.Candidate)
	}
}

// newBrowser builds a Pion peer that answers offers for id.
func newBrowser(t *testing.T, id string, stream *services.StreamService) *browserPeer {
	se := webrtc.SettingEngine{}
	se.SetIncludeLoopbackCandidate(true)
	pc, err := webrtc.NewAPI(webrtc.WithSettingEngine(se)).NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	b := &browserPeer{id: id, t: t, pc: pc, stream: stream}
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		init := c.ToJSON()
		out, _ := json.Marshal(services.RTCSignal{Kind: "candidate", Candidate: &init})
		_ = stream.HandleSignal(id, out)
	})
	return b
}

// ctrlMsg is a control channel message as the browser sees it.
type ctrlMsg struct {
	Type   string               `json:"type"`
	ID     uint64               `json:"id"`
	Host   services.SessionMeta `json:"host"`
	Source string               `json:"source"`
	FPS    float64              `json:"fps"`
	Width  int                  `json:"width"`
	Codec  string               `json:"codec"`
	Reason string               `json:"reason"`
}

func newStream(t *testing.T) (*services.StreamService, context.CancelFunc) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	stream, err := services.NewStreamService(services.StreamConfig{
		Width: 160, Height: 120, FPS: 30, BitrateKbps: 300, IncludeLoopback: true, Logger: logger,
		Welcome: services.SessionMeta{Name: "mac", OS: "darwin"},
	}, services.NewICEStore())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = stream.Run(ctx) }()
	return stream, cancel
}

func TestASessionStreamsToTheBrowser(t *testing.T) {
	stream, cancel := newStream(t)
	defer cancel()
	other := make(chan string, 10)
	stream.OnControl(func(peerID, kind string, _ []byte) { other <- peerID + ":" + kind })

	b := newBrowser(t, "viewer", stream)
	frames := make(chan struct{}, 100)
	b.pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if track.Codec().MimeType != webrtc.MimeTypeVP8 {
			t.Errorf("codec %s", track.Codec().MimeType)
		}
		for {
			if _, _, err := track.ReadRTP(); err != nil {
				return
			}
			select {
			case frames <- struct{}{}:
			default:
			}
		}
	})
	msgs := make(chan ctrlMsg, 50)
	controls := make(chan *webrtc.DataChannel, 1)
	b.pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() != "control" {
			t.Errorf("unexpected channel %q", dc.Label())
			return
		}
		if !dc.Ordered() || dc.MaxRetransmits() != nil {
			t.Error("the control channel must be ordered and reliable")
		}
		dc.OnOpen(func() { controls <- dc })
		dc.OnMessage(func(m webrtc.DataChannelMessage) {
			var got ctrlMsg
			_ = json.Unmarshal(m.Data, &got)
			if got.Type == "ping" {
				out, _ := json.Marshal(map[string]any{"type": "pong", "id": got.ID})
				_ = dc.SendText(string(out))
			}
			msgs <- got
		})
	})

	stream.SetSender(relay{b})
	if err := stream.AddViewer("viewer"); err != nil {
		t.Fatal(err)
	}

	// Video: RTP packets keep arriving.
	deadline := time.After(10 * time.Second)
	for got := 0; got < 20; got++ {
		select {
		case <-frames:
		case <-deadline:
			t.Fatalf("only %d RTP packets received", got)
		}
	}

	// The control channel says who the host is, then what it sends.
	welcome := waitForMsg(t, msgs, "welcome")
	if welcome.Host != (services.SessionMeta{Name: "mac", OS: "darwin"}) || welcome.Source != "test-pattern" {
		t.Fatalf("welcome %+v", welcome)
	}
	stats := waitForMsg(t, msgs, "stream_stats")
	if stats.Width != 160 || stats.Codec != "VP8" || stats.FPS <= 0 {
		t.Fatalf("stream_stats %+v", stats)
	}
	waitFor(t, func() bool { return stream.RTT("viewer") >= 0 })
	if st := stream.Stats(); st.VideoViewers != 1 || st.Width != 160 {
		t.Fatalf("stats %+v", st)
	}

	// Messages the stream does not handle go to the controllers.
	dc := <-controls
	_ = dc.SendText(`{"type":"session_end"}`)
	select {
	case got := <-other:
		if got != "viewer:session_end" {
			t.Fatalf("routed %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("control message not routed")
	}

	// The host hangs up: the last message arrives, then the connection
	// closes.
	end, _ := json.Marshal(map[string]string{"type": "session_end", "reason": models.EndHost})
	stream.Hangup("viewer", end)
	if got := waitForMsg(t, msgs, "session_end"); got.Reason != models.EndHost {
		t.Fatalf("session_end %+v", got)
	}
	if stream.ViewerCount() != 0 || stream.Stats().VideoViewers != 0 {
		t.Fatal("viewer not removed")
	}
	waitFor(t, func() bool {
		s := b.pc.ConnectionState()
		return s == webrtc.PeerConnectionStateClosed || s == webrtc.PeerConnectionStateDisconnected || s == webrtc.PeerConnectionStateFailed
	})
}

func TestEveryViewerHasItsOwnTrack(t *testing.T) {
	stream, cancel := newStream(t)
	defer cancel()
	a := newBrowser(t, "a", stream)
	b := newBrowser(t, "b", stream)
	stream.SetSender(multiRelay{"a": a, "b": b})
	ids := make(chan string, 2)
	for _, p := range []*browserPeer{a, b} {
		p.pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
			if _, _, err := track.ReadRTP(); err == nil {
				ids <- p.id + ":" + track.ID()
			}
		})
		if err := stream.AddViewer(p.id); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		select {
		case <-ids:
		case <-time.After(10 * time.Second):
			t.Fatal("a viewer got no video")
		}
	}
	stream.RemoveViewer("a")
	if stream.ViewerCount() != 1 {
		t.Fatal("removing one viewer touched the other")
	}
}

// multiRelay routes the stream's signals to the right fake browser.
type multiRelay map[string]*browserPeer

func (m multiRelay) Send(env signalclient.Envelope) error {
	if b := m[env.To]; b != nil {
		go b.onSignal(env.Payload)
	}
	return nil
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitForMsg(t *testing.T, ch chan ctrlMsg, typ string) ctrlMsg {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case m := <-ch:
			if m.Type == typ {
				return m
			}
		case <-deadline:
			t.Fatalf("no %s message", typ)
			return ctrlMsg{}
		}
	}
}
