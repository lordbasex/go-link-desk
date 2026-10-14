// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package netprobe

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"strconv"
	"time"
)

// natpmpPort is where a router listens for NAT-PMP (RFC 6886) and PCP.
var natpmpPort = 5351

// NATPMPExternal asks the router for its outside address (NAT-PMP opcode
// 0). It reads only: no mapping is made.
func NATPMPExternal(ctx context.Context, gw netip.Addr) (netip.Addr, error) {
	c, err := net.Dial("udp4", net.JoinHostPort(gw.String(), strconv.Itoa(natpmpPort)))
	if err != nil {
		return netip.Addr{}, err
	}
	defer c.Close()
	end := deadline(ctx, 2*time.Second)
	buf := make([]byte, 16)
	// RFC 6886 3.1: retry at 250 ms, doubling.
	for wait := 250 * time.Millisecond; time.Now().Before(end); wait *= 2 {
		if _, err := c.Write([]byte{0, 0}); err != nil {
			return netip.Addr{}, err
		}
		t := time.Now().Add(wait)
		if t.After(end) {
			t = end
		}
		_ = c.SetReadDeadline(t)
		n, err := c.Read(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return netip.Addr{}, err
		}
		return parseNATPMP(buf[:n])
	}
	return netip.Addr{}, errors.New("netprobe: no NAT-PMP answer")
}

// parseNATPMP reads an external address response: version 0, opcode 128,
// result 0, the epoch, then the address.
func parseNATPMP(b []byte) (netip.Addr, error) {
	if len(b) < 12 || b[0] != 0 || b[1] != 128 {
		return netip.Addr{}, errors.New("netprobe: not a NAT-PMP answer")
	}
	if code := binary.BigEndian.Uint16(b[2:4]); code != 0 {
		return netip.Addr{}, errors.New("netprobe: NAT-PMP refused (" + strconv.Itoa(int(code)) + ")")
	}
	a := netip.AddrFrom4([4]byte(b[8:12]))
	if a.IsUnspecified() {
		return netip.Addr{}, errors.New("netprobe: the router has no outside address")
	}
	return a, nil
}
