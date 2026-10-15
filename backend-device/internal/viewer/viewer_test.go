// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package viewer

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
	"github.com/lordbasex/go-link-desk/backend-device/pkg/encoder"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/signalclient"
)

func init() {
	settingEngine = func() webrtc.SettingEngine {
		se := webrtc.SettingEngine{}
		se.SetIncludeLoopbackCandidate(true)
		return se
	}
}

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// fakeHub stands in for signalhub and the host's session logic: it puts
// the viewer in the room, takes its knock and hands the viewer to the
// host's StreamService, which offers.
type fakeHub struct {
	t      *testing.T
	s      *Session
	stream *services.StreamService
	perms  models.Permissions
	// decide answers a knock; nil accepts with perms.
	decide func(h *fakeHub)
	codecs []string

	mu     sync.Mutex
	sent   []signalclient.Envelope
	closed bool
}

const hostID = "host-peer"
const viewerID = "viewer-peer"

func (h *fakeHub) Send(env signalclient.Envelope) error {
	h.mu.Lock()
	h.sent = append(h.sent, env)
	h.mu.Unlock()
	switch env.Type {
	case signalclient.TypeJoin, signalclient.TypeClaim, signalclient.TypeReach:
		reply := signalclient.TypeJoined
		if env.Type != signalclient.TypeJoin {
			reply = signalclient.TypePaired
		}
		go h.s.Handle(signalclient.Envelope{Type: reply, Remote: hostID})
	case signalclient.TypeSignal:
		var head struct{ Kind string }
		_ = json.Unmarshal(env.Payload, &head)
		if head.Kind == "knock" {
			go func() {
				if h.decide != nil {
					h.decide(h)
					return
				}
				h.accept()
			}()
			return nil
		}
		go func() { _ = h.stream.HandleSignal(viewerID, env.Payload) }()
	}
	return nil
}

func (h *fakeHub) Close() {
	h.mu.Lock()
	h.closed = true
	h.mu.Unlock()
}

// fromHost delivers a payload from the host to the viewer.
func (h *fakeHub) fromHost(v any) {
	b, _ := json.Marshal(v)
	h.s.Handle(signalclient.Envelope{Type: signalclient.TypeSignal, From: hostID, Payload: b})
}

func (h *fakeHub) accept() {
	h.fromHost(map[string]any{"kind": "accepted", "permissions": h.perms})
	if err := h.stream.AddViewer(viewerID, h.codecs); err != nil {
		h.t.Error(err)
	}
}

// hostSender relays the host's offers and candidates to the viewer.
type hostSender struct{ h *fakeHub }

func (r hostSender) Send(env signalclient.Envelope) error {
	env.From = hostID
	go r.h.s.Handle(env)
	return nil
}

func newHostStream(t *testing.T) *services.StreamService {
	t.Helper()
	stream, err := services.NewStreamService(services.StreamConfig{
		Width: 160, Height: 120, FPS: 30, BitrateKbps: 300, IncludeLoopback: true, Logger: quiet,
		Welcome: services.SessionMeta{Name: "office", OS: "darwin"},
	}, services.NewICEStore())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = stream.Run(ctx) }()
	return stream
}

// recorder keeps what the window would see.
type recorder struct {
	mu       sync.Mutex
	statuses []Status
	host     HostInfo
	frames   []Frame
	displays int
	chat     []ChatLine
	linked   []NewLink
	unlinked bool
	bound    bool
}

func (r *recorder) events() Events {
	return Events{
		Status: func(s Status) { r.mu.Lock(); r.statuses = append(r.statuses, s); r.mu.Unlock() },
		Host:   func(h HostInfo) { r.mu.Lock(); r.host = h; r.mu.Unlock() },
		Video: func(f Frame) {
			r.mu.Lock()
			if len(r.frames) < 1000 {
				r.frames = append(r.frames, f)
			}
			r.mu.Unlock()
		},
		Displays: func([]Display, int) { r.mu.Lock(); r.displays++; r.mu.Unlock() },
		Chat:     func(c ChatLine) { r.mu.Lock(); r.chat = append(r.chat, c); r.mu.Unlock() },
		Linked:   func(l NewLink) { r.mu.Lock(); r.linked = append(r.linked, l); r.mu.Unlock() },
		Unlinked: func() { r.mu.Lock(); r.unlinked = true; r.mu.Unlock() },
		Bound:    func() { r.mu.Lock(); r.bound = true; r.mu.Unlock() },
	}
}

