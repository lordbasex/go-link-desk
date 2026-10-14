// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"net"
	"strconv"
	"strings"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
)

// The direct connection's settings, shared by both Settings windows: one
// UDP port for every connection, opened in the router by the app or by
// hand, and the addresses where viewers reach it.

// parseUDPPort reads the port typed: empty or 0 is a random one.
func parseUDPPort(v string) (int, string) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, ""
	}
	p, err := strconv.Atoi(v)
	if err != nil || (p != 0 && (p < 1024 || p > 65535)) {
		return 0, L("The port is a number from 1024 to 65535 (0 or empty: a random one).")
	}
	return p, ""
}

// parseAnnounce reads the announced addresses: separated by commas,
// spaces or semicolons.
func parseAnnounce(v string) ([]string, string) {
	var ips []string
	for _, f := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' || r == ';' }) {
		if net.ParseIP(f) == nil {
			return nil, Lf("%s is not an IP address.", f)
		}
		ips = append(ips, f)
	}
	return ips, ""
}

// directChanged says the port, its mapping or the addresses changed since
// this run started: they apply when the app opens again.
func (u *ui) directChanged(cfg models.Config) bool {
	s := u.prefs.start
	return u.prefs.startSet && (cfg.UDPPort != s.UDPPort || cfg.MapUDPPort != s.MapUDPPort ||
		strings.Join(cfg.AnnounceIPs, ",") != strings.Join(s.AnnounceIPs, ","))
}

// portStatus tells what the router did with the port, when the app asks
// it to open it ("" when there is nothing to tell). tint is ok, warn or "".
func (u *ui) portStatus(cfg models.Config) (text, tint string) {
	if !cfg.MapUDPPort {
		return "", ""
	}
	if cfg.UDPPort == 0 {
		return L("Choose a port first: a random one cannot be opened in the router."), "warn"
	}
	p := u.opts.Ports
	if p == nil || u.directChanged(cfg) {
		return "", ""
	}
	st := p.State()
	switch {
	case !st.Asked:
		return L("Asking the router…"), ""
	case st.Open():
		via := "UPnP"
		if st.Via == "nat-pmp" {
			via = "NAT-PMP"
		}
		if st.Outside.IsValid() {
			return Lf("Open in the router (%s): %s, port %d.", via, st.Outside.String(), st.Port), "ok"
		}
		return Lf("Open in the router (%s): port %d.", via, st.Port), "ok"
	}
	return L("The router did not open it: turn on UPnP or NAT-PMP in it, or forward the port by hand."), "warn"
}
