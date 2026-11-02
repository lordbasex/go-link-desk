// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package lordshare

import (
	"bytes"
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"
)

// peer is the system's end of a connection.
type peer struct {
	t *testing.T
	c net.Conn
}

func pair(t *testing.T) (*Client, peer) {
	t.Helper()
	a, b := net.Pipe()
	c := New(a)
	t.Cleanup(c.Close)
	return c, peer{t, b}
}

func (p peer) next() (byte, []byte) {
	p.t.Helper()
	_ = p.c.SetReadDeadline(time.Now().Add(2 * time.Second))
	ty, d, err := readFrame(p.c, nil)
	if err != nil {
		p.t.Fatal(err)
	}
	return ty, append([]byte(nil), d...)
}

func (p peer) send(t byte, d []byte) {
	p.t.Helper()
	if _, err := p.c.Write(frame(nil, t, d)); err != nil {
		p.t.Fatal(err)
	}
}

func u16s(v ...int) []byte {
	var b []byte
	for _, x := range v {
		b = binary.LittleEndian.AppendUint16(b, uint16(x))
	}
	return b
}

func TestStartIsWhatTheParserTakes(t *testing.T) {
	c, p := pair(t)
	go c.Start(FlagAll, CodecVP8, 30, 2500)
	ty, d := p.next()
	want := []byte{15, 2, 30, 0, 0xC4, 9, 0, 0}
	if ty != msgStart || !bytes.Equal(d, want) {
		t.Fatalf("start: %d % x", ty, d)
	}
	// Out of range goes in range: the server ends a connection for less.
	go c.Start(0, 9, 1000, 1<<30)
	_, d = p.next()
	if d[0] != FlagVideo || d[1] != CodecVP8 || binary.LittleEndian.Uint16(d[2:]) != MaxHz || binary.LittleEndian.Uint32(d[4:]) != MaxKbps {
		t.Fatalf("clamped: % x", d)
	}
}

func TestInputStaysInTheParsersRange(t *testing.T) {
	c, p := pair(t)
	go func() {
		c.Move(-5, 40000)
		c.Button(3, true) // has no RP_BTN: nothing goes
		c.Key(0, 1)       // not a key
		c.Key(1000, 1)    // nor this
		c.Key(30, 2)
		c.Wheel(0) // nothing
		c.Wheel(-300)
		c.Scroll(2, 1<<30, -1.5)
	}()
	ty, d := p.next()
	if ty != msgPtr || !bytes.Equal(d, u16s(0, 32767)) {
		t.Fatalf("move: %d % x", ty, d)
	}
	ty, d = p.next()
	if ty != msgKey || !bytes.Equal(d, []byte{30, 0, 2}) {
		t.Fatalf("key: %d % x", ty, d)
	}
	ty, d = p.next()
	if ty != msgWheel || int8(d[0]) != -127 {
		t.Fatalf("wheel: %d % x", ty, d)
	}
	ty, d = p.next()
	if ty != msgScroll || len(d) != 9 || int32(binary.LittleEndian.Uint32(d[1:])) != 1<<24 || int32(binary.LittleEndian.Uint32(d[5:])) != -384 {
		t.Fatalf("scroll: %d % x", ty, d)
	}
}

func TestTextIsCutAtCodePoints(t *testing.T) {
	c, p := pair(t)
	s := strings.Repeat("ñ", MaxText) // 2 bytes each
	go c.Text(s + "\x00")
	got := ""
	for len(got) < len(s) {
		ty, d := p.next()
		if ty != msgText || len(d) > MaxText || !strings.HasPrefix(s, got+string(d)) {
			t.Fatalf("piece of %d bytes", len(d))
		}
		got += string(d)
	}
}

