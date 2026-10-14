// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package netprobe

import (
	"net/netip"
	"os"
)

// Gateway is the default route's router, from /proc/net/route.
func Gateway() netip.Addr {
	b, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return netip.Addr{}
	}
	return parseProcRoute(b)
}
