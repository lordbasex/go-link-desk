// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !darwin && !linux && !windows

package netprobe

import "net/netip"

// Gateway is not known on this system; UPnP still finds the router.
func Gateway() netip.Addr { return netip.Addr{} }
