// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package sysinfo

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestReadHardware(t *testing.T) {
	hw := ReadHardware(context.Background())
	if hw.OS != runtime.GOOS || hw.Cores < 1 || hw.MemTotal == 0 {
		t.Fatalf("hardware %+v", hw)
	}
}

func TestSample(t *testing.T) {
	s := NewSampler()
	time.Sleep(200 * time.Millisecond)
	u := s.Sample(context.Background())
	if u.MemUsed == 0 || u.ProcessRSS == 0 || u.CPUPercent < 0 || u.CPUPercent > 100 {
		t.Fatalf("usage %+v", u)
	}
}

// TestHelperBusy is the child process of TestTheGamesCountInTheProcessCPU:
// it only burns one core for a while.
func TestHelperBusy(t *testing.T) {
	if os.Getenv("SYSINFO_BUSY") != "1" {
		t.Skip("helper process")
	}
	end := time.Now().Add(3 * time.Second)
	n := 0
	for time.Now().Before(end) {
		n++
	}
	_ = n
}

func TestTheGamesCountInTheProcessCPU(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperBusy$")
	cmd.Env = append(os.Environ(), "SYSINFO_BUSY=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Wait() }()
	s := NewSampler()
	time.Sleep(time.Second)
	u := s.Sample(context.Background())
	// This test process idles: the busy child is what shows.
	if u.ProcessCPUPercent < 30 {
		t.Fatalf("process CPU %.1f%%: the child process is not counted", u.ProcessCPUPercent)
	}
	if len(s.children) != 1 {
		t.Fatalf("children tracked: %d", len(s.children))
	}
}
