// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package telemetry

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// clock is a settable time for the store.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time  { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) set(t time.Time) { c.mu.Lock(); c.t = t; c.mu.Unlock() }

func open(t *testing.T) (*Store, *clock) {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "telemetry.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	c := &clock{t: time.Date(2026, 10, 7, 21, 0, 0, 0, time.UTC)}
	s.now = c.now
	return s, c
}

var t0 = time.Date(2026, 10, 7, 21, 0, 0, 0, time.UTC)

func at(sec float64) time.Time { return t0.Add(time.Duration(sec * float64(time.Second))) }

func TestRunsSamplesAndEventsAreKeptPerRoom(t *testing.T) {
	s, c := open(t)
	r := s.Room("room1")
	r.Start("a1b2c3d4e5", "The Simpsons")
	r.Peer("peerA", "Nico")
	for i := 0; i < 10; i++ {
		c.set(at(float64(i)))
		r.Sample("", "room", Metrics{"fps_in": 60, "gap_max_ms": 17})
		r.Sample("peerA", "peer", Metrics{"rtt_ms": float64(20 + i)})
	}
	r.Event(Warn, "rr_loss", "peerA", "lost packets", map[string]any{"pct": 3})
	c.set(at(20))
	r.End("archived")
	s.Room("room2").Start("ffff", "Other")
	s.Sync()

	runs, err := s.Runs("room1")
	if err != nil || len(runs) != 1 || runs[0].ID != "a1b2c3d4e5" || runs[0].Game != "The Simpsons" || !runs[0].Ended.Equal(at(20)) {
		t.Fatalf("runs %+v %v", runs, err)
	}
	peers, _ := s.Peers("room1")
	if len(peers) != 1 || peers[0].Name != "Nico" {
		t.Fatalf("peers %+v", peers)
	}
	series, err := s.Series("room1", at(0), at(9), 5*time.Second, []string{"peer.rtt_ms"})
	if err != nil || len(series) != 1 || len(series[0].Avg) != 2 {
		t.Fatalf("series %+v %v", series, err)
	}
	if series[0].Avg[0] != 22 || series[0].Max[1] != 29 {
		t.Fatalf("avg %v max %v", series[0].Avg, series[0].Max)
	}
	warns, _ := s.Events("room1", at(0), at(30), 0, Warn)
	if len(warns) != 1 || warns[0].Kind != "rr_loss" || warns[0].Peer != "peerA" {
		t.Fatalf("warnings %+v", warns)
	}
	room, run, err := s.FindRun("#a1b2c3")
	if err != nil || room != "room1" || run.ID != "a1b2c3d4e5" {
		t.Fatalf("find %q %+v %v", room, run, err)
	}

	var out bytes.Buffer
	if err := s.Export(&out, "room1", at(0), at(30)); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if !strings.Contains(lines[0], `"type":"run"`) || !strings.Contains(out.String(), `"kind":"rr_loss"`) || len(lines) < 22 {
		t.Fatalf("export:\n%s", out.String())
	}

	if err := s.DeleteRoom("room1"); err != nil {
		t.Fatal(err)
	}
	if runs, _ := s.Runs("room1"); len(runs) != 0 {
		t.Fatal("the room's runs are still there")
	}
	if runs, _ := s.Runs("room2"); len(runs) != 1 {
		t.Fatal("deleting a room took another one's data")
	}
}

func TestTheDatabaseIsPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v %v", fi.Mode(), err)
	}
}

func TestAnOpenRunEndsWhenTheDeviceStartsAgain(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	r := s.Room("r")
	r.Start("x1", "Game")
	r.Sample("", "room", Metrics{"fps_in": 60})
	s.Close() // stopped without End
	s, err = Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if runs, _ := s.Runs("r"); len(runs) != 1 || !runs[0].Ended.IsZero() {
		t.Fatalf("opening (the CLI) changed an open run: %+v", runs)
	}
	if err := s.EndOpenRuns(); err != nil {
		t.Fatal(err)
	}
	runs, _ := s.Runs("r")
	if len(runs) != 1 || runs[0].Ended.IsZero() {
		t.Fatalf("runs %+v", runs)
	}
}

func TestIncidentsSayWhereAFreezeCameFrom(t *testing.T) {
	s, c := open(t)
	r := s.Room("room")
	r.Start("g1", "Game")
	// Three players, all fine, voice flowing.
	sample := func(sec float64, freeze map[string]float64, loss map[string]float64) {
		c.set(at(sec))
		for _, p := range []string{"host", "ana", "bea"} {
			r.Sample(p, "client", Metrics{"freeze_ms": freeze[p], "video_loss_pct": loss[p]})
			r.Sample(p, "peer", Metrics{"voice_in_pps": 50})
		}
	}
	for sec := 0.0; sec < 10; sec += 2 {
		sample(sec, nil, nil)
	}
	// At 11 s the game stops sending frames for 2 s: everyone freezes.
	c.set(at(11))
	r.Event(Warn, "frame_gap", "", "no frame from the game", map[string]any{"gap_ms": 2000})
	sample(12, map[string]float64{"host": 1900, "ana": 1950, "bea": 1900}, nil)
	for sec := 14.0; sec < 40; sec += 2 {
		sample(sec, nil, nil)
	}
	// At 40 s only Bea freezes and loses packets: her internet.
	sample(40, map[string]float64{"bea": 800}, map[string]float64{"bea": 9})
	for sec := 42.0; sec < 60; sec += 2 {
		sample(sec, nil, nil)
	}
	// At 60 s everyone freezes and loses packets: the host's upload.
	sample(60, map[string]float64{"host": 900, "ana": 900, "bea": 900}, map[string]float64{"host": 6, "ana": 7, "bea": 8})

	inc, err := s.Incidents("room", at(0), at(70))
	if err != nil {
		t.Fatal(err)
	}
	if len(inc) != 3 {
		t.Fatalf("%d incidents: %+v", len(inc), inc)
	}
	if inc[0].Verdict != VerdictDevice || inc[0].DeviceGapMs != 2000 || !inc[0].Voice {
		t.Fatalf("first %+v", inc[0])
	}
	if inc[1].Verdict != VerdictGuest || len(inc[1].Peers) != 1 || inc[1].Peers[0] != "bea" {
		t.Fatalf("second %+v", inc[1])
	}
	if inc[2].Verdict != VerdictHostNetwork {
		t.Fatalf("third %+v", inc[2])
	}
}

func TestANilRecorderRecordsNothing(t *testing.T) {
	var s *Store
	r := s.Room("x")
	r.Start("id", "g")
	r.Sample("", "room", Metrics{"a": 1})
	r.Event(Info, "k", "", "m", nil)
	r.End("done")
	if r.ID() != "" || s.DeleteRoom("x") != nil {
		t.Fatal("nil store")
	}
}
