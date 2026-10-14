// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package netprobe

import (
	"context"
	"net/netip"
	"os/exec"
	"time"
)

// Gateway is the default route's router, as route(8) says.
func Gateway() netip.Addr {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/sbin/route", "-n", "get", "default").Output()
	if err != nil {
		return netip.Addr{}
	}
	return parseRouteGet(out)
}
