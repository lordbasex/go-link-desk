// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux && !headless

package portal

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

// Streams as the portal's Start answers them: a(ua{sv}), size and
// position as (ii) structs; left to right.
func TestParseStreams(t *testing.T) {
	v := dbus.MakeVariant([][]any{
		{uint32(52), map[string]dbus.Variant{"size": dbus.MakeVariant([]any{int32(1920), int32(1080)}), "position": dbus.MakeVariant([]any{int32(1280), int32(0)})}},
		{uint32(49), map[string]dbus.Variant{"size": dbus.MakeVariant([]any{int32(1280), int32(800)}), "position": dbus.MakeVariant([]any{int32(0), int32(0)})}},
	})
	got := parseStreams(v)
	want := []Stream{{Node: 49, W: 1280, H: 800}, {Node: 52, X: 1280, W: 1920, H: 1080}}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("stream %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
	s := &Session{streams: got}
	if st := s.streamAt(1500, 10); st.Node != 52 {
		t.Errorf("point on the second monitor: stream %d", st.Node)
	}
}
