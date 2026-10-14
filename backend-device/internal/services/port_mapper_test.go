// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"errors"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/netprobe"
)

// The router refuses first, then opens the port; stopping closes it.
func TestPortMapper(t *testing.T) {
	var calls, removed atomic.Int32
	changed := make(chan PortMapState, 4)
	p := &PortMapper{Port: 40000, retry: 10 * time.Millisecond}
	p.OnChange(func() { changed <- p.State() })
	p.mapUDP = func(_ context.Context, port int, lease time.Duration) (netprobe.Mapping, error) {
		if port != 40000 || lease != time.Hour {
			t.Errorf("asked %d for %v", port, lease)
		}
		if calls.Add(1) == 1 {
			return netprobe.Mapping{}, errors.New("no UPnP gateway answered")
		}
		return netprobe.Mapping{Via: "upnp", Port: 40000, Internal: 40000, External: netip.MustParseAddr("181.23.153.151"), Lifetime: time.Hour}, nil
	}
	p.unmap = func(_ context.Context, m netprobe.Mapping) error {
		if m.Port != 40000 {
			t.Errorf("removed %+v", m)
		}
		removed.Add(1)
		return nil
	}
	if a, _ := p.Outside(); a.IsValid() {
		t.Fatal("an outside address before asking")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- p.Run(ctx) }()
	if st := <-changed; st.Open() || st.Err == "" || !st.Asked {
		t.Fatalf("first: %+v", st)
	}
	if st := <-changed; !st.Open() || st.Via != "upnp" || st.Err != "" {
		t.Fatalf("second: %+v", st)
	}
	if a, port := p.Outside(); a != netip.MustParseAddr("181.23.153.151") || port != 40000 {
		t.Fatalf("outside %v:%d", a, port)
	}
	cancel()
	<-done
	if removed.Load() != 1 {
		t.Fatalf("removed %d times", removed.Load())
	}
}
