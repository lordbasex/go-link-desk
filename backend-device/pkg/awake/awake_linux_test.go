// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux

package awake

import (
	"os"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// On a desktop by hand (DESK_AWAKE_TEST=1): it holds the display awake for
// a while, to look at the desktop's inhibitors meanwhile.
func TestDesktopHold(t *testing.T) {
	if os.Getenv("DESK_AWAKE_TEST") == "" {
		t.Skip("set DESK_AWAKE_TEST=1 on a desktop")
	}
	if c, err := dbus.ConnectSessionBus(); err == nil {
		for _, in := range inhibitors {
			t.Logf("%s: %v", in.name, in.ask(c))
		}
		c.Close()
	}
	if !hold() {
		t.Fatal("the desktop did not take the hold")
	}
	time.Sleep(8 * time.Second)
	release()
}
