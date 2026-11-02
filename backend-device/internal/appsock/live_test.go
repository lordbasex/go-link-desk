// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package appsock

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/lordlive"
)

func sendMsg(t *testing.T, c net.Conn, typ byte, data []byte) {
	t.Helper()
	b := make([]byte, 16+len(data))
	binary.LittleEndian.PutUint32(b, uint32(len(data)))
	b[4] = typ
	copy(b[16:], data)
	if _, err := c.Write(b); err != nil {
		t.Fatal(err)
	}
}

func TestAMutedCallSendsNothing(t *testing.T) {
	hub := lordlive.NewHub()
	l := &LiveCallMedia{Hub: hub}
	got := make(chan []byte, 4)
	v, err := l.StartEncodedVoice(func(p []byte) { got <- p })
	if err != nil {
		t.Fatal(err)
	}
	defer v.Stop()
	a, b := net.Pipe()
	if err := hub.Deliver("live 1", a); err != nil {
		t.Fatal(err)
	}
	v.Mute(true)
	sendMsg(t, b, lordlive.TypeMic, []byte{1})
	select {
	case <-got:
		t.Fatal("a muted microphone went out")
	case <-time.After(100 * time.Millisecond):
	}
	v.Mute(false)
	sendMsg(t, b, lordlive.TypeMic, []byte{2})
	select {
	case p := <-got:
		if p[0] != 2 {
			t.Fatalf("%v", p)
		}
	case <-time.After(time.Second):
		t.Fatal("nothing came")
	}
}

func TestACameraBeforeItsStreamAsksForAKeyframe(t *testing.T) {
	l := &LiveCallMedia{Hub: lordlive.NewHub()}
	asked := false
	l.ShowCamera("vp8", 0, []byte{1}, func() { asked = true })
	if !asked {
		t.Fatal("a picture with nowhere to go asked for nothing")
	}
}

func TestStoppingACameraTakesItsTagBack(t *testing.T) {
	hub := lordlive.NewHub()
	l := &LiveCallMedia{Hub: hub}
	stop, err := l.StartEncodedCamera("h264", func() bool { return false }, func([]byte, time.Duration) {})
	if err != nil {
		t.Fatal(err)
	}
	stop()
	a, b := net.Pipe()
	if err := hub.Deliver("camera 1", a); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Read(make([]byte, 1)); err == nil {
		t.Fatal("a link after the call stayed open")
	}
}
