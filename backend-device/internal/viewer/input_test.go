// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package viewer

import (
	"testing"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/inputproto"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/keymap"
)

type sinkRec struct {
	pointer []inputproto.Packet
	input   []inputproto.Packet
	clip    []string
}

func (s *sinkRec) SendPointer(b []byte) {
	p, _ := inputproto.Decode(b)
	s.pointer = append(s.pointer, p)
}

func (s *sinkRec) SendInput(b []byte) {
	p, err := inputproto.Decode(b)
	if err != nil {
		panic(err)
	}
	s.input = append(s.input, p)
}

func (s *sinkRec) SendClipboard(text string, paste bool) bool {
	if paste {
		text += " (paste)"
	}
	s.clip = append(s.clip, text)
	return true
}

// macOS virtual key codes.
const (
	macA       = 0x00
	macV       = 0x09
	macCommand = 0x37
)

func TestNothingIsSentWithoutControl(t *testing.T) {
	s := &sinkRec{}
	in := NewInput(s)
	in.Move(0.5, 0.5, true)
	in.Button(0, true, 0.5, 0.5, true)
	in.Key(macA, true, false, 0)
	in.Wheel(0, 10, false)
	if len(s.pointer)+len(s.input) != 0 {
		t.Fatalf("sent %v %v", s.pointer, s.input)
	}
}

func TestPointerAndButtons(t *testing.T) {
	s := &sinkRec{}
	in := NewInput(s)
	in.Allow(true, false)
	in.SetDisplay(1)
	in.Move(0.5, 1, true)
	in.Move(0.2, 0.2, false) // over the window, not the picture
	in.Move(0, 0, true)
	if len(s.pointer) != 2 || s.pointer[0].X != 32768 || s.pointer[0].Y != 65535 || s.pointer[0].Display != 1 || s.pointer[1].Seq != s.pointer[0].Seq+1 {
		t.Fatalf("%+v", s.pointer)
	}
	in.Button(0, true, 0.25, 0.75, true)
	in.Button(0, false, 0, 0, false) // released outside the picture
	in.Button(2, false, 0.5, 0.5, true)
	if len(s.input) != 2 || !s.input[0].Down || s.input[0].X != 16384 || s.input[1].Down || s.input[1].Display != 0xFF {
		t.Fatalf("%+v", s.input)
	}
}

func TestWheelFractionsAddUp(t *testing.T) {
	s := &sinkRec{}
	in := NewInput(s)
	in.Allow(true, false)
	in.Wheel(0, 0.4, false)
	in.Wheel(0, 0.4, false)
	in.Wheel(0, 0.4, false)
	in.Wheel(0, -3, true)
	if len(s.input) != 2 || s.input[0].DY != 1 || s.input[0].Mode != inputproto.WheelPixels || s.input[1].DY != -2 || s.input[1].Mode != inputproto.WheelLines {
		t.Fatalf("%+v", s.input)
	}
}

func TestKeysGoByPositionAndCmdLetsGoOfItsKeys(t *testing.T) {
	s := &sinkRec{}
	in := NewInput(s)
	in.Allow(true, false)
	in.Key(macCommand, true, false, inputproto.ModMeta)
	in.Key(macA, true, false, inputproto.ModMeta)
	in.Key(macA, true, true, inputproto.ModMeta)
	// macOS never reports A's release: Cmd going up lets go of it.
	in.Key(macCommand, false, false, 0)
	in.Key(macA, false, false, 0) // already released: dropped
	codes := []string{}
	for _, p := range s.input {
		state := "up"
		if p.Repeat {
			state = "repeat"
		} else if p.Down {
			state = "down"
		}
		codes = append(codes, keymap.Keys[p.Keycode].Code+" "+state)
	}
	want := []string{"MetaLeft down", "KeyA down", "KeyA repeat", "MetaLeft up", "KeyA up"}
	if len(codes) != len(want) {
		t.Fatalf("%v", codes)
	}
	for i := range want {
		if codes[i] != want[i] {
			t.Fatalf("%v, want %v", codes, want)
		}
	}
}

func TestCmdVPastesThisComputersClipboard(t *testing.T) {
	s := &sinkRec{}
	in := NewInput(s)
	in.Clipboard = func() (string, bool) { return "hola", true }
	in.Allow(true, true)
	in.Key(macV, true, false, inputproto.ModMeta)
	in.Key(macV, false, false, inputproto.ModMeta)
	if len(s.clip) != 1 || s.clip[0] != "hola (paste)" || len(s.input) != 0 {
		t.Fatalf("clip %v input %v", s.clip, s.input)
	}
	// Without the clipboard permission, Cmd+V is a key like any other.
	in.Allow(true, false)
	in.Key(macV, true, false, inputproto.ModMeta)
	if len(s.clip) != 1 || len(s.input) != 1 {
		t.Fatalf("clip %v input %v", s.clip, s.input)
	}
}

func TestLosingControlReleasesEverything(t *testing.T) {
	s := &sinkRec{}
	in := NewInput(s)
	in.Allow(true, false)
	in.Key(macA, true, false, 0)
	in.Allow(false, false)
	if last := s.input[len(s.input)-1]; last.Type != inputproto.TypeReleaseAll {
		t.Fatalf("%+v", last)
	}
	in.Key(macA, false, false, 0)
	if s.input[len(s.input)-1].Type != inputproto.TypeReleaseAll {
		t.Fatal("a release after losing control")
	}
}

func TestComboAndText(t *testing.T) {
	s := &sinkRec{}
	in := NewInput(s)
	in.Allow(true, false)
	in.Combo("MetaLeft", "Space")
	if len(s.input) != 4 || !s.input[1].Down || s.input[1].Mods != inputproto.ModMeta || s.input[3].Mods != 0 {
		t.Fatalf("%+v", s.input)
	}
	s.input = nil
	long := make([]byte, inputproto.MaxText+10)
	for i := range long {
		long[i] = 'x'
	}
	in.TypeText(string(long))
	if len(s.input) != 2 || len(s.input[0].Text)+len(s.input[1].Text) != len(long) {
		t.Fatalf("%d packets", len(s.input))
	}
}