func (r *recorder) last() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.statuses) == 0 {
		return Status{}
	}
	return r.statuses[len(r.statuses)-1]
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func startSession(t *testing.T, door Door, perms models.Permissions, decide func(*fakeHub)) (*Session, *fakeHub, *recorder) {
	t.Helper()
	rec := &recorder{}
	s := New(Options{Door: door, Name: "Ana", Agent: "go-link Desk test", Logger: quiet, Events: rec.events()})
	hub := &fakeHub{t: t, s: s, stream: newHostStream(t), perms: perms, decide: decide}
	hub.stream.SetSender(hostSender{hub})
	s.Start(hub)
	s.Connected(nil)
	t.Cleanup(s.HangUp)
	return s, hub, rec
}

func TestAJoinedSessionGetsThePictureAndTalks(t *testing.T) {
	if !encoder.Codecs {
		t.Skip("built without cgo: no libvpx or libopus")
	}
	controls := make(chan string, 20)
	s, hub, rec := startSession(t, Door{Kind: DoorJoin, Code: "915355636"}, models.Permissions{Control: true}, nil)
	hub.stream.OnControl(func(peerID, kind string, data []byte) {
		if peerID == viewerID {
			controls <- kind + " " + string(data)
		}
	})

	waitFor(t, "live", func() bool { return rec.last().Kind == StatusLive })
	if p := s.Permissions(); !p.View || !p.Control {
		t.Fatalf("permissions %+v", p)
	}
	// The knock carries the name, the agent and the codecs.
	hub.mu.Lock()
	var knock map[string]any
	for _, env := range hub.sent {
		if env.Type == signalclient.TypeSignal {
			_ = json.Unmarshal(env.Payload, &knock)
			break
		}
	}
	hub.mu.Unlock()
	if knock["kind"] != "knock" || knock["name"] != "Ana" || knock["ua"] != "go-link Desk test" {
		t.Fatalf("knock %v", knock)
	}

	// Frames come whole, the first one a keyframe.
	waitFor(t, "frames", func() bool { rec.mu.Lock(); defer rec.mu.Unlock(); return len(rec.frames) >= 10 })
	rec.mu.Lock()
	first := rec.frames[0]
	rec.mu.Unlock()
	if !first.Keyframe || first.Codec != "vp8" || len(first.Data) == 0 {
		t.Fatalf("first frame %+v", first.Codec)
	}
	waitFor(t, "welcome", func() bool { rec.mu.Lock(); defer rec.mu.Unlock(); return rec.host.Name == "office" })

	// The viewer's messages reach the host.
	line, ok := s.SendChat("  hola  ")
	if !ok || line.Text != "hola" {
		t.Fatalf("chat %+v", line)
	}
	select {
	case got := <-controls:
		if got != `chat {"text":"hola","type":"chat"}` {
			t.Fatalf("host got %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("chat did not arrive")
	}

	// The host hangs up.
	end, _ := json.Marshal(map[string]string{"type": "session_end", "reason": models.EndHost})
	hub.stream.Hangup(viewerID, end)
	waitFor(t, "ended", func() bool { return rec.last().Kind == StatusEnded })
	if r := rec.last().Reason; r != models.EndHost {
		t.Fatalf("reason %q", r)
	}
	hub.mu.Lock()
	closed := hub.closed
	hub.mu.Unlock()
	if !closed {
		t.Fatal("the viewer did not leave signalhub")
	}
}

func TestARejectedKnockEnds(t *testing.T) {
	_, _, rec := startSession(t, Door{Kind: DoorJoin, Code: "915355636"}, models.Permissions{}, func(h *fakeHub) {
		h.fromHost(map[string]any{"kind": "rejected", "reason": "busy"})
	})
	waitFor(t, "rejected", func() bool { return rec.last().Kind == StatusRejected })
	if rec.last().Reason != "busy" {
		t.Fatalf("%+v", rec.last())
	}
}

func TestSignalErrorsBeforeTheRoom(t *testing.T) {
	for msg, want := range map[string]StatusKind{
		"invalid or expired code": StatusNotFound,
		"rate limit exceeded":     StatusRateLimited,
		"device not connected":    StatusOffline,
	} {
		rec := &recorder{}
		s := New(Options{Door: Door{Kind: DoorJoin, Code: "1"}, Logger: quiet, Events: rec.events()})
		s.Start(nopTransport{})
		s.Handle(signalclient.Envelope{Type: signalclient.TypeError, Error: msg})
		if got := s.Status().Kind; got != want {
			t.Errorf("%q: %s, want %s", msg, got, want)
		}
	}
}

func TestReachingYourselfIsRefused(t *testing.T) {
	s := New(Options{Door: Door{Kind: DoorReach, Device: &models.RemoteDevice{DeviceID: "me"}}, OwnDeviceID: "me", Logger: quiet})
	s.Start(nopTransport{})
	s.Connected(nil)
	if s.Status().Kind != StatusSelf {
		t.Fatalf("%+v", s.Status())
	}
}

type nopTransport struct{}

func (nopTransport) Send(signalclient.Envelope) error { return nil }
func (nopTransport) Close()                           {}
