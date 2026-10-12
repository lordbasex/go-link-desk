// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build windows && !headless

package inject

import "testing"

// Every pixel comes back as itself through Windows' own rounding, on
// screens of any width and from monitors left of the primary one.
func TestAbsoluteCoordinatesLandOnTheirPixel(t *testing.T) {
	for _, axis := range []struct{ origin, size int }{{0, 1554}, {0, 1920}, {-1920, 3840}, {0, 7680}, {-1080, 2160}} {
		for p := axis.origin; p < axis.origin+axis.size; p++ {
			n := normalize(float64(p)+0.7, float64(axis.origin), float64(axis.size))
			if back := int(int64(n)*int64(axis.size)/65536) + axis.origin; back != p {
				t.Fatalf("pixel %d of %+v went as %d and lands on %d", p, axis, n, back)
			}
		}
	}
	if n := normalize(-5000, 0, 1920); n != 0 {
		t.Fatalf("outside on the left: %d", n)
	}
	if n := normalize(9000, 0, 1920); n != 65535 {
		t.Fatalf("outside on the right: %d", n)
	}
}

func TestKeysGoByScanCode(t *testing.T) {
	a := key(0x1E, false) // A
	if a.scan != 0x1E || a.flag != keyScanCode || a.vk != 0 {
		t.Fatalf("A: %+v", a)
	}
	up := key(0xE048, true) // the up arrow, an extended key
	if up.scan != 0x48 || up.flag != keyScanCode|keyExtended|keyUp {
		t.Fatalf("up arrow: %+v", up)
	}
	win := key(0xE05B, false) // the left Windows key
	if win.scan != 0x5B || win.flag&keyExtended == 0 {
		t.Fatalf("Windows key: %+v", win)
	}
	pause := key(0xE11D, false)
	if pause.vk != vkPause || pause.flag&keyScanCode != 0 {
		t.Fatalf("Pause: %+v", pause)
	}
}

// An injector lets go of everything it held. Only keys that do nothing are
// pressed: the test may run on someone's desktop.
func TestReleaseAllForgetsWhatWasHeld(t *testing.T) {
	w := New().(*win)
	w.Key(0x76, true, false, ModShift) // Shift+F24, held: it types nothing
	if !w.keys[0x76] || len(w.mods) != 1 {
		t.Fatalf("held: %v %v", w.keys, w.mods)
	}
	w.ReleaseAll()
	if len(w.keys) != 0 || len(w.buttons) != 0 || len(w.mods) != 0 {
		t.Fatalf("still held: %v %v %v", w.keys, w.buttons, w.mods)
	}
}

// Text goes by the layout's keys: a letter by its key, its capital with
// Shift; an emoji has no key (it goes as a Unicode character).
func TestTextFindsTheLayoutsKeys(t *testing.T) {
	hkl, _, _ := procGetLayout.Call(0)
	a, shift, ok := layoutKey('a', hkl)
	if !ok || shift != 0 || a == 0 {
		t.Fatalf("a: %#x %d %v", a, shift, ok)
	}
	if capA, shift, ok := layoutKey('A', hkl); !ok || shift != 1 || capA != a {
		t.Fatalf("A: %#x %d %v (a is %#x)", capA, shift, ok, a)
	}
	if _, _, ok := layoutKey('😀', hkl); ok {
		t.Fatal("an emoji has a key")
	}
	if _, _, ok := layoutKey('\x01', hkl); ok {
		t.Fatal("a control character has a key")
	}
}
