// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package keymap

import (
	"encoding/json"
	"os"
	"testing"
)

// The generated table is protocol/keycodes.json, entry for entry: run
// go generate after changing the file.
func TestTheTableIsTheProtocols(t *testing.T) {
	raw, err := os.ReadFile("../../../protocol/keycodes.json")
	if err != nil {
		t.Fatal(err)
	}
	var table struct {
		Keys []struct {
			Code          string
			Mac, Win, X11 int
		} `json:"keys"`
	}
	if err := json.Unmarshal(raw, &table); err != nil {
		t.Fatal(err)
	}
	if len(table.Keys) != len(Keys) {
		t.Fatalf("%d keys in the file, %d generated: run go generate ./pkg/keymap", len(table.Keys), len(Keys))
	}
	for i, k := range table.Keys {
		if g := Keys[i]; g.Code != k.Code || g.Mac != k.Mac || g.Win != k.Win || g.X11 != k.X11 {
			t.Fatalf("key %d: file %+v, generated %+v", i, k, g)
		}
	}
}

func TestLookup(t *testing.T) {
	a := Index("KeyA")
	if k, ok := Lookup(a); !ok || k.Mac != 0 || k.Win != 0x1E || k.X11 != 'a' {
		t.Fatalf("KeyA %d: %+v", a, k)
	}
	if _, ok := Lookup(0); ok {
		t.Fatal("keycode 0 is Unidentified")
	}
	if _, ok := Lookup(60000); ok {
		t.Fatal("an unknown keycode was found")
	}
	if Index("NoSuchKey") != 0 {
		t.Fatal("an unknown code has an index")
	}
}
