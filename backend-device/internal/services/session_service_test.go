// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/signalclient"
)

func next(t *testing.T, f *fakeSender) signalclient.Envelope {
	t.Helper()
	select {
	case env := <-f.ch:
		return env
	case <-time.After(2 * time.Second):
		t.Fatal("nothing sent")
		return signalclient.Envelope{}
	}
}

func newSession() (*SessionService, *RoomOpener, *StatusService, *fakeSender) {
	s, opener, st, snd, _ := newSessionWith()
	return s, opener, st, snd
}

func newSessionWith() (*SessionService, *RoomOpener, *StatusService, *fakeSender, *fakeViewers) {
	st := NewStatusService(deviceID, "test", "ws://x")
	opener := NewRoomOpener()
	v := &fakeViewers{}
	s := NewSessionService(opener, st, SessionMeta{Name: "mac", OS: "darwin"}, v, slog.New(slog.NewTextHandler(io.Discard, nil)))
	snd := newFakeSender()
	opener.SetSender(snd)
	s.SetSender(snd)
	return s, opener, st, snd, v
}

func TestSessionOpensAnInviteOnlyRoomAndShowsItsCode(t *testing.T) {
	s, opener, st, snd := newSession()
	s.OnConnect(signalclient.Envelope{PeerID: "A"})
	open := next(t, snd)
	if open.Type != signalclient.TypeRoomOpen || !open.InviteOnly || open.Public {
		t.Fatalf("room_open: %+v", open)
	}
	var meta SessionMeta
	if err := json.Unmarshal(open.Meta, &meta); err != nil || meta != (SessionMeta{Name: "mac", OS: "darwin"}) {
		t.Fatalf("meta %s: %v", open.Meta, err)
	}
	opener.OnMessage(signalclient.Envelope{Type: signalclient.TypeRoomOpened, RoomID: "R"})
	if inv := next(t, snd); inv.Type != signalclient.TypeInviteCreate || inv.RoomID != "R" {
		t.Fatalf("invite_create: %+v", inv)
	}
	s.OnMessage(signalclient.Envelope{Type: signalclient.TypeInviteCreated, RoomID: "R", Invite: "abcdefghijklmnopqrstuv", Code: "915355636"})
	got := st.Snapshot().Session
	if got == nil || got.RoomID != "R" || got.Code != "915355636" || got.Invite != "abcdefghijklmnopqrstuv" {
		t.Fatalf("session %+v", got)
	}

	// Another room's invitation is not ours.
	s.OnMessage(signalclient.Envelope{Type: signalclient.TypeInviteCreated, RoomID: "X", Code: "111111111"})
	if st.Snapshot().Session.Code != "915355636" {
		t.Fatal("took another room's code")
	}

	// A new code is a new invitation for the same room.
	s.NewCode()
	if inv := next(t, snd); inv.Type != signalclient.TypeInviteCreate || inv.RoomID != "R" {
		t.Fatalf("new code: %+v", inv)
	}

	s.OnDisconnect(nil)
	if st.Snapshot().Session != nil {
		t.Fatal("the session outlived the connection")
	}
	s.NewCode()
	select {
	case env := <-snd.ch:
		t.Fatalf("asked for a code without a room: %+v", env)
	default:
	}
}

func TestALateRoomFromAnOldConnectionIsDropped(t *testing.T) {
	s, opener, st, snd := newSession()
	s.OnConnect(signalclient.Envelope{})
	next(t, snd)
	// The link drops and comes back before room_opened arrives.
	opener.OnDisconnect(nil)
	s.OnDisconnect(nil)
	s.OnConnect(signalclient.Envelope{})
	next(t, snd)
	opener.OnMessage(signalclient.Envelope{Type: signalclient.TypeRoomOpened, RoomID: "NEW"})
	if got := st.Snapshot().Session; got == nil || got.RoomID != "NEW" {
		t.Fatalf("session %+v", got)
	}
}

func TestFormatCode(t *testing.T) {
	for in, want := range map[string]string{"915355636": "915 355 636", "123": "123", "": ""} {
		if got := FormatCode(in); got != want {
			t.Errorf("FormatCode(%q) = %q, want %q", in, got, want)
		}
	}
}

