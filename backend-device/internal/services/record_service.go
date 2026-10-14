// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"
)

// RecordService lets the viewer in session save pictures of the screen and
// record it, only with the Record permission, and never without the person
// at the computer seeing it: the viewer asks first ("screenshot" {id},
// "recording" {id, on}) and saves or starts recording only after the host
// answered yes, which it does once the session panel shows it. Taking the
// permission away stops a recording ("recording" {on:false} unasked).
//
// The host cannot see what the viewer's browser does with the picture it
// already gets; this is for honest viewers, so that the person at the
// computer always knows (docs/security.md).
type RecordService struct {
	send   func(peerID string, msg []byte) bool
	status *StatusService
	log    *slog.Logger
	now    func() time.Time

	mu      sync.Mutex
	peer    string // the viewer in session
	allowed bool
	since   time.Time // recording since; zero: not recording
	shots   int
	last    time.Time
}

// NewRecordService builds the service; send delivers a control message.
func NewRecordService(send func(peerID string, msg []byte) bool, status *StatusService, log *slog.Logger) *RecordService {
	if log == nil {
		log = slog.Default()
	}
	return &RecordService{send: send, status: status, log: log, now: time.Now}
}

// Allow follows the session: peer is in it with Record on or off. A new
// viewer starts with nothing recorded; a viewer that leaves, or loses the
// permission, stops recording.
func (r *RecordService) Allow(peer string, in, on bool) {
	r.mu.Lock()
	switch {
	case in && peer != r.peer:
		r.peer, r.since, r.shots, r.last = peer, time.Time{}, 0, time.Time{}
	case !in && peer == r.peer:
		r.peer, r.allowed, r.since = "", false, time.Time{}
		r.mu.Unlock()
		return
	case !in:
		r.mu.Unlock()
		return
	}
	r.allowed = on
	stop := !on && !r.since.IsZero()
	if stop {
		r.since = time.Time{}
	}
	r.mu.Unlock()
	if stop {
		r.log.Info("the viewer stopped recording the screen: the permission was taken away", "peer_id", peer)
		r.reply(peer, recordingReply{Type: "recording", On: false})
		r.publish()
	}
}

type recordingReply struct {
	Type string `json:"type"`
	ID   uint32 `json:"id"`
	On   bool   `json:"on"`
}

type screenshotReply struct {
	Type string `json:"type"`
	ID   uint32 `json:"id"`
	OK   bool   `json:"ok"`
}

// Handle takes the viewer's "recording" {id, on}: it asks to start (yes
// only with the permission) or says it stopped. The answer carries the
// state the host shows.
func (r *RecordService) Handle(peer string, data []byte) {
	var m struct {
		ID uint32 `json:"id"`
		On bool   `json:"on"`
	}
	if json.Unmarshal(data, &m) != nil {
		return
	}
	r.mu.Lock()
	if peer == "" || peer != r.peer {
		r.mu.Unlock()
		return
	}
	was := !r.since.IsZero()
	on := m.On && r.allowed
	switch {
	case on && !was:
		r.since = r.now()
	case !on && was:
		r.since = time.Time{}
	}
	r.mu.Unlock()
	switch {
	case on && !was:
		r.log.Info("the viewer is recording the screen", "peer_id", peer)
	case !on && was:
		r.log.Info("the viewer stopped recording the screen", "peer_id", peer)
	case m.On && !on:
		r.log.Info("the viewer asked to record the screen without the permission", "peer_id", peer)
	}
	if on != was {
		r.publish() // shown before the answer goes
	}
	r.reply(peer, recordingReply{Type: "recording", ID: m.ID, On: on})
}

// HandleScreenshot takes the viewer's "screenshot" {id}: it asks to save a
// picture of the screen, yes only with the permission.
func (r *RecordService) HandleScreenshot(peer string, data []byte) {
	var m struct {
		ID uint32 `json:"id"`
	}
	if json.Unmarshal(data, &m) != nil {
		return
	}
	r.mu.Lock()
	if peer == "" || peer != r.peer {
		r.mu.Unlock()
		return
	}
	ok := r.allowed
	if ok {
		r.shots++
		r.last = r.now()
	}
	r.mu.Unlock()
	if ok {
		r.log.Info("the viewer saved a screenshot", "peer_id", peer)
		r.publish()
	} else {
		r.log.Info("the viewer asked for a screenshot without the permission", "peer_id", peer)
	}
	r.reply(peer, screenshotReply{Type: "screenshot", ID: m.ID, OK: ok})
}

// Recording says whether the viewer in session records the screen.
func (r *RecordService) Recording() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.since.IsZero()
}

func (r *RecordService) publish() {
	if r.status == nil {
		return
	}
	r.mu.Lock()
	peer, since, shots, last := r.peer, r.since, r.shots, r.last
	r.mu.Unlock()
	if peer != "" {
		r.status.SetRecording(peer, since, shots, last)
	}
}

func (r *RecordService) reply(peer string, v any) {
	b, _ := json.Marshal(v)
	r.send(peer, b)
}
