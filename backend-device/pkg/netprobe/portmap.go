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

// Mapping is a UDP port the router forwards to this computer, asked for
// with NAT-PMP or UPnP. It lasts Lifetime (0: until it is removed); keep it
// with MapUDP again before then, and remove it with Unmap.
type Mapping struct {
	Via      string     // "nat-pmp" or "upnp"
	External netip.Addr // the router's outside address; invalid if it did not say
	Port     int        // the outside port
	Internal int        // this computer's port
	Lifetime time.Duration

	gw  netip.Addr // NAT-PMP: the router
	wan wan        // UPnP: its WAN connection service
}

// PortDescription names the mapping in the router's list.
const PortDescription = "go-link Desk"

// MapUDP asks the router to forward UDP port (outside) to the same port on
// this computer, for lifetime: NAT-PMP first (one packet), then UPnP. Only
// this computer is named, never another address.
func MapUDP(ctx context.Context, port int, lifetime time.Duration) (Mapping, error) {
	if port < 1 || port > 65535 {
		return Mapping{}, errors.New("netprobe: no port to map")
	}
	gw := Gateway()
	var errs []error
	if gw.IsValid() {
		m, err := natpmpMap(ctx, gw, port, port, lifetime)
		if err == nil {
			m.External, _ = NATPMPExternal(ctx, gw)
			return m, nil
		}
		errs = append(errs, err)
	}
	lan := LocalIP()
	if !lan.IsValid() {
		return Mapping{}, errors.New("netprobe: this computer has no address")
	}
	for w, err := range wanConnections(ctx, gw) {
		if err != nil {
			errs = append(errs, err)
			break
		}
		m, err := upnpMap(ctx, w, lan, port, lifetime)
		if err == nil {
			m.External, _ = w.external(ctx)
			return m, nil
		}
		errs = append(errs, err)
	}
	return Mapping{}, errors.Join(errs...)
}

// Unmap removes a mapping from the router.
func Unmap(ctx context.Context, m Mapping) error {
	switch m.Via {
	case "nat-pmp":
		// RFC 6886 3.4: lifetime 0 and outside port 0 remove it.
		_, err := natpmpMap(ctx, m.gw, m.Internal, 0, 0)
		return err
	case "upnp":
		_, err := m.wan.call(ctx, "DeletePortMapping",
			[2]string{"NewRemoteHost", ""},
			[2]string{"NewExternalPort", strconv.Itoa(m.Port)},
			[2]string{"NewProtocol", "UDP"})
		return err
	}
	return nil
}

// natpmpMap sends a UDP mapping request (opcode 1).
func natpmpMap(ctx context.Context, gw netip.Addr, internal, external int, lifetime time.Duration) (Mapping, error) {
	c, err := net.Dial("udp4", net.JoinHostPort(gw.String(), strconv.Itoa(natpmpPort)))
	if err != nil {
		return Mapping{}, err
	}
	defer c.Close()
	req := make([]byte, 12)
	req[1] = 1 // map UDP
	binary.BigEndian.PutUint16(req[4:], uint16(internal))
	binary.BigEndian.PutUint16(req[6:], uint16(external))
	binary.BigEndian.PutUint32(req[8:], uint32(lifetime/time.Second))
	end := deadline(ctx, 2*time.Second)
	buf := make([]byte, 16)
	for wait := 250 * time.Millisecond; time.Now().Before(end); wait *= 2 {
		if _, err := c.Write(req); err != nil {
			return Mapping{}, err
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
			return Mapping{}, err
		}
		m, err := parseNATPMPMap(buf[:n], internal)
		m.gw = gw
		return m, err
	}
	return Mapping{}, errors.New("netprobe: no NAT-PMP answer")
}

// parseNATPMPMap reads a UDP mapping response: version 0, opcode 129,
// result, epoch, the internal and outside ports, the lifetime granted.
func parseNATPMPMap(b []byte, internal int) (Mapping, error) {
	if len(b) < 16 || b[0] != 0 || b[1] != 129 {
		return Mapping{}, errors.New("netprobe: not a NAT-PMP answer")
	}
	if code := binary.BigEndian.Uint16(b[2:4]); code != 0 {
		return Mapping{}, errors.New("netprobe: NAT-PMP refused the port (" + strconv.Itoa(int(code)) + ")")
	}
	if int(binary.BigEndian.Uint16(b[8:10])) != internal {
		return Mapping{}, errors.New("netprobe: NAT-PMP answered for another port")
	}
	return Mapping{
		Via:      "nat-pmp",
		Internal: internal,
		Port:     int(binary.BigEndian.Uint16(b[10:12])),
		Lifetime: time.Duration(binary.BigEndian.Uint32(b[12:16])) * time.Second,
	}, nil
}

// upnpMap calls AddPortMapping for the same port outside and in. A router
// that keeps only permanent mappings (error 725) gets one without a lease,
// removed by Unmap.
func upnpMap(ctx context.Context, w wan, lan netip.Addr, port int, lifetime time.Duration) (Mapping, error) {
	add := func(lease time.Duration) error {
		_, err := w.call(ctx, "AddPortMapping",
			[2]string{"NewRemoteHost", ""},
			[2]string{"NewExternalPort", strconv.Itoa(port)},
			[2]string{"NewProtocol", "UDP"},
			[2]string{"NewInternalPort", strconv.Itoa(port)},
			[2]string{"NewInternalClient", lan.String()},
			[2]string{"NewEnabled", "1"},
			[2]string{"NewPortMappingDescription", PortDescription},
			[2]string{"NewLeaseDuration", strconv.Itoa(int(lease / time.Second))})
		return err
	}
	err := add(lifetime)
	var ue *upnpError
	if errors.As(err, &ue) && ue.Code == 725 && lifetime > 0 {
		lifetime = 0
		err = add(0)
	}
	if err != nil {
		return Mapping{}, err
	}
	return Mapping{Via: "upnp", Internal: port, Port: port, Lifetime: lifetime, wan: w}, nil
}
