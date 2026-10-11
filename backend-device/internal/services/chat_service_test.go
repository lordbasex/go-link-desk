// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"
)

func TestChatBothWaysWithTheViewerInSession(t *testing.T) {
	var out []string
	c := NewChatService(func(peer string, b []byte) bool {
		var m struct{ Type, Text string }
		_ = json.Unmarshal(b, &m)
		out = append(out, peer+":"+m.Type+":"+m.Text)
		return true
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	var seen []ChatMessage
	c.OnMessage(func(m ChatMessage) { seen = append(seen, m) })
	if c.Send("hola") {
		t.Fatal("sent with nobody in session")
	}
	c.SetPeer("P")
	c.Handle("Q", []byte(`{"type":"chat","text":"not in session"}`))
	c.Handle("P", []byte(`{"type":"chat","text":"  necesito ayuda‮  "}`))
	if !c.Send("ya voy") || len(out) != 1 || out[0] != "P:chat:ya voy" {
		t.Fatalf("sent %v", out)
	}
	if len(seen) != 2 || !seen[0].FromViewer || seen[0].Text != "necesito ayuda" || seen[1].FromViewer {
		t.Fatalf("messages %+v", seen)
	}
	c.SetPeer("R") // a new session starts empty
	if len(c.Messages()) != 0 {
		t.Fatal("the last session's chat stayed")
	}
}

func TestTypingBothWays(t *testing.T) {
	var out []string
	c := NewChatService(func(peer string, b []byte) bool {
		out = append(out, peer+":"+string(b))
		return true
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	var typing []bool
	c.OnTyping(func(on bool) { typing = append(typing, on) })
	c.Typing(true) // nobody in session
	c.SetPeer("P")
	c.HandleTyping("Q", []byte(`{"type":"typing","on":true}`)) // not the viewer in session
	c.HandleTyping("P", []byte(`{"type":"typing","on":true}`))
	c.HandleTyping("P", []byte(`{"type":"typing","on":false}`))
	c.Typing(true)
	if len(typing) != 2 || !typing[0] || typing[1] {
		t.Fatalf("typing %v", typing)
	}
	if len(out) != 1 || out[0] != `P:{"type":"typing","on":true}` {
		t.Fatalf("sent %v", out)
	}
}
