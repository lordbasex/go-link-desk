// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package netprobe

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"net/netip"
	"strings"
)

// parseRouteGet reads "gateway: 192.168.0.1" from macOS's route -n get.
func parseRouteGet(out []byte) netip.Addr {
	s := bufio.NewScanner(bytes.NewReader(out))
	for s.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(s.Text()), ":")
		if ok && k == "gateway" {
			if a, err := netip.ParseAddr(strings.TrimSpace(v)); err == nil && a.Is4() {
				return a
			}
		}
	}
	return netip.Addr{}
}

// parseProcRoute finds the default route (destination 00000000) in
// /proc/net/route, whose addresses are little-endian hex.
func parseProcRoute(b []byte) netip.Addr {
	s := bufio.NewScanner(bytes.NewReader(b))
	s.Scan() // header
	for s.Scan() {
		f := strings.Fields(s.Text())
		if len(f) < 3 || f[1] != "00000000" {
			continue
		}
		raw, err := hex.DecodeString(f[2])
		if err != nil || len(raw) != 4 {
			continue
		}
		var a [4]byte
		binary.BigEndian.PutUint32(a[:], binary.LittleEndian.Uint32(raw))
		if gw := netip.AddrFrom4(a); !gw.IsUnspecified() {
			return gw
		}
	}
	return netip.Addr{}
}
