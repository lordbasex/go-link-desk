// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package netprobe

import (
	"net/netip"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Gateway is the first IPv4 gateway of an adapter that is up, preferring
// the one that has this computer's outgoing address.
func Gateway() netip.Addr {
	size := uint32(15 << 10)
	var buf []byte
	for i := 0; i < 3; i++ {
		buf = make([]byte, size)
		err := windows.GetAdaptersAddresses(windows.AF_INET, windows.GAA_FLAG_INCLUDE_GATEWAYS|windows.GAA_FLAG_SKIP_ANYCAST|windows.GAA_FLAG_SKIP_MULTICAST, 0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
		if err == nil {
			break
		}
		if err != windows.ERROR_BUFFER_OVERFLOW {
			return netip.Addr{}
		}
	}
	lan := LocalIP()
	var first netip.Addr
	for a := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); a != nil; a = a.Next {
		if a.OperStatus != windows.IfOperStatusUp || a.FirstGatewayAddress == nil {
			continue
		}
		gw := sockaddr4(a.FirstGatewayAddress.Address)
		if !gw.IsValid() {
			continue
		}
		if !first.IsValid() {
			first = gw
		}
		for u := a.FirstUnicastAddress; u != nil; u = u.Next {
			if sockaddr4(u.Address) == lan {
				return gw
			}
		}
	}
	return first
}

func sockaddr4(s windows.SocketAddress) netip.Addr {
	if s.Sockaddr == nil || s.Sockaddr.Addr.Family != windows.AF_INET {
		return netip.Addr{}
	}
	in := (*windows.RawSockaddrInet4)(unsafe.Pointer(s.Sockaddr))
	return netip.AddrFrom4(in.Addr)
}
