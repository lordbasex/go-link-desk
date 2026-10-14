// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"context"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/netprobe"
)

// netTesting is set while "Test my connection…" looks at the network, so a
// second click does not start another.
var netTesting atomic.Bool

// testConnection is "Test my connection…": it finds what a browser cannot
// (this computer's local address, its router, the router's outside
// address and the first hops toward the signaling server) and opens the
// connection test page with it after "#d=", which browsers never send to
// a server (docs/network.md).
func (u *ui) testConnection() {
	if !netTesting.CompareAndSwap(false, true) {
		return
	}
	defer netTesting.Store(false)
	ctx, cancel := context.WithTimeout(u.opts.Ctx, 8*time.Second)
	defer cancel()
	r := netprobe.Probe(ctx, signalHost(u.opts.SignalURL))
	// The port this run uses, not one set since.
	if u.prefs.startSet {
		r.Port = u.prefs.start.UDPPort
	}
	if p := u.opts.Ports; p != nil && p.State().Open() {
		r.Open = p.State().Via
	}
	u.opts.Logger.Info("connection test", "lan", r.LAN, "gw", r.GW, "wan", r.WAN, "hops", r.Hops, "upnp", r.UPnP, "natpmp", r.PMP, "udp_port", r.Port, "open", r.Open)
	page, err := url.Parse(networkPage(u.opts.WebURL) + r.Fragment())
	if err != nil {
		return
	}
	fyne.Do(func() { _ = u.app.OpenURL(page) })
}

// networkPage is the test on this host's website.
func networkPage(web string) string {
	if web == "" {
		web = models.DefaultWebURL
	}
	return strings.TrimRight(web, "/") + "/network"
}

// signalHost is the signaling server's host name, whose first hops count;
// the official one when none is set.
func signalHost(signal string) string {
	if u, err := url.Parse(signal); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	return "signal.go-link.org"
}
