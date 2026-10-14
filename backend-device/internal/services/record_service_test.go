// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
)

func TestRecordOnlyWithThePermissionAndShown(t *testing.T) {
	var out []string
	status := NewStatusService("d", "v", "ws://x")
	status.SetSession(&models.SessionStatus{})
	status.SetActive(&models.ActiveSession{PeerID: "P", Name: "Ana"})
	r := NewRecordService(func(peer string, b []byte) bool {
		out = append(out, peer+":"+string(b))
		return true
	}, status, slog.New(slog.NewTextHandler(io.Discard, nil)))
	at := time.Date(2026, 10, 14, 15, 4, 0, 0, time.UTC)
	r.now = func() time.Time { return at }
	active := func() models.ActiveSession { return *status.Snapshot().Session.Active }

	r.Allow("P", true, false)
	r.Handle("P", []byte(`{"type":"recording","id":1,"on":true}`))
	r.HandleScreenshot("P", []byte(`{"type":"screenshot","id":2}`))
	r.Handle("Q", []byte(`{"type":"recording","id":3,"on":true}`)) // not in session: no answer
	if r.Recording() || active().Screenshots != 0 {
		t.Fatal("recorded without the permission")
	}

	r.Allow("P", true, true)
	r.Handle("P", []byte(`{"type":"recording","id":4,"on":true}`))
	r.HandleScreenshot("P", []byte(`{"type":"screenshot","id":5}`))
	if a := active(); !r.Recording() || !a.RecordingSince.Equal(at) || a.Screenshots != 1 || !a.LastScreenshot.Equal(at) {
		t.Fatalf("not shown: %+v", a)
	}
	r.Allow("P", true, false) // the person at the computer unticks it
	if r.Recording() || !active().RecordingSince.IsZero() {
		t.Fatal("still recording without the permission")
	}
	want := []string{
		`P:{"type":"recording","id":1,"on":false}`,
		`P:{"type":"screenshot","id":2,"ok":false}`,
		`P:{"type":"recording","id":4,"on":true}`,
		`P:{"type":"screenshot","id":5,"ok":true}`,
		`P:{"type":"recording","id":0,"on":false}`,
	}
	if len(out) != len(want) {
		t.Fatalf("sent %v", out)
	}
	for i := range want {
		if out[i] != want[i] {
			t.Fatalf("sent %v", out)
		}
	}

	// A new viewer starts with nothing recorded; one that left records nothing.
	r.Allow("P", false, false)
	r.Allow("R", true, true)
	r.Handle("P", []byte(`{"type":"recording","id":6,"on":true}`))
	if r.Recording() || len(out) != len(want) {
		t.Fatalf("a viewer that left recorded: %v", out)
	}
}
