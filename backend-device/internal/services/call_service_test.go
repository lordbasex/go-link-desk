// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func newCall(t *testing.T) (*CallService, *StreamService, chan CallStatus) {
	t.Helper()
	s, err := NewStreamService(StreamConfig{IncludeLoopback: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}, NewICEStore())
	if err != nil {
		t.Fatal(err)
	}
	s.SetSender(newFakeSender())
	t.Cleanup(s.CloseAll)
	if err := s.AddViewer("P", nil); err != nil {
		t.Fatal(err)
	}
	c := NewCallService(s, TestCallMedia{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	states := make(chan CallStatus, 32)
	c.OnStatus(func(st CallStatus) { states <- st })
	c.SetPeer("P", "Ana")
	t.Cleanup(c.End)
	return c, s, states
}

func waitState(t *testing.T, states chan CallStatus, want string) CallStatus {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case st := <-states:
			if st.State == want {
				return st
			}
		case <-deadline:
			t.Fatalf("never %s", want)
		}
	}
}

func TestACallNeedsSomeoneToTakeIt(t *testing.T) {
	c, _, states := newCall(t)
	c.HandleControl("P", "call_request", []byte(`{"type":"call_request","video":true,"audio":true}`))
	waitState(t, states, CallRinging)
	waitState(t, states, CallDeclined) // nobody to answer
	if st := c.Status(); st.HostCamera || st.HostMic {
		t.Fatalf("media on after a decline: %+v", st)
	}
}

func TestATakenCallStartsTheHostsMediaAndEnds(t *testing.T) {
	c, _, states := newCall(t)
	asked := make(chan string, 1)
	c.SetConsent(CallConsentFunc(func(_ context.Context, name string, video bool) (CallAnswer, error) {
		asked <- name
		return CallAnswer{Accept: true, Camera: true, Mic: false}, nil
	}))
	c.HandleControl("Q", "call_request", []byte(`{"video":true}`)) // not in session
	c.HandleControl("P", "call_request", []byte(`{"video":true,"audio":true}`))
	if name := <-asked; name != "Ana" {
		t.Fatalf("asked about %q", name)
	}
	st := waitState(t, states, CallActive)
	if !st.HostCamera || st.HostMic || !st.ViewerCamera {
		t.Fatalf("%+v", st)
	}
	c.SetMic(true)
	if !c.Status().HostMic {
		t.Fatal("the microphone did not come on")
	}
	c.SetCamera(false)
	if c.Status().HostCamera {
		t.Fatal("the camera stayed on")
	}
	c.HandleControl("P", "call_end", nil)
	waitState(t, states, CallEnded)
	// A new session starts without a call.
	c.SetPeer("R", "Bea")
	if st := c.Status(); st.State != CallIdle || st.Peer != "R" {
		t.Fatalf("%+v", st)
	}
}

func TestAutoAcceptCall(t *testing.T) {
	c, _, states := newCall(t)
	c.SetConsent(AutoAcceptCall)
	c.HandleControl("P", "call_request", []byte(`{"video":true,"audio":true}`))
	st := waitState(t, states, CallActive)
	if !st.HostCamera || !st.HostMic {
		t.Fatalf("%+v", st)
	}
	c.SetPeer("", "") // the session ended
	waitState(t, states, CallEnded)
}
