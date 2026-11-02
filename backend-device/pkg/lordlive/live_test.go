// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build unix

package lordlive

import (
	"encoding/binary"
	"net"
	"os"
	"syscall"
	"testing"
	"time"
)

func readRaw(t *testing.T, c net.Conn) (byte, byte, int64, []byte) {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	h := make([]byte, headerSize)
	if _, err := readFull(c, h); err != nil {
		t.Fatal(err)
	}
	n := binary.LittleEndian.Uint32(h)
	d := make([]byte, n)
	if n > 0 {
		if _, err := readFull(c, d); err != nil {
			t.Fatal(err)
		}
	}
	return h[4], h[5], int64(binary.LittleEndian.Uint64(h[8:])), d
}

func readFull(c net.Conn, b []byte) (int, error) {
	got := 0
	for got < len(b) {
		n, err := c.Read(b[got:])
		got += n
		if err != nil {
			return got, err
		}
	}
	return got, nil
}

func TestCodecThenPictureOnTheWire(t *testing.T) {
	a, b := net.Pipe()
	s := NewStream(a)
	defer s.Close()
	if !s.Video("h264", true, 40*time.Millisecond, []byte{1, 2, 3}) {
		t.Fatal("dropped")
	}
	if ty, _, _, d := readRaw(t, b); ty != TypeCodec || string(d) != "h264" {
		t.Fatalf("first %d %q", ty, d)
	}
	ty, fl, pts, d := readRaw(t, b)
	if ty != TypeVideo || fl != FlagKey || pts != 40 || len(d) != 3 {
		t.Fatalf("picture %d %d %d %v", ty, fl, pts, d)
	}
}

func TestSlowHelperDropsUntilAKeyframe(t *testing.T) {
	a, b := net.Pipe()
	s := NewStream(a)
	defer s.Close()
	// nobody reads: the writer holds one, the queue takes queueLen
	lost := false
	for i := 0; i < queueLen+8; i++ {
		if !s.Video("vp8", false, 0, []byte{1}) {
			lost = true
		}
	}
	if !lost {
		t.Fatal("nothing was dropped")
	}
	for {
		_ = b.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		h := make([]byte, headerSize+1)
		if _, err := readFull(b, h); err != nil {
			break
		}
	}
	if s.Video("vp8", false, 0, []byte{1}) {
		t.Fatal("a picture after a gap went through without a keyframe")
	}
	if !s.Video("vp8", true, 0, []byte{1}) {
		t.Fatal("the keyframe was dropped")
	}
}

func TestReadRefusesAnEnormousLength(t *testing.T) {
	a, b := net.Pipe()
	s := NewStream(a)
	defer s.Close()
	go func() {
		h := make([]byte, headerSize)
		binary.LittleEndian.PutUint32(h, maxMessage+1)
		_, _ = b.Write(h)
	}()
	if _, err := s.Read(); err == nil {
		t.Fatal("read it")
	}
}

func TestHubGivesOnlyWhatIsWaited(t *testing.T) {
	h := NewHub()
	got := make(chan *Stream, 1)
	h.On(KindLive, 7, func(s *Stream) { got <- s })
	a, _ := net.Pipe()
	if err := h.Deliver("live 7", a); err != nil {
		t.Fatal(err)
	}
	select {
	case s := <-got:
		s.Close()
	case <-time.After(time.Second):
		t.Fatal("not delivered")
	}
	c, d := net.Pipe()
	if err := h.Deliver("live 8", c); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Read(make([]byte, 1)); err == nil {
		t.Fatal("a link nobody waits for stayed open")
	}
	e, _ := net.Pipe()
	if err := h.Deliver("../etc 1", e); err == nil {
		t.Fatal("a bad line was taken")
	}
}

func TestServeTakesADescriptor(t *testing.T) {
	sv, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_SEQPACKET, 0)
	if err != nil {
		t.Skip("no seqpacket here:", err)
	}
	stream, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	f := os.NewFile(uintptr(sv[1]), "s")
	c, err := net.FileConn(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	h := NewHub()
	got := make(chan *Stream, 1)
	h.On(KindCamera, 3, func(s *Stream) { got <- s })
	go h.serve(c.(*net.UnixConn))
	if err := syscall.Sendmsg(sv[0], []byte("camera 3"), syscall.UnixRights(stream[0]), nil, 0); err != nil {
		t.Fatal(err)
	}
	select {
	case s := <-got:
		s.Keyframe()
		buf := make([]byte, headerSize)
		if n, _ := syscall.Read(stream[1], buf); n != headerSize || buf[4] != TypeKeyframe {
			t.Fatalf("read %d %v", n, buf)
		}
		s.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("not delivered")
	}
	syscall.Close(sv[0])
	syscall.Close(stream[1])
}
