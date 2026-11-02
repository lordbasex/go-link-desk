// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package appsock

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"
)

// app is the app's end of a socket.
type app struct {
	t *testing.T
	c net.Conn
	s map[string]json.RawMessage
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newApp(t *testing.T) (*app, *Sock) {
	t.Helper()
	a, b := net.Pipe()
	s := New(b, quiet())
	t.Cleanup(func() { s.Close(); _ = a.Close() })
	return &app{t: t, c: a, s: map[string]json.RawMessage{}}, s
}

func (a *app) read() map[string]json.RawMessage {
	a.t.Helper()
	_ = a.c.SetReadDeadline(time.Now().Add(3 * time.Second))
	b, err := ReadFrame(a.c)
	if err != nil {
		a.t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		a.t.Fatal(err)
	}
	if string(m["t"]) == `"state"` {
		var set map[string]json.RawMessage
		_ = json.Unmarshal(m["set"], &set)
		for k, v := range set {
			a.s[k] = v
		}
	}
	return m
}

func (a *app) send(m map[string]any) {
	a.t.Helper()
	b, _ := json.Marshal(m)
	if err := WriteFrame(a.c, b); err != nil {
		a.t.Fatal(err)
	}
}

func TestFirstFrameHasEverything(t *testing.T) {
	a, s := newApp(t)
	s.Set("a", 1)
	s.Set("b", "x")
	go func() { _ = s.Serve() }()
	a.read()
	if string(a.s["a"]) != "1" || string(a.s["b"]) != `"x"` {
		t.Fatal(a.s)
	}
}

func TestOnlyChangesGoOut(t *testing.T) {
	a, s := newApp(t)
	s.Set("a", 1)
	go func() { _ = s.Serve() }()
	a.read()
	s.Set("a", 1)
	s.Set("b", 2)
	if m := a.read(); strings.Contains(string(m["set"]), `"a"`) {
		t.Fatal(string(m["set"]))
	}
}

func TestRequestsAreAnswered(t *testing.T) {
	a, s := newApp(t)
	s.Handle("yes", func(Msg) (any, error) { return map[string]int{"n": 1}, nil })
	s.Handle("no", func(Msg) (any, error) { return nil, Reason("outside_home") })
	s.HandleSlow("slow", func(Msg) (any, error) { return "ok", nil })
	go func() { _ = s.Serve() }()
	a.send(map[string]any{"t": "yes", "id": 1})
	if m := a.read(); string(m["data"]) != `{"n":1}` {
		t.Fatal(m)
	}
	a.send(map[string]any{"t": "no", "id": 2})
	if m := a.read(); string(m["err"]) != `"outside_home"` {
		t.Fatal(m)
	}
	a.send(map[string]any{"t": "slow", "id": 3})
	if m := a.read(); string(m["data"]) != `"ok"` {
		t.Fatal(m)
	}
	a.send(map[string]any{"t": "what", "id": 4})
	if m := a.read(); string(m["err"]) != `"unknown"` {
		t.Fatal(m)
	}
}

func TestLongFrameClosesTheSocket(t *testing.T) {
	a, s := newApp(t)
	done := make(chan error, 1)
	go func() { done <- s.Serve() }()
	go func() {
		var hd [4]byte
		binary.LittleEndian.PutUint32(hd[:], MaxFrame+1)
		_, _ = a.c.Write(hd[:])
	}()
	if err := <-done; !errors.Is(err, ErrFrame) {
		t.Fatal(err)
	}
	<-s.Closed()
}

func TestNotJSONClosesTheSocket(t *testing.T) {
	a, s := newApp(t)
	done := make(chan error, 1)
	go func() { done <- s.Serve() }()
	go func() { _ = WriteFrame(a.c, []byte("hello")) }()
	if err := <-done; !errors.Is(err, ErrFrame) {
		t.Fatal(err)
	}
}

func TestEndOfFileEndsServe(t *testing.T) {
	a, s := newApp(t)
	done := make(chan error, 1)
	go func() { done <- s.Serve() }()
	_ = a.c.Close()
	if err := <-done; err == nil {
		t.Fatal("no error")
	}
	<-s.Closed()
}

func TestTooLongAListLosesItsOldest(t *testing.T) {
	a, s := newApp(t)
	long := make([]string, 200)
	for i := range long {
		long[i] = strings.Repeat("x", 500)
	}
	long[199] = "last"
	s.Set("chat", long)
	go func() { _ = s.Serve() }()
	a.read()
	var got []string
	if err := json.Unmarshal(a.s["chat"], &got); err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || len(got) == 200 || got[len(got)-1] != "last" {
		t.Fatal(len(got))
	}
}

func TestStateSplitsAcrossFrames(t *testing.T) {
	a, s := newApp(t)
	big := strings.Repeat("y", 40<<10)
	s.Set("a", big)
	s.Set("b", big)
	go func() { _ = s.Serve() }()
	a.read()
	a.read()
	if len(a.s) != 2 {
		t.Fatal(len(a.s))
	}
}