// fakeViewers records what the session asks of the stream.
type fakeViewers struct {
	mu sync.Mutex
	viewerCalls
}

// viewerCalls is what fakeViewers recorded.
type viewerCalls struct {
	added   []string
	signals []string
	hungUp  []string
	removed []string
	closed  int
	last    []byte
}

func (f *fakeViewers) AddViewer(peerID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.added = append(f.added, peerID)
	return nil
}

func (f *fakeViewers) HandleSignal(from string, payload json.RawMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.signals = append(f.signals, from+":"+string(payload))
	return nil
}

func (f *fakeViewers) Hangup(peerID string, last []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hungUp = append(f.hungUp, peerID)
	f.last = last
}

func (f *fakeViewers) RemoveViewer(peerID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, peerID)
}

func (f *fakeViewers) CloseAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed++
}

func (f *fakeViewers) snapshot() viewerCalls {
	f.mu.Lock()
	defer f.mu.Unlock()
	return viewerCalls{added: slices.Clone(f.added), signals: slices.Clone(f.signals), hungUp: slices.Clone(f.hungUp), removed: slices.Clone(f.removed), closed: f.closed, last: f.last}
}

// openRoom brings a session to an open room "R" with a code.
func openRoom(t *testing.T) (*SessionService, *StatusService, *fakeSender, *fakeViewers) {
	t.Helper()
	s, opener, st, snd, v := newSessionWith()
	s.OnConnect(signalclient.Envelope{})
	next(t, snd)
	opener.OnMessage(signalclient.Envelope{Type: signalclient.TypeRoomOpened, RoomID: "R"})
	next(t, snd) // invite_create
	s.OnMessage(signalclient.Envelope{Type: signalclient.TypeInviteCreated, RoomID: "R", Code: "915355636"})
	return s, st, snd, v
}

func join(s *SessionService, peer string) {
	s.OnMessage(signalclient.Envelope{Type: signalclient.TypePeerJoined, RoomID: "R", Remote: peer})
}

func signalFrom(s *SessionService, peer, payload string) {
	s.OnMessage(signalclient.Envelope{Type: signalclient.TypeSignal, From: peer, Payload: json.RawMessage(payload)})
}

