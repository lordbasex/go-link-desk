// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"encoding/json"
	"io"
	"log/slog"
	"regexp"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/signalclient"
)

var ufrag = regexp.MustCompile(`a=ice-ufrag:(\S+)`)

// answerOffer waits for the next offer, answers it from browser (without
// candidates: the connection never comes up) and returns its ICE
// username.
func answerOffer(t *testing.T, s *StreamService, snd *fakeSender, browser *webrtc.PeerConnection) string {
	t.Helper()
	for {
		env := next(t, snd)
		var sig RTCSignal
		if json.Unmarshal(env.Payload, &sig) != nil || sig.Kind != "offer" {
			continue // candidates
		}
		m := ufrag.FindStringSubmatch(sig.SDP)
		if m == nil {
			t.Fatalf("offer without ice-ufrag: %s", sig.SDP)
		}
		if err := browser.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: sig.SDP}); err != nil {
			t.Fatal(err)
		}
		answer, err := browser.CreateAnswer(nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := browser.SetLocalDescription(answer); err != nil {
			t.Fatal(err)
		}
		out, _ := json.Marshal(RTCSignal{Kind: "answer", SDP: answer.SDP})
		if err := s.HandleSignal(env.To, out); err != nil {
			t.Fatal(err)
		}
		return m[1]
	}
}

func TestAnICERestartOffersNewCredentialsThenGivesUp(t *testing.T) {
	s, err := NewStreamService(StreamConfig{IncludeLoopback: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}, NewICEStore())
	if err != nil {
		t.Fatal(err)
	}
	snd := &fakeSender{ch: make(chan signalclient.Envelope, 200)}
	s.SetSender(snd)
	closed := make(chan string, 1)
	s.OnClosed(func(peer, reason string) { closed <- peer + ":" + reason })
	if err := s.AddViewer("P"); err != nil {
		t.Fatal(err)
	}
	defer s.RemoveViewer("P")
	browser, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	first := answerOffer(t, s, snd, browser)

	s.mu.Lock()
	v := s.viewers["P"]
	s.mu.Unlock()
	seen := map[string]bool{first: true}
	for range maxRestarts {
		s.restartICE(v)
		u := answerOffer(t, s, snd, browser)
		if seen[u] {
			t.Fatalf("restart reused the ICE username %q", u)
		}
		seen[u] = true
	}
	// One more failure ends the session.
	s.restartICE(v)
	select {
	case got := <-closed:
		if got != "P:"+models.EndNetwork {
			t.Fatalf("closed %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the stream never gave up")
	}
	if s.ViewerCount() != 0 {
		t.Fatal("viewer kept")
	}
}
