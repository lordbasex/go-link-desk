// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package inputproto

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type vector struct {
	Name   string `json:"name"`
	Hex    string `json:"hex"`
	Packet struct {
		Type    string `json:"type"`
		Seq     uint16 `json:"seq"`
		Display uint8  `json:"display"`
		X, Y    uint16
		DX      int16 `json:"dx"`
		DY      int16 `json:"dy"`
		Button  uint8 `json:"button"`
		Down    bool  `json:"down"`
		Repeat  bool  `json:"repeat"`
		Mode    uint8 `json:"mode"`
		Keycode uint16
		Mods    uint8  `json:"mods"`
		Text    string `json:"text"`
	} `json:"packet"`
}

func TestTheSharedVectors(t *testing.T) {
	raw, err := os.ReadFile("../../../protocol/vectors/input.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Packets []vector `json:"packets"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	types := map[string]byte{"abs": TypeAbs, "rel": TypeRel, "button": TypeButton, "wheel": TypeWheel, "key": TypeKey, "text": TypeText, "release_all": TypeReleaseAll}
	for _, v := range file.Packets {
		b, err := hex.DecodeString(strings.ReplaceAll(v.Hex, " ", ""))
		if err != nil {
			t.Fatalf("%s: %v", v.Name, err)
		}
		p, err := Decode(b)
		if err != nil {
			t.Fatalf("%s: %v", v.Name, err)
		}
		w := v.Packet
		want := Packet{Type: types[w.Type], Seq: w.Seq, Display: w.Display, X: w.X, Y: w.Y, DX: w.DX, DY: w.DY, Button: w.Button,
			Down: w.Down, Repeat: w.Repeat, Mode: w.Mode, Keycode: w.Keycode, Mods: w.Mods, Text: w.Text}
		if p != want {
			t.Errorf("%s:\n got  %+v\n want %+v", v.Name, p, want)
		}
		if got := Encode(want); !bytes.Equal(got, b) {
			t.Errorf("%s: Encode = %x, want %x", v.Name, got, b)
		}
	}
}

func TestBadPackets(t *testing.T) {
	for _, b := range [][]byte{
		nil,
		{0x01, 0, 1},                // short
		{0x3f, 0},                   // long
		{0x10, 9, 1, 0, 0, 0, 0, 0}, // no such button
		{0x30, 0, 1, 3, 0},          // down must be 0, 1 or 2
		{0x20, 0, 0, 0, 0, 2},       // no such wheel mode
		{0x31},                      // empty text
		{0x31, 0xff},                // not UTF-8
		append([]byte{0x31}, make([]byte, MaxText+1)...),
		{0x77},
	} {
		if _, err := Decode(b); err == nil {
			t.Errorf("%x was taken", b)
		}
	}
}

func TestSeqNewer(t *testing.T) {
	for _, tc := range []struct {
		seq, last uint16
		want      bool
	}{{2, 1, true}, {1, 1, false}, {1, 2, false}, {0, 65535, true}, {65535, 0, false}, {100, 65500, true}} {
		if got := SeqNewer(tc.seq, tc.last); got != tc.want {
			t.Errorf("SeqNewer(%d, %d) = %v", tc.seq, tc.last, got)
		}
	}
}

func TestEncodeCutsLongTextAtACharacter(t *testing.T) {
	b := Encode(Packet{Type: TypeText, Text: strings.Repeat("ñ", MaxText)})
	if len(b)-1 > MaxText {
		t.Fatalf("%d bytes of text", len(b)-1)
	}
	if _, err := Decode(b); err != nil {
		t.Fatal(err)
	}
}