// reply waits for the next signal to peer and returns its payload.
func reply(t *testing.T, snd *fakeSender, peer string) map[string]any {
	t.Helper()
	env := next(t, snd)
	if env.Type != signalclient.TypeSignal || env.To != peer {
		t.Fatalf("expected a signal to %s, got %+v", peer, env)
	}
	var m map[string]any
	if err := json.Unmarshal(env.Payload, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// askedConsent is a consent that hands each request to the test.
type askedConsent struct {
	reqs    chan ConsentRequest
	answers chan error
}

func newAskedConsent() *askedConsent {
	return &askedConsent{reqs: make(chan ConsentRequest, 4), answers: make(chan error, 4)}
}

func (c *askedConsent) Ask(ctx context.Context, req ConsentRequest) (models.Permissions, error) {
	c.reqs <- req
	select {
	case err := <-c.answers:
		return models.Permissions{Control: true}, err
	case <-ctx.Done():
		return models.Permissions{}, ctx.Err()
	}
}

func (c *askedConsent) request(t *testing.T) ConsentRequest {
	t.Helper()
	select {
	case r := <-c.reqs:
		return r
	case <-time.After(2 * time.Second):
		t.Fatal("nobody was asked")
		return ConsentRequest{}
	}
}

func TestAnAcceptedViewerGetsTheStream(t *testing.T) {
	s, st, snd, v := openRoom(t)
	c := newAskedConsent()
	s.SetConsent(c)
	join(s, "P")

	// Before it knocks, nothing it sends reaches the stream.
	signalFrom(s, "P", `{"kind":"answer","sdp":"x"}`)
	signalFrom(s, "P", `{"kind":"knock","name":"  Ana\n‮evil ","ua":"Chrome 141 on macOS"}`)
	req := c.request(t)
	if req.PeerID != "P" || req.Knock.Name != "Ana evil" || req.Knock.Browser != "Chrome 141 on macOS" {
		t.Fatalf("asked %+v", req)
	}
	// A second knock while asked is not a second question.
	signalFrom(s, "P", `{"kind":"knock","name":"Ana"}`)
	c.answers <- nil

	got := reply(t, snd, "P")
	if got["kind"] != "accepted" {
		t.Fatalf("reply %v", got)
	}
	perm := got["permissions"].(map[string]any)
	if perm["view"] != true || perm["control"] != true {
		t.Fatalf("permissions %v", perm)
	}
	if a := st.Snapshot().Session.Active; a == nil || a.Name != "Ana evil" || !a.Permissions.View {
		t.Fatalf("active %+v", a)
	}
	if got := v.snapshot(); !slices.Equal(got.added, []string{"P"}) || len(got.signals) != 0 {
		t.Fatalf("stream %+v", got)
	}
	select {
	case r := <-c.reqs:
		t.Fatalf("asked twice: %+v", r)
	default:
	}

	// Now its answer and candidates go to the stream; offers never do.
	signalFrom(s, "P", `{"kind":"answer","sdp":"x"}`)
	signalFrom(s, "P", `{"kind":"offer","sdp":"x"}`)
	if got := v.snapshot().signals; len(got) != 1 || got[0] != `P:{"kind":"answer","sdp":"x"}` {
		t.Fatalf("signals %v", got)
	}

	// A new code keeps the session on.
	s.OnMessage(signalclient.Envelope{Type: signalclient.TypeInviteCreated, RoomID: "R", Code: "111222333"})
	if st.Snapshot().Session.Active == nil {
		t.Fatal("a new code ended the session")
	}

	// The viewer leaving signalhub ends it.
	s.OnMessage(signalclient.Envelope{Type: signalclient.TypePeerLeft, From: "P"})
	if st.Snapshot().Session.Active != nil {
		t.Fatal("the session outlived its viewer")
	}
	if got := v.snapshot().removed; !slices.Equal(got, []string{"P"}) {
		t.Fatalf("removed %v", got)
	}
}

func TestRejectedViewersGetNothing(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer func(c *askedConsent, s *SessionService)
		reason string
	}{
		{"declined", func(c *askedConsent, _ *SessionService) { c.answers <- ErrDeclined }, models.RejectDeclined},
		{"timeout", func(*askedConsent, *SessionService) {}, models.RejectTimeout},
		{"nobody to ask", func(_ *askedConsent, s *SessionService) {}, models.RejectUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, st, snd, v := openRoom(t)
			c := newAskedConsent()
			if tc.reason != models.RejectUnavailable {
				s.SetConsent(c)
			}
			s.setTimeout(100 * time.Millisecond)
			join(s, "P")
			signalFrom(s, "P", `{"kind":"knock","name":"Mallory"}`)
			if tc.reason != models.RejectUnavailable {
				c.request(t)
			}
			tc.answer(c, s)
			got := reply(t, snd, "P")
			if got["kind"] != "rejected" || got["reason"] != tc.reason {
				t.Fatalf("reply %v", got)
			}
			// It cannot knock again, and what it sends is ignored.
			signalFrom(s, "P", `{"kind":"knock","name":"Mallory"}`)
			signalFrom(s, "P", `{"kind":"answer","sdp":"x"}`)
			select {
			case env := <-snd.ch:
				t.Fatalf("answered a turned-down guest: %+v", env)
			case <-time.After(150 * time.Millisecond):
			}
			if got := v.snapshot(); len(got.added)+len(got.signals) != 0 || st.Snapshot().Session.Active != nil {
				t.Fatalf("stream %+v", got)
			}
		})
	}
}

