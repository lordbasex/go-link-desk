// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build lordos

package services

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/lordshare"
)

type fakeStream struct {
	mu      sync.Mutex
	want    EncodedWant
	hearing bool
	notices []string
}

func (f *fakeStream) EncodedWant() EncodedWant {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.want
}
func (f *fakeStream) set(w EncodedWant) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.want = w
}
func (f *fakeStream) EncodedFrame(string, bool, time.Duration, []byte, int, int) {}
func (f *fakeStream) EncodedAudio([]byte, time.Duration)                         {}
func (f *fakeStream) Hearing() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hearing
}
func (f *fakeStream) EncodedResized() {}
func (f *fakeStream) Notice(c string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notices = append(f.notices, c)
}

type fakeDriver struct {
	mu   sync.Mutex
	got  []string
	done chan struct{}
}

func (d *fakeDriver) add(s string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.got = append(d.got, s)
}
func (d *fakeDriver) Start(flags, codec, hz, kbps int) {
	if flags != lordshare.FlagAll || hz != encodedHz {
		panic("start")
	}
	if codec == lordshare.CodecH264 {
		d.add("start h264")
	} else {
		d.add("start vp8")
	}
}
func (d *fakeDriver) Stop()                 { d.add("stop") }
func (d *fakeDriver) Keyframe()             { d.add("key") }
func (d *fakeDriver) Rate(int)              { d.add("rate") }
func (d *fakeDriver) Hear(on bool)          { d.add(map[bool]string{true: "hear", false: "mute"}[on]) }
func (d *fakeDriver) Done() <-chan struct{} { return d.done }
func (d *fakeDriver) list() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.got...)
}

// harness runs drive with manual ticks.
type harness struct {
	s    *fakeStream
	d    *fakeDriver
	tick chan time.Time
	over chan lordshare.Reason
	ends chan string
	stop func()
}

func newHarness(t *testing.T) *harness {
	h := &harness{s: &fakeStream{}, d: &fakeDriver{done: make(chan struct{})}, tick: make(chan time.Time), over: make(chan lordshare.Reason), ends: make(chan string, 4)}
	e := &EncodedSource{Stream: h.s, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), End: func(r string) { h.ends <- r }}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.drive(ctx, h.over, h.d, h.tick); close(done) }()
	h.stop = func() { cancel(); <-done }
	t.Cleanup(h.stop)
	return h
}

func (h *harness) step() { h.tick <- time.Now(); h.tick <- time.Now() }

func has(l []string, s string) int {
	n := 0
	for _, v := range l {
		if v == s {
			n++
		}
	}
	return n
}

func TestNoViewerNoShare(t *testing.T) {
	h := newHarness(t)
	h.step()
	if len(h.d.list()) != 0 {
		t.Fatal(h.d.list())
	}
}

func TestShareLastsAsLongAsTheViewer(t *testing.T) {
	h := newHarness(t)
	h.s.set(EncodedWant{Viewers: 1, Codec: CodecH264, Kbps: 2000})
	h.step()
	if has(h.d.list(), "start h264") != 1 {
		t.Fatal(h.d.list())
	}
	h.s.set(EncodedWant{Viewers: 0})
	h.step()
	if has(h.d.list(), "stop") != 1 {
		t.Fatal(h.d.list())
	}
}

func TestFollowsTheViewer(t *testing.T) {
	h := newHarness(t)
	h.s.set(EncodedWant{Viewers: 1, Codec: CodecVP8, Kbps: 2000})
	h.step()
	h.s.set(EncodedWant{Viewers: 1, Codec: CodecVP8, Kbps: 900, NeedKey: true})
	h.s.mu.Lock()
	h.s.hearing = true
	h.s.mu.Unlock()
	h.step()
	l := h.d.list()
	if has(l, "start vp8") != 1 || has(l, "rate") != 1 || has(l, "hear") != 1 {
		t.Fatal(l)
	}
	if n := has(l, "key"); n != 1 { // two ticks, one every 500 ms
		t.Fatal(l)
	}
}

func TestTakenAwayEndsTheSessionAndDoesNotRetry(t *testing.T) {
	for _, r := range []lordshare.Reason{lordshare.Stopped, lordshare.Locked, lordshare.Revoked, lordshare.Denied, lordshare.RemoteDesktop} {
		h := newHarness(t)
		h.s.set(EncodedWant{Viewers: 1, Codec: CodecH264, Kbps: 2000})
		h.step()
		h.over <- r
		if got := <-h.ends; got != r.String() {
			t.Fatal(got)
		}
		h.step()
		if has(h.d.list(), "start h264") != 1 {
			t.Fatalf("%s: it started again: %v", r, h.d.list())
		}
		h.stop()
	}
}

func TestOwnStopIsNoEnd(t *testing.T) {
	h := newHarness(t)
	h.over <- lordshare.EndedByApp
	h.step()
	if len(h.ends) != 0 {
		t.Fatal("ended")
	}
}

func TestEncodedCodecNeverHigh(t *testing.T) {
	if EncodedCodec([]string{CodecH264High, CodecH264, CodecVP8}) != CodecH264 {
		t.Fatal("should be Baseline")
	}
	if EncodedCodec([]string{CodecH264High, CodecVP8}) != CodecVP8 {
		t.Fatal("High isn't what the session sends")
	}
}
