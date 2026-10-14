// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package viewer

import (
	"context"
	"encoding/json"
	"time"
)

// AskTimeout is how long the computer has to answer a screenshot or
// recording request: no answer is a no.
const AskTimeout = 5 * time.Second

// AskScreenshot asks the computer before saving a picture of its screen:
// true once the person there is shown it (only with the record permission).
func (s *Session) AskScreenshot(ctx context.Context) bool {
	if !s.Permissions().Record {
		return false
	}
	return s.ask(ctx, map[string]any{"type": "screenshot"})
}

// StartRecording asks the computer to show that this viewer records its
// screen: true once the person there sees it, and only then may the
// recording start.
func (s *Session) StartRecording(ctx context.Context) bool {
	if !s.Permissions().Record {
		return false
	}
	if !s.ask(ctx, map[string]any{"type": "recording", "on": true}) {
		return false
	}
	if !s.Permissions().Record { // taken away while asking
		s.StopRecording()
		return false
	}
	return true
}

// StopRecording tells the computer the recording stopped.
func (s *Session) StopRecording() {
	s.mu.Lock()
	was := s.recording
	s.recording = false
	s.mu.Unlock()
	if was {
		s.sendControl(map[string]any{"type": "recording", "id": 0, "on": false})
	}
}

// Recording says whether the computer shows that this viewer records.
func (s *Session) Recording() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recording
}

// ask sends a question the computer answers with the same id.
func (s *Session) ask(ctx context.Context, msg map[string]any) bool {
	s.mu.Lock()
	if s.status.Over() {
		s.mu.Unlock()
		return false
	}
	if s.asks == nil {
		s.asks = make(map[uint32]chan bool)
	}
	s.askSeq++
	if s.askSeq == 0 {
		s.askSeq = 1
	}
	id := s.askSeq
	ch := make(chan bool, 1)
	s.asks[id] = ch
	s.mu.Unlock()
	msg["id"] = id
	s.sendControl(msg)
	defer func() {
		s.mu.Lock()
		delete(s.asks, id)
		s.mu.Unlock()
	}()
	t := time.NewTimer(AskTimeout)
	defer t.Stop()
	select {
	case yes := <-ch:
		return yes
	case <-t.C:
		return false
	case <-ctx.Done():
		return false
	}
}

// answered takes the computer's screenshot or recording answer.
func (s *Session) answered(kind string, data []byte) {
	var m struct {
		ID uint32 `json:"id"`
		OK bool   `json:"ok"`
		On bool   `json:"on"`
	}
	if json.Unmarshal(data, &m) != nil {
		return
	}
	yes := m.OK
	if kind == "recording" {
		yes = m.On
	}
	s.mu.Lock()
	ch, waiting := s.asks[m.ID]
	if waiting {
		delete(s.asks, m.ID)
	}
	stopped := false
	late := false
	if kind == "recording" {
		switch {
		case yes && s.perms.Record && waiting:
			s.recording = true
		case yes && !waiting:
			late = true // a yes that came after its time: nothing records here
		case !yes && s.recording:
			s.recording, stopped = false, true
		}
	}
	s.mu.Unlock()
	if waiting {
		ch <- yes && (kind != "recording" || s.Permissions().Record)
	}
	if late {
		s.sendControl(map[string]any{"type": "recording", "id": 0, "on": false})
	}
	if stopped {
		if fn := s.opts.Events.Recording; fn != nil {
			fn(false)
		}
	}
}

// recordAllowed follows a permission change: a recording stops without it.
func (s *Session) recordAllowed(on bool) {
	s.mu.Lock()
	stopped := !on && s.recording
	if stopped {
		s.recording = false
	}
	s.mu.Unlock()
	if stopped {
		if fn := s.opts.Events.Recording; fn != nil {
			fn(false)
		}
	}
}

// failAsks answers every open question no (the session ended).
func (s *Session) failAsks() {
	s.mu.Lock()
	asks := s.asks
	s.asks = nil
	s.recording = false
	s.mu.Unlock()
	for _, ch := range asks {
		select {
		case ch <- false:
		default:
		}
	}
}