func TestOneSessionAtATime(t *testing.T) {
	s, _, snd, _ := openRoom(t)
	c := newAskedConsent()
	s.SetConsent(c)
	join(s, "P")
	join(s, "Q")
	signalFrom(s, "P", `{"kind":"knock","name":"Ana"}`)
	c.request(t)
	// While Ana's request is on screen, Bob is turned away.
	signalFrom(s, "Q", `{"kind":"knock","name":"Bob"}`)
	if got := reply(t, snd, "Q"); got["kind"] != "rejected" || got["reason"] != models.RejectBusy {
		t.Fatalf("reply %v", got)
	}
	c.answers <- nil
	reply(t, snd, "P") // accepted
	// And while Ana is in session.
	join(s, "Z")
	signalFrom(s, "Z", `{"kind":"knock","name":"Zoe"}`)
	if got := reply(t, snd, "Z"); got["reason"] != models.RejectBusy {
		t.Fatalf("reply %v", got)
	}
}

func TestSignalsFromStrangersAreIgnored(t *testing.T) {
	s, _, snd, v := openRoom(t)
	s.SetConsent(AutoAccept)
	// Never joined the room (another room's guest, or a linked browser).
	signalFrom(s, "X", `{"kind":"knock","name":"Eve"}`)
	// Too big to read.
	join(s, "P")
	signalFrom(s, "P", `{"kind":"knock","name":"`+strings.Repeat("a", maxSignalPayload)+`"}`)
	select {
	case env := <-snd.ch:
		t.Fatalf("answered %+v", env)
	case <-time.After(100 * time.Millisecond):
	}
	if got := v.snapshot(); len(got.added) != 0 {
		t.Fatalf("stream %+v", got)
	}
}

func TestTheViewerLeavingClosesItsRequest(t *testing.T) {
	s, _, snd, _ := openRoom(t)
	c := newAskedConsent()
	s.SetConsent(c)
	join(s, "P")
	signalFrom(s, "P", `{"kind":"knock","name":"Ana"}`)
	c.request(t)
	s.OnMessage(signalclient.Envelope{Type: signalclient.TypePeerLeft, From: "P"})
	select {
	case env := <-snd.ch:
		t.Fatalf("answered a viewer that left: %+v", env)
	case <-time.After(100 * time.Millisecond):
	}
	// The question is gone: another viewer can knock.
	join(s, "Q")
	signalFrom(s, "Q", `{"kind":"knock","name":"Bob"}`)
	if r := c.request(t); r.PeerID != "Q" {
		t.Fatalf("asked %+v", r)
	}
}

func TestEndingASession(t *testing.T) {
	for _, tc := range []struct {
		name   string
		end    func(s *SessionService)
		reason string
		hangup bool
	}{
		{"the host disconnects", func(s *SessionService) { s.End(models.EndHost) }, models.EndHost, true},
		{"the viewer hangs up", func(s *SessionService) { s.ViewerHungUp("P") }, models.EndViewer, false},
		{"the network fails", func(s *SessionService) { s.StreamClosed("P", models.EndNetwork) }, models.EndNetwork, false},
		{"signalhub goes away", func(s *SessionService) { s.OnDisconnect(nil) }, models.EndSignal, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, st, snd, v := openRoom(t)
			s.SetConsent(AutoAccept)
			join(s, "P")
			signalFrom(s, "P", `{"kind":"knock","name":"Ana"}`)
			reply(t, snd, "P")
			tc.end(s)
			if sess := st.Snapshot().Session; sess != nil && sess.Active != nil {
				t.Fatal("still in session")
			}
			got := v.snapshot()
			if tc.hangup {
				if !slices.Equal(got.hungUp, []string{"P"}) || !strings.Contains(string(got.last), `"reason":"host"`) {
					t.Fatalf("hangup %+v %s", got.hungUp, got.last)
				}
			}
			// After the end, the viewer is ignored until it leaves.
			signalFrom(s, "P", `{"kind":"knock","name":"Ana"}`)
			signalFrom(s, "P", `{"kind":"answer","sdp":"x"}`)
			if n := len(v.snapshot().signals); n != 0 {
				t.Fatalf("signals after the end: %d", n)
			}
		})
	}
}

func TestKnockCleaning(t *testing.T) {
	k := models.Knock{Name: strings.Repeat("é", 100), Browser: "a\tb\x00c"}.Clean()
	if utf8.RuneCountInString(k.Name) != models.MaxKnockName || k.Browser != "a bc" {
		t.Fatalf("clean %q %q", k.Name, k.Browser)
	}
}
