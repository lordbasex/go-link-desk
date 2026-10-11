// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"testing"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
)

func TestASessionWithoutInputEndsAfterTheTimeSet(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	var ended []string
	w := NewIdleWatch(func(r string) { ended = append(ended, r) }, func() time.Time { return now })
	w.Session("P")
	now = now.Add(time.Hour)
	w.check()
	if len(ended) != 0 {
		t.Fatal("ended with no limit set")
	}
	w.SetMinutes(5)
	w.Touch("P")
	now = now.Add(4 * time.Minute)
	w.Touch("other") // someone else's input is not this viewer's
	w.check()
	now = now.Add(59 * time.Second)
	w.Touch("P")
	now = now.Add(4 * time.Minute)
	w.check()
	if len(ended) != 0 {
		t.Fatal("input kept it alive")
	}
	now = now.Add(time.Minute)
	w.check()
	w.check()
	if len(ended) != 1 || ended[0] != models.EndIdle {
		t.Fatalf("ended %v", ended)
	}
	// A new session starts its own clock.
	w.Session("Q")
	w.check()
	if len(ended) != 1 {
		t.Fatal("a new session inherited the old clock")
	}
}
