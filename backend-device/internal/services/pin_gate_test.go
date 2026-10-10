// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"testing"
	"time"
)

func TestNewPINIsSixDigits(t *testing.T) {
	for range 50 {
		if pin := NewPIN(); !ValidPIN(pin) {
			t.Fatalf("bad PIN %q", pin)
		}
	}
	for _, bad := range []string{"", "12345", "1234567", "12a456", " 12345"} {
		if ValidPIN(bad) {
			t.Fatalf("%q passed", bad)
		}
	}
}

func TestPinGateLimitsGuesses(t *testing.T) {
	now := time.Date(2026, 9, 27, 3, 0, 0, 0, time.UTC)
	g := NewPinGate(func() time.Time { return now })
	g.SetPrivate(true)
	pin := g.Issue().Pin
	if r := g.Check("a", wrongPin(pin), ""); r.OK || r.Reason != "wrong" || r.Left != 4 {
		t.Fatalf("first wrong: %+v", r)
	}
	for range 3 {
		g.Check("a", wrongPin(pin), "")
	}
	if r := g.Check("a", wrongPin(pin), ""); r.Reason != "blocked" {
		t.Fatalf("fifth wrong: %+v", r)
	}
	// Blocked even with the right PIN.
	if r := g.Check("a", pin, ""); r.OK {
		t.Fatal("a blocked guest got in")
	}
	// Many guests guessing lock the whole room for a while.
	for i := range 16 {
		g.Check(string(rune('c'+i)), wrongPin(pin), "")
	}
	if r := g.Check("z", pin, ""); r.OK || r.Reason != "locked" || r.RetryAfter <= 0 {
		t.Fatalf("room lock: %+v", r)
	}
	now = now.Add(pinLock + time.Second)
	if r := g.Check("z", pin, ""); !r.OK {
		t.Fatalf("after the lock: %+v", r)
	}
}

func TestEveryInvitationLetsInOnePerson(t *testing.T) {
	now := time.Date(2026, 9, 27, 3, 0, 0, 0, time.UTC)
	g := NewPinGate(func() time.Time { return now })
	g.SetPrivate(true)
	first, second := g.Issue(), g.Issue()
	if first.Pin == second.Pin || !first.ExpiresAt.Equal(now.Add(passTTL)) {
		t.Fatalf("passes %+v %+v", first, second)
	}
	in := g.Check("ana", first.Pin, "")
	if !in.OK || in.Token == "" {
		t.Fatalf("first use: %+v", in)
	}
	// Someone else with the same invitation is told it was used.
	if r := g.Check("bob", first.Pin, ""); r.OK || r.Reason != "used" {
		t.Fatalf("second use: %+v", r)
	}
	// Ana reloads the page: her token brings her back without a PIN.
	if r := g.Check("ana-again", "", in.Token); !r.OK || r.Token != in.Token {
		t.Fatalf("token: %+v", r)
	}
	if r := g.Check("eve", "", "made-up"); r.OK {
		t.Fatal("a made-up token got in")
	}
	// The host's own key always works.
	if r := g.Check("host", "", g.OwnerKey()); !r.OK || !r.Owner {
		t.Fatalf("owner key: %+v", r)
	}
	// An invitation nobody used expires; a used one keeps working.
	now = now.Add(passTTL)
	if r := g.Check("bob", second.Pin, ""); r.OK {
		t.Fatal("an expired invitation got in")
	}
	if r := g.Check("ana-3", "", in.Token); !r.OK {
		t.Fatalf("token after the invitation time: %+v", r)
	}
}

func TestAnOpenGateAsksNothing(t *testing.T) {
	g := NewPinGate(nil)
	if g.Required() || !g.Check("a", "", "").OK {
		t.Fatal("an open room asked for a PIN")
	}
}

// wrongPin returns a PIN that is not pin.
func wrongPin(pin string) string {
	if pin == "000000" {
		return "111111"
	}
	return "000000"
}

func TestAPinGuesserCannotLockOutTheHostOrGuestsAlreadyIn(t *testing.T) {
	now := time.Date(2026, 9, 27, 3, 0, 0, 0, time.UTC)
	g := NewPinGate(func() time.Time { return now })
	g.SetPrivate(true)
	pass := g.Issue()
	in := g.Check("ana", pass.Pin, "")
	// Many wrong PINs lock the room...
	for i := range pinRoomFails {
		g.Check(string(rune('a'+i))+"-guesser", wrongPin(pass.Pin), "")
	}
	if r := g.Check("new", g.Issue().Pin, ""); r.Reason != "locked" {
		t.Fatalf("the PIN path should be locked: %+v", r)
	}
	// ...but not for the host or a guest coming back with its token.
	if r := g.Check("host", "", g.OwnerKey()); !r.OK {
		t.Fatalf("the host was locked out: %+v", r)
	}
	if r := g.Check("ana-reload", "", in.Token); !r.OK {
		t.Fatalf("a guest already in was locked out: %+v", r)
	}
}
