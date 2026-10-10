// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package sysinfo reads hardware details and live CPU/RAM usage. It uses
// gopsutil, which works on Windows, macOS and Linux without cgo.
package sysinfo

import (
	"context"
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

// Hardware is fixed information about the machine.
type Hardware struct {
	Hostname string `json:"hostname"` // the computer's name, shown as the device's name
	OS       string `json:"os"`       // "darwin", "windows", "linux"
	Arch     string `json:"arch"`     // "amd64", "arm64"
	Platform string `json:"platform"` // e.g. "darwin 15.6"
	CPUModel string `json:"cpu_model"`
	Cores    int    `json:"cores"`
	MemTotal uint64 `json:"mem_total"` // bytes
}

// Usage is a live sample.
type Usage struct {
	CPUPercent        float64 `json:"cpu_percent"`         // whole machine, 0-100
	ProcessCPUPercent float64 `json:"process_cpu_percent"` // this program and its games, 0-100 per core
	MemUsed           uint64  `json:"mem_used"`            // bytes, whole machine
	ProcessRSS        uint64  `json:"process_rss"`         // bytes, this program and its games
	NetSentBps        uint64  `json:"net_sent_bps"`        // bytes per second, all interfaces
	NetRecvBps        uint64  `json:"net_recv_bps"`        // bytes per second, all interfaces
}

// ReadHardware collects the fixed details. Missing values stay empty.
func ReadHardware(ctx context.Context) Hardware {
	hw := Hardware{OS: runtime.GOOS, Arch: runtime.GOARCH, Cores: runtime.NumCPU()}
	if info, err := host.InfoWithContext(ctx); err == nil {
		hw.Platform = info.Platform + " " + info.PlatformVersion
		hw.Hostname = info.Hostname
	}
	if infos, err := cpu.InfoWithContext(ctx); err == nil && len(infos) > 0 {
		hw.CPUModel = infos[0].ModelName
	}
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		hw.MemTotal = vm.Total
	}
	return hw
}

// Disk is the space of the volume that holds a folder.
type Disk struct {
	Total uint64 `json:"total"` // bytes
	Free  uint64 `json:"free"`  // bytes available to this user
}

// DiskOf measures the volume that holds path; ok is false when it cannot.
func DiskOf(path string) (Disk, bool) {
	u, err := disk.Usage(path)
	if err != nil || u.Total == 0 {
		return Disk{}, false
	}
	return Disk{Total: u.Total, Free: u.Free}, true
}

// Sampler measures usage between consecutive calls.
type Sampler struct {
	proc     *process.Process
	children map[int32]*process.Process // each game runs as a child process
	lastSent uint64
	lastRecv uint64
	lastAt   time.Time
}

// NewSampler prepares a sampler for this process.
func NewSampler() *Sampler {
	p, _ := process.NewProcess(int32(os.Getpid()))
	s := &Sampler{proc: p, children: map[int32]*process.Process{}}
	s.Sample(context.Background()) // prime the CPU counters
	return s
}

// Sample returns usage since the previous call.
func (s *Sampler) Sample(ctx context.Context) Usage {
	var u Usage
	if pct, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(pct) > 0 {
		u.CPUPercent = pct[0]
	}
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		u.MemUsed = vm.Used
	}
	if s.proc != nil {
		if pct, err := s.proc.PercentWithContext(ctx, 0); err == nil {
			u.ProcessCPUPercent = pct
		}
		if mi, err := s.proc.MemoryInfoWithContext(ctx); err == nil {
			u.ProcessRSS = mi.RSS
		}
		kidsCPU, kidsRSS := s.sampleChildren(ctx)
		u.ProcessCPUPercent += kidsCPU
		u.ProcessRSS += kidsRSS
	}
	// Network traffic: the difference since the previous sample.
	if io, err := net.IOCountersWithContext(ctx, false); err == nil && len(io) > 0 {
		now := time.Now()
		if !s.lastAt.IsZero() && io[0].BytesSent >= s.lastSent && io[0].BytesRecv >= s.lastRecv {
			if secs := now.Sub(s.lastAt).Seconds(); secs > 0 {
				u.NetSentBps = uint64(float64(io[0].BytesSent-s.lastSent) / secs)
				u.NetRecvBps = uint64(float64(io[0].BytesRecv-s.lastRecv) / secs)
			}
		}
		s.lastSent, s.lastRecv, s.lastAt = io[0].BytesSent, io[0].BytesRecv, now
	}
	return u
}

// sampleChildren adds up the game processes. A process is kept between
// samples: its CPU percent is measured since the previous one.
func (s *Sampler) sampleChildren(ctx context.Context) (cpuPct float64, rss uint64) {
	kids, err := s.proc.ChildrenWithContext(ctx)
	if err != nil {
		kids = nil // no children (or cannot list them)
	}
	alive := make(map[int32]bool, len(kids))
	for _, k := range kids {
		alive[k.Pid] = true
		p, ok := s.children[k.Pid]
		if !ok {
			p = k
			s.children[k.Pid] = p
		}
		if pct, err := p.PercentWithContext(ctx, 0); err == nil {
			cpuPct += pct
		}
		if mi, err := p.MemoryInfoWithContext(ctx); err == nil {
			rss += mi.RSS
		}
	}
	for pid := range s.children {
		if !alive[pid] {
			delete(s.children, pid)
		}
	}
	return cpuPct, rss
}
