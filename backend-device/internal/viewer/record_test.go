// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package viewer

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// sent is the last control message waiting to go out (no connection yet).
func (s *Session) lastSent(t *testing.T) map[string]any {
	t.Helper()
	end := time.Now().Add(2 * time.Second)
	for time.Now().Before(end) {
		s.mu.Lock()
		n := len(s.pendingControl)
		var last []byte
		if n > 0 {
			last = s.pendingControl[n-1]
		}
		s.mu.Unlock()
		if last != nil {
			var m map[string]any
			_ = json.Unmarshal(last, &m)
			return m
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("nothing sent")
	return nil
}

func TestScreenshotsAndRecordingAskTheComputer(t *testing.T) {
	var stopped []bool
	s := New(Options{Events: Events{Recording: func(on bool) { stopped = append(stopped, on) }}})
	ctx := context.Background()
	if s.AskScreenshot(ctx) || s.StartRecording(ctx) {
		t.Fatal("asked without the permission")
	}
	s.onControl([]byte(`{"type":"permissions","permissions":{"view":true,"record":true}}`))

	got := make(chan bool, 1)
	go func() { got <- s.StartRecording(ctx) }()
	m := s.lastSent(t)
	if m["type"] != "recording" || m["on"] != true {
		t.Fatalf("sent %v", m)
	}
	s.onControl([]byte(`{"type":"recording","id":` + jsonNum(m["id"]) + `,"on":true}`))
	if !<-got || !s.Recording() {
		t.Fatal("not recording after the yes")
	}

	// The person at the computer unticks it: the recording stops here.
	s.onControl([]byte(`{"type":"permissions","permissions":{"view":true}}`))
	if s.Recording() || len(stopped) != 1 {
		t.Fatalf("still recording (%v)", stopped)
	}

	// A yes that never comes is a no.
	s.onControl([]byte(`{"type":"permissions","permissions":{"view":true,"record":true}}`))
	c, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if s.AskScreenshot(c) {
		t.Fatal("a screenshot without an answer")
	}
	// A late yes is turned down.
	s.onControl([]byte(`{"type":"recording","id":99,"on":true}`))
	if s.Recording() {
		t.Fatal("a late yes started a recording")
	}
	if m := s.lastSent(t); m["type"] != "recording" || m["on"] != false {
		t.Fatalf("the late yes was not turned down: %v", m)
	}
}

func jsonNum(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
