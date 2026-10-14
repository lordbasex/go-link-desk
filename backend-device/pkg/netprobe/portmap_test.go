// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package netprobe

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

// A fake router grants a NAT-PMP mapping for another outside port, then
// removes it.
func TestNATPMPMap(t *testing.T) {
	c, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	old := natpmpPort
	natpmpPort = c.LocalAddr().(*net.UDPAddr).Port
	defer func() { natpmpPort = old }()
	got := make(chan []byte, 2)
	go func() {
		buf := make([]byte, 16)
		for {
			n, from, err := c.ReadFrom(buf)
			if err != nil {
				return
			}
			req := append([]byte(nil), buf[:n]...)
			got <- req
			resp := make([]byte, 16)
			resp[1] = 129
			copy(resp[8:10], req[4:6])            // internal port
			copy(resp[10:12], []byte{0x9c, 0x41}) // outside 40001
			copy(resp[12:16], req[8:12])          // lifetime as asked
			_, _ = c.WriteTo(resp, from)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	m, err := natpmpMap(ctx, netip.MustParseAddr("127.0.0.1"), 40000, 40000, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if m.Via != "nat-pmp" || m.Internal != 40000 || m.Port != 40001 || m.Lifetime != time.Hour {
		t.Fatalf("%+v", m)
	}
	req := <-got
	if len(req) != 12 || req[1] != 1 || binary.BigEndian.Uint16(req[4:]) != 40000 || binary.BigEndian.Uint32(req[8:]) != 3600 {
		t.Fatalf("request % x", req)
	}
	if err := Unmap(ctx, m); err != nil {
		t.Fatal(err)
	}
	req = <-got
	if binary.BigEndian.Uint16(req[6:]) != 0 || binary.BigEndian.Uint32(req[8:]) != 0 {
		t.Fatalf("removal % x", req)
	}
	if _, err := parseNATPMPMap([]byte{0, 129, 0, 2, 0, 0, 0, 0, 0x9c, 0x40, 0, 0, 0, 0, 0, 0}, 40000); err == nil {
		t.Fatal("a refused mapping was taken")
	}
	if _, err := parseNATPMPMap([]byte{0, 129, 0, 0, 0, 0, 0, 0, 0x9c, 0x41, 0, 0, 0, 0, 0, 0}, 40000); err == nil {
		t.Fatal("a mapping for another port was taken")
	}
}

// A router that keeps only permanent mappings refuses the lease (725):
// the mapping is asked again without one, for this computer only, and
// removed by Unmap.
func TestUPnPMap(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, r.Header.Get("SOAPAction")+" "+string(b))
		if strings.Contains(string(b), "<NewLeaseDuration>3600<") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><s:Fault><faultcode>s:Client</faultcode><faultstring>UPnPError</faultstring><detail><UPnPError xmlns="urn:schemas-upnp-org:control-1-0"><errorCode>725</errorCode><errorDescription>OnlyPermanentLeasesSupported</errorDescription></UPnPError></detail></s:Fault></s:Body></s:Envelope>`))
			return
		}
		_, _ = w.Write([]byte(`<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body/></s:Envelope>`))
	}))
	defer srv.Close()
	w := wan{client: srv.Client(), control: srv.URL + "/ctl", service: "urn:schemas-upnp-org:service:WANIPConnection:1"}
	m, err := upnpMap(context.Background(), w, netip.MustParseAddr("192.168.1.20"), 40000, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if m.Via != "upnp" || m.Port != 40000 || m.Lifetime != 0 {
		t.Fatalf("%+v", m)
	}
	if len(bodies) != 2 || !strings.Contains(bodies[1], "<NewLeaseDuration>0<") ||
		!strings.Contains(bodies[1], "<NewInternalClient>192.168.1.20<") || !strings.Contains(bodies[1], "<NewProtocol>UDP<") ||
		!strings.Contains(bodies[1], "<NewPortMappingDescription>go-link Desk<") || !strings.HasPrefix(bodies[1], `"urn:schemas-upnp-org:service:WANIPConnection:1#AddPortMapping"`) {
		t.Fatalf("%q", bodies)
	}
	if err := Unmap(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bodies[2], "#DeletePortMapping") || !strings.Contains(bodies[2], "<NewExternalPort>40000<") {
		t.Fatalf("%q", bodies[2])
	}
	e := parseFault("AddPortMapping", []byte("not xml")).(*upnpError)
	if e.Code != 0 {
		t.Fatalf("%+v", e)
	}
}
