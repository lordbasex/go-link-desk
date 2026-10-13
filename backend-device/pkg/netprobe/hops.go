// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package netprobe

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"net/netip"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// maxHops is how far Hops looks: this computer's router, a router in
// front of it, and a third in a cascade of three.
const maxHops = 3

// Hops are the first routers toward target, as the system's own traceroute
// finds them (traceroute on macOS, tracepath or traceroute on Linux,
// tracert on Windows; none needs administrator rights). Many routers do
// not answer UPnP or NAT-PMP, but every router answers here: a second hop
// on a private network is a second router in cascade.
func Hops(ctx context.Context, target string) []netip.Addr {
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip4", target)
	if err != nil || len(ips) == 0 {
		return nil
	}
	dst := ips[0].Unmap().String()
	for _, args := range traceCommands(dst) {
		path, err := exec.LookPath(args[0])
		if err != nil {
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
		cmd := exec.CommandContext(cctx, path, args[1:]...)
		hideWindow(cmd)
		out, _ := cmd.Output() // a timeout still leaves the hops it printed
		cancel()
		if hops := parseHops(out, maxHops); len(hops) > 0 {
			return hops
		}
	}
	return nil
}

var (
	hopLine = regexp.MustCompile(`^\s*(\d+)\??:?\s`)
	ipv4    = regexp.MustCompile(`\b(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\b`)
)

// parseHops reads any of the three tools' output: a line that starts with
// the hop's number, and the last IPv4 address on it (tracert puts it at
// the end, traceroute and tracepath after the number). Hops without an
// answer ("*") end the list, so the hops returned are the first ones in
// order with none missing.
func parseHops(out []byte, max int) []netip.Addr {
	found := map[int]netip.Addr{}
	s := bufio.NewScanner(bytes.NewReader(out))
	for s.Scan() {
		line := s.Text()
		m := hopLine.FindStringSubmatch(line)
		if m == nil || strings.Contains(line, "LOCALHOST") {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil || n < 1 || n > max {
			continue
		}
		if _, seen := found[n]; seen {
			continue
		}
		all := ipv4.FindAllString(line[len(m[0]):], -1)
		if len(all) == 0 {
			continue
		}
		if a, err := netip.ParseAddr(all[len(all)-1]); err == nil && a.Is4() {
			found[n] = a
		}
	}
	var hops []netip.Addr
	for i := 1; i <= max; i++ {
		a, ok := found[i]
		if !ok {
			break
		}
		hops = append(hops, a)
	}
	return hops
}
