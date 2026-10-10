// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package controllers

import (
	"io"
	"log/slog"
	"testing"
)

type hungUp []string

func (h *hungUp) ViewerHungUp(peer string) { *h = append(*h, peer) }

func TestControlRoutesByType(t *testing.T) {
	c := NewControl(slog.New(slog.NewTextHandler(io.Discard, nil)))
	var got []string
	c.Handle("chat", func(peer string, data []byte) { got = append(got, peer+":"+string(data)) })
	c.Dispatch("P", "chat", []byte(`{"type":"chat"}`))
	c.Dispatch("P", "unknown", []byte(`{"type":"unknown"}`))
	if len(got) != 1 || got[0] != `P:{"type":"chat"}` {
		t.Fatalf("routed %v", got)
	}

	var h hungUp
	SessionRoutes(c, &h)
	c.Dispatch("P", "session_end", nil)
	if len(h) != 1 || h[0] != "P" {
		t.Fatalf("hung up %v", h)
	}
}
