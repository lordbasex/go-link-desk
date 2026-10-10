// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

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
	st := NewStatusService(deviceID, "test", "ws://x")
	opener := NewRoomOpener()
	s := NewSessionService(opener, st, SessionMeta{Name: "mac", OS: "darwin"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	snd := newFakeSender()
	opener.SetSender(snd)
	s.SetSender(snd)
	return s, opener, st, snd
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