func TestShareLife(t *testing.T) {
	c, p := pair(t)
	started := make(chan State, 1)
	ended := make(chan Reason, 1)
	frames := make(chan Video, 1)
	c.SetMedia(Media{
		Started: func(s State) { started <- s },
		Ended:   func(r Reason) { ended <- r },
		Video:   func(v Video) { v.Data = bytes.Clone(v.Data); frames <- v },
	})
	c.Hear(true)
	c.Awake(true)
	if c.Running() {
		t.Fatal("running before a start")
	}
	go c.Start(FlagAll, CodecH264, 30, 1000)
	p.next()
	// A query's answer while a prompt waits doesn't mean the share runs.
	state := []byte{0, 15, 3, 0, 0, 5, 32, 3, 60, 0}
	p.send(msgState, state)
	time.Sleep(20 * time.Millisecond)
	if c.Running() {
		t.Fatal("a state alone started the share")
	}
	p.send(msgSize, u16s(1280, 800))
	s := <-started
	if s.W != 1280 || s.H != 800 || s.Granted != FlagAll {
		t.Fatalf("state: %v", s)
	}
	// What was asked early is sent now: the system ignores it before.
	if ty, d := p.next(); ty != msgHear || d[0] != 1 {
		t.Fatalf("hear: %d", ty)
	}
	if ty, d := p.next(); ty != msgAwake || d[0] != 1 {
		t.Fatalf("awake: %d", ty)
	}
	p.send(msgVFrame, append([]byte{1, 1, 0, 0, 9, 0, 0, 0}, 0xAA))
	if v := <-frames; v.Codec != CodecH264 || !v.Key || v.Ms != 9 || !bytes.Equal(v.Data, []byte{0xAA}) {
		t.Fatalf("video: %+v", v)
	}
	p.send(msgEnd, []byte{byte(Locked)})
	if r := <-ended; r != Locked || r.String() != "locked" {
		t.Fatalf("end: %v", r)
	}
	if c.Running() {
		t.Fatal("still running")
	}
}

func TestRefused(t *testing.T) {
	c, p := pair(t)
	got := make(chan Reason, 1)
	c.SetMedia(Media{Refused: func(r Reason) { got <- r }})
	go c.Start(FlagVideo, CodecH264, 30, 1000)
	p.next()
	p.send(msgRefused, []byte{byte(RemoteDesktop)})
	if r := <-got; r.String() != "remote_desktop" {
		t.Fatal(r)
	}
	p.send(msgSize, u16s(800, 600)) // the session changing size starts nothing
	time.Sleep(20 * time.Millisecond)
	if c.Running() {
		t.Fatal("running after a refusal")
	}
}

func TestPointerShapeAndClipboard(t *testing.T) {
	c, p := pair(t)
	p.send(msgCursor, []byte{10, 0, 20, 0, 1})
	p.send(msgShape, append(u16s(1, 1, 0, 0), 1, 2, 3, 4))
	p.send(msgShape, u16s(9, 9, 0, 0)) // too short: ignored
	p.send(msgClip, []byte("hello"))
	deadline := time.Now().Add(time.Second)
	for {
		if txt, gen := c.Clipboard(); gen == 1 && txt == "hello" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no clipboard")
		}
		time.Sleep(time.Millisecond)
	}
	if v := c.Pointer(); v.X != 10 || v.Y != 20 || v.Mode != 1 {
		t.Fatalf("pointer %+v", v)
	}
	if s := c.Shape(); s.W != 1 || s.Gen != 1 || len(s.ARGB) != 4 {
		t.Fatalf("shape %+v", s)
	}
}

func TestConnectionGone(t *testing.T) {
	c, p := pair(t)
	ended := make(chan Reason, 1)
	c.SetMedia(Media{Ended: func(r Reason) { ended <- r }})
	go c.Start(FlagVideo, CodecH264, 30, 1000)
	p.next()
	p.send(msgSize, u16s(640, 480))
	p.next()
	p.next()
	_ = p.c.Close()
	if r := <-ended; r != EndedByApp {
		t.Fatal(r)
	}
	<-c.Done()
	c.Key(30, 1) // never blocks
	c.Move(1, 1)
}

func TestBadStream(t *testing.T) {
	a, b := net.Pipe()
	c := New(a)
	go func() { _, _ = b.Write([]byte{0, 0, 0, 0, 1}) }()
	select {
	case <-c.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("a zero length didn't end it")
	}
}

func TestReasonsMatchTheServer(t *testing.T) {
	if EndedByApp != 13 || NoIdentity.String() != "no_identity" || Reason(99).String() != "reason_99" {
		t.Fatal(EndedByApp)
	}
}

func TestCheck(t *testing.T) {
	c, p := pair(t)
	got := make(chan State, 1)
	go func() {
		st, _ := c.Check(2 * time.Second)
		got <- st
	}()
	if ty, _ := p.next(); ty != msgQuery {
		t.Fatal(ty)
	}
	p.send(msgState, []byte{byte(Busy), 0, 3, 0, 0, 5, 32, 3, 60, 0})
	if st := <-got; st.Blocked != Busy {
		t.Fatal(st)
	}
	if _, ok := c.Check(10 * time.Millisecond); ok {
		t.Fatal("an answer out of nowhere")
	}
}
