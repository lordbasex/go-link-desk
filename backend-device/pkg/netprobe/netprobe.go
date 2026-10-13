// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package netprobe finds what a browser cannot about this computer's
// network, for the connection test (desk.go-link.org/network,
// docs/network.md): its local address, its router (the default gateway)
// and the router's own outside address, which the router tells over
// NAT-PMP (RFC 6886) or UPnP IGD. A private or 100.64.0.0/10 outside
// address means another router, or the provider's CGNAT, is in front.
//
// Nothing here opens a port or changes the router: NAT-PMP's "external
// address" request and UPnP's GetExternalIPAddress only read.
package netprobe

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/netip"
	"sync"
	"time"
)

// Result is what the apps put after "#d=" in the test page's address.
type Result struct {
	Src string `json:"src"`
	LAN string `json:"lan,omitempty"`
	GW  string `json:"gw,omitempty"`
	WAN string `json:"wan,omitempty"`
	// Hops are the first routers toward go-link Desk's server.
	Hops []string `json:"hops,omitempty"`
	UPnP bool     `json:"upnp"`
	PMP  bool     `json:"pmp"`
	Net  string   `json:"net,omitempty"`
}

// Fragment is "#d=<base64url JSON>", which browsers never send to a server.
func (r Result) Fragment() string {
	b, _ := json.Marshal(r)
	return "#d=" + base64.RawURLEncoding.EncodeToString(b)
}

// Probe looks for everything at once, within ctx (a few seconds is enough:
// routers answer on the local network in milliseconds). target is the host
// whose first hops count (go-link Desk's signaling server).
func Probe(ctx context.Context, target string) Result {
	r := Result{Src: "desktop", Net: "other"}
	lan := LocalIP()
	if lan.IsValid() {
		r.LAN = lan.String()
		r.Net = interfaceKind(lan)
	}
	gw := Gateway()
	if gw.IsValid() {
		r.GW = gw.String()
	}
	var (
		mu   sync.Mutex
		wg   sync.WaitGroup
		pmp  netip.Addr
		upnp netip.Addr
	)
	if gw.IsValid() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if a, err := NATPMPExternal(ctx, gw); err == nil {
				mu.Lock()
				pmp = a
				mu.Unlock()
			}
		}()
	}
	var hops []netip.Addr
	if target != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h := Hops(ctx, target)
			mu.Lock()
			hops = h
			mu.Unlock()
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if a, err := UPnPExternal(ctx, gw); err == nil {
			mu.Lock()
			upnp = a
			mu.Unlock()
		}
	}()
	wg.Wait()
	r.PMP, r.UPnP = pmp.IsValid(), upnp.IsValid()
	for _, h := range hops {
		r.Hops = append(r.Hops, h.String())
	}
	switch {
	case upnp.IsValid():
		r.WAN = upnp.String()
	case pmp.IsValid():
		r.WAN = pmp.String()
	}
	return r
}

// LocalIP is the address this computer leaves the network with: the one a
// UDP "connection" to a public address picks. No packet is sent.
func LocalIP() netip.Addr {
	c, err := net.Dial("udp4", "192.0.2.1:9") // TEST-NET-1: never reached
	if err != nil {
		return netip.Addr{}
	}
	defer c.Close()
	if a, ok := c.LocalAddr().(*net.UDPAddr); ok {
		if ip, ok := netip.AddrFromSlice(a.IP.To4()); ok {
			return ip
		}
	}
	return netip.Addr{}
}

// interfaceKind says "wifi", "ethernet" or "other" for the interface that
// has this address, by its name (en0 on a Mac is usually Wi-Fi, but not
// always, so only names that say it count).
func interfaceKind(ip netip.Addr) string {
	ifs, err := net.Interfaces()
	if err != nil {
		return "other"
	}
	for _, i := range ifs {
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok {
				if p, ok := netip.AddrFromSlice(n.IP.To4()); ok && p == ip {
					return kindOfName(i.Name)
				}
			}
		}
	}
	return "other"
}

func kindOfName(name string) string {
	switch {
	case hasPrefix(name, "wl", "wlan", "Wi-Fi", "WLAN", "wifi"):
		return "wifi"
	case hasPrefix(name, "eth", "enp", "eno", "ens", "Ethernet"):
		return "ethernet"
	}
	return "other"
}

func hasPrefix(s string, ps ...string) bool {
	for _, p := range ps {
		if len(s) >= len(p) && s[:len(p)] == p {
			return true
		}
	}
	return false
}

// deadline is ctx's deadline, or d from now.
func deadline(ctx context.Context, d time.Duration) time.Time {
	if t, ok := ctx.Deadline(); ok {
		return t
	}
	return time.Now().Add(d)
}
