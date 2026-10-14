// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"log/slog"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/netprobe"
)

// portLease is how long the router is asked to keep the port: it is asked
// again at half of it, and the port closes by itself an hour after the
// app stops without removing it.
const portLease = time.Hour

// PortMapper keeps the fixed UDP port (Config.UDPPort) open in the router
// with NAT-PMP or UPnP, when the user asked for it (Config.MapUDPPort): it
// asks when it starts, again before the lease ends and after a failure
// (the router may come back, the computer may move), and removes the port
// when it stops.
type PortMapper struct {
	Port int
	Log  *slog.Logger

	mapUDP func(context.Context, int, time.Duration) (netprobe.Mapping, error)
	unmap  func(context.Context, netprobe.Mapping) error
	retry  time.Duration // after a failure (default 5 min)

	mu       sync.Mutex
	state    PortMapState
	onChange []func()
}

// OnChange calls f after every change of State.
func (p *PortMapper) OnChange(f func()) {
	p.mu.Lock()
	p.onChange = append(p.onChange, f)
	p.mu.Unlock()
}

// PortMapState is what the router did with the port.
type PortMapState struct {
	Asked   bool   // a first answer (or its absence) came
	Via     string // "nat-pmp" or "upnp" when the port is open
	Port    int    // the outside port
	Outside netip.Addr
	Err     string // why it is not open
}

// Open says the router forwards the port now.
func (s PortMapState) Open() bool { return s.Via != "" }

// State is the latest answer.
func (p *PortMapper) State() PortMapState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

// Outside is the router's outside address while the port is open there.
func (p *PortMapper) Outside() (netip.Addr, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.state.Open() {
		return netip.Addr{}, 0
	}
	return p.state.Outside, p.state.Port
}

// Run keeps the port open until ctx ends, then removes it.
func (p *PortMapper) Run(ctx context.Context) error {
	mapUDP, unmap, retry := p.mapUDP, p.unmap, p.retry
	if mapUDP == nil {
		mapUDP, unmap = netprobe.MapUDP, netprobe.Unmap
	}
	if retry == 0 {
		retry = 5 * time.Minute
	}
	log := p.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	var (
		held   netprobe.Mapping
		failed bool
	)
	defer func() {
		if held.Via == "" {
			return
		}
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := unmap(c, held); err != nil {
			log.Warn("udp port: could not close it in the router", "port", held.Port, "via", held.Via, "err", err)
			return
		}
		log.Info("udp port closed in the router", "port", held.Port, "via", held.Via)
	}()
	for {
		c, cancel := context.WithTimeout(ctx, 10*time.Second)
		m, err := mapUDP(c, p.Port, portLease)
		cancel()
		if ctx.Err() != nil {
			if err == nil {
				held = m
			}
			return ctx.Err()
		}
		wait := retry
		st := PortMapState{Asked: true}
		if err != nil {
			st.Err = err.Error()
			if !failed {
				log.Warn("udp port: the router did not open it (UPnP and NAT-PMP)", "port", p.Port, "err", err)
			}
			failed = true
		} else {
			if held.Via == "" || failed || m.Port != held.Port || m.External != held.External {
				log.Info("udp port open in the router", "port", p.Port, "outside", m.External.String()+":"+strconv.Itoa(m.Port), "via", m.Via, "lease", m.Lifetime)
			}
			held, failed = m, false
			st.Via, st.Port, st.Outside = m.Via, m.Port, m.External
			if m.Lifetime > 0 {
				wait = m.Lifetime / 2
			} else {
				wait = portLease / 2 // permanent: still checked
			}
		}
		p.mu.Lock()
		changed := p.state != st
		p.state = st
		watchers := p.onChange
		p.mu.Unlock()
		if changed {
			for _, f := range watchers {
				f()
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}
