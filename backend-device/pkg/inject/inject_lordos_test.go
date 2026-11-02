// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux && headless && lordos

package inject

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/lordshare"
)

type fakeShare struct {
	mu  sync.Mutex
	got []string
	st  lordshare.State
}

func (f *fakeShare) add(s string, a ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = append(f.got, fmt.Sprintf(s, a...))
}
func (f *fakeShare) Move(x, y int)                  { f.add("move %d %d", x, y) }
func (f *fakeShare) Button(b int, d bool)           { f.add("btn %d %v", b, d) }
func (f *fakeShare) Wheel(s int)                    { f.add("wheel %d", s) }
func (f *fakeShare) Scroll(p int, x, y float64)     { f.add("scroll %d %g %g", p, x, y) }
func (f *fakeShare) Key(c, v int)                   { f.add("key %d %d", c, v) }
func (f *fakeShare) Text(s string)                  { f.add("text %s", s) }
func (f *fakeShare) State() (lordshare.State, bool) { return f.st, true }

func fake() (*injector, *fakeShare) {
	f := &fakeShare{}
	return &injector{share: func() sharer { return f }, keys: map[int]bool{}, btns: map[int]bool{}, synth: map[int]bool{}}, f
}

func TestKeysGoAsEvdev(t *testing.T) {
	i, f := fake()
	i.Key('a', true, false, 0)
	i.Key('a', true, true, 0)
	i.Key('a', false, false, 0)
	if want := []string{"key 30 1", "key 30 2", "key 30 0"}; !reflect.DeepEqual(f.got, want) {
		t.Fatal(f.got)
	}
}

func TestModifiersThatNeverArrived(t *testing.T) {
	i, f := fake()
	i.Key('a', true, false, ModCtrl)
	i.Key('a', false, false, ModCtrl)
	if want := []string{"key 29 1", "key 30 1", "key 30 0", "key 29 0"}; !reflect.DeepEqual(f.got, want) {
		t.Fatal(f.got)
	}
}

func TestNothingStaysHeld(t *testing.T) {
	i, f := fake()
	i.Key('a', true, false, 0)
	i.Button(Right, true, 5, 6, true)
	f.got = nil
	i.ReleaseAll()
	if len(f.got) != 2 {
		t.Fatal(f.got)
	}
}

func TestBackAndForwardHaveNoButton(t *testing.T) {
	i, f := fake()
	i.Button(Back, true, 0, 0, false)
	if len(f.got) != 0 {
		t.Fatal(f.got)
	}
}

func TestWheelGoesUpWhenPositive(t *testing.T) {
	i, f := fake()
	i.Scroll(0, 3, true)
	if f.got[0] != "wheel -3" {
		t.Fatal(f.got)
	}
}

func TestFineScrollIsAGesture(t *testing.T) {
	i, f := fake()
	i.Scroll(1, 7, false)
	i.Scroll(0, 2, false)
	if want := []string{"scroll 1 0 0", "scroll 2 1 7", "scroll 2 0 2"}; !reflect.DeepEqual(f.got, want) {
		t.Fatal(f.got)
	}
	time.Sleep(2 * scrollEnd)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.got[len(f.got)-1] != "scroll 3 0 0" {
		t.Fatal(f.got)
	}
}

func TestTrustedIsTheGrant(t *testing.T) {
	i, f := fake()
	if i.Trusted() {
		t.Fatal("trusted with no grant")
	}
	f.st.Granted = lordshare.FlagInput
	if !i.Trusted() {
		t.Fatal("not trusted with the grant")
	}
}
