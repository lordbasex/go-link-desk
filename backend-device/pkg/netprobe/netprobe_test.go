// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package netprobe

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseRoutes(t *testing.T) {
	mac := []byte("   route to: default\ndestination: default\n       mask: default\n    gateway: 192.168.0.1\n  interface: en0\n")
	if got := parseRouteGet(mac); got != netip.MustParseAddr("192.168.0.1") {
		t.Fatalf("macOS: %v", got)
	}
	linux := []byte("Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n" +
		"eth0\t0000A8C0\t00000000\t0001\t0\t0\t100\t00FFFFFF\t0\t0\t0\n" +
		"eth0\t00000000\t0100A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n")
	if got := parseProcRoute(linux); got != netip.MustParseAddr("192.168.0.1") {
		t.Fatalf("Linux: %v", got)
	}
	if parseProcRoute([]byte("header\n")).IsValid() || parseRouteGet([]byte("nothing")).IsValid() {
		t.Fatal("a gateway out of nothing")
	}
}

// A fake router answers NAT-PMP's external address request.
func TestNATPMPExternal(t *testing.T) {
	c, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	old := natpmpPort
	natpmpPort = c.LocalAddr().(*net.UDPAddr).Port
	defer func() { natpmpPort = old }()
	go func() {
		buf := make([]byte, 16)
		n, from, err := c.ReadFrom(buf)
		if err != nil || n != 2 || buf[0] != 0 || buf[1] != 0 {
			return
		}
		_, _ = c.WriteTo([]byte{0, 128, 0, 0, 0, 0, 0, 9, 192, 168, 1, 2}, from)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	a, err := NATPMPExternal(ctx, netip.MustParseAddr("127.0.0.1"))
	if err != nil || a != netip.MustParseAddr("192.168.1.2") {
		t.Fatalf("%v %v", a, err)
	}
	if _, err := parseNATPMP([]byte{0, 128, 0, 3, 0, 0, 0, 0, 0, 0, 0, 0}); err == nil {
		t.Fatal("a refusal was taken")
	}
}

// A fake gateway: its description and GetExternalIPAddress.
func TestUPnPAsk(t *testing.T) {
	var soapAction string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rootDesc.xml":
			_, _ = w.Write([]byte(`<?xml version="1.0"?><root><device><deviceList><device><deviceList><device><serviceList>
<service><serviceType>urn:schemas-upnp-org:service:WANIPConnection:1</serviceType><controlURL>/ctl/IPConn</controlURL></service>
</serviceList></device></deviceList></device></deviceList></device></root>`))
		case "/ctl/IPConn":
			soapAction = r.Header.Get("SOAPAction")
			_, _ = w.Write([]byte(`<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:GetExternalIPAddressResponse xmlns:u="urn:schemas-upnp-org:service:WANIPConnection:1"><NewExternalIPAddress>100.72.14.9</NewExternalIPAddress></u:GetExternalIPAddressResponse></s:Body></s:Envelope>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	a, err := askExternal(context.Background(), srv.Client(), srv.URL+"/rootDesc.xml")
	if err != nil || a != netip.MustParseAddr("100.72.14.9") {
		t.Fatalf("%v %v", a, err)
	}
	if soapAction != `"urn:schemas-upnp-org:service:WANIPConnection:1#GetExternalIPAddress"` {
		t.Fatalf("SOAPAction %q", soapAction)
	}
}

func TestSSDPAnswers(t *testing.T) {
	ans := []byte("HTTP/1.1 200 OK\r\nCACHE-CONTROL: max-age=120\r\nLocation: http://192.168.0.1:5000/rootDesc.xml\r\nST: urn:schemas-upnp-org:device:InternetGatewayDevice:1\r\n\r\n")
	loc := ssdpLocation(ans)
	if loc != "http://192.168.0.1:5000/rootDesc.xml" {
		t.Fatalf("%q", loc)
	}
	router := &net.UDPAddr{IP: net.IPv4(192, 168, 0, 1), Port: 1900}
	if !localDescription(loc, router) {
		t.Fatal("the router's own description refused")
	}
	for _, bad := range []string{"https://192.168.0.1/d.xml", "http://8.8.8.8/d.xml", "http://192.168.0.7/d.xml", "http://user@192.168.0.1/d.xml", "file:///etc/passwd"} {
		if localDescription(bad, router) {
			t.Fatalf("%s taken", bad)
		}
	}
	if ssdpLocation([]byte("NOTIFY * HTTP/1.1\r\nLocation: http://192.168.0.1/x\r\n\r\n")) != "" {
		t.Fatal("a NOTIFY taken as an answer")
	}
}

// A control URL on another host is never called.
func TestControlURLStaysOnTheDevice(t *testing.T) {
	body := []byte(`<root><URLBase>http://10.0.0.9:80/</URLBase><device><serviceList><service><serviceType>urn:schemas-upnp-org:service:WANIPConnection:1</serviceType><controlURL>http://example.com/ctl</controlURL></service></serviceList></device></root>`)
	if _, _, err := controlURL("http://192.168.0.1:5000/rootDesc.xml", body); err == nil {
		t.Fatal("a control URL elsewhere was taken")
	}
}

func TestFragment(t *testing.T) {
	f := Result{Src: "desktop", LAN: "192.168.0.12", GW: "192.168.0.1", WAN: "192.168.1.2", UPnP: true, Net: "ethernet"}.Fragment()
	if !strings.HasPrefix(f, "#d=") || strings.ContainsAny(f[3:], "+/=") {
		t.Fatalf("%q", f)
	}
	b, err := base64.RawURLEncoding.DecodeString(f[3:])
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(b, &back); err != nil || back["src"] != "desktop" || back["wan"] != "192.168.1.2" || back["upnp"] != true {
		t.Fatalf("%v %v", back, err)
	}
}

func TestKindOfName(t *testing.T) {
	for name, want := range map[string]string{"wlan0": "wifi", "wlp3s0": "wifi", "Wi-Fi": "wifi", "eth0": "ethernet", "enp0s3": "ethernet", "Ethernet 2": "ethernet", "en0": "other", "utun3": "other"} {
		if got := kindOfName(name); got != want {
			t.Errorf("%s: %s", name, got)
		}
	}
}

// NETPROBE_LIVE=1 asks this computer's real router.
func TestLive(t *testing.T) {
	if os.Getenv("NETPROBE_LIVE") == "" {
		t.Skip("NETPROBE_LIVE=1 to ask the real router")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r := Probe(ctx, "signal.go-link.org")
	t.Logf("%+v %s", r, r.Fragment())
}

func TestParseHops(t *testing.T) {
	mac := []byte("traceroute to 54.208.64.177 (54.208.64.177), 3 hops max, 40 byte packets\n 1  172.17.0.1  0.469 ms\n 2  192.168.1.1  0.605 ms\n 3  200.51.241.1  3.067 ms\n")
	tracepath := []byte(" 1?: [LOCALHOST]                      pmtu 1500\n 1:  172.17.0.1                                            0.402ms\n 1:  172.17.0.1                                            0.311ms\n 2:  192.168.1.1                                           0.712ms\n 3:  no reply\n")
	tracert := []byte("\r\nTraza a 54.208.64.177 sobre caminos de 3 saltos como máximo\r\n\r\n  1    <1 ms    <1 ms    <1 ms  192.168.0.1\r\n  2     1 ms     1 ms     1 ms  100.72.0.1\r\n  3     *        *        *     Tiempo de espera agotado para esta solicitud.\r\n")
	for name, c := range map[string]struct {
		out  []byte
		want string
	}{
		"traceroute": {mac, "[172.17.0.1 192.168.1.1 200.51.241.1]"},
		"tracepath":  {tracepath, "[172.17.0.1 192.168.1.1]"},
		"tracert":    {tracert, "[192.168.0.1 100.72.0.1]"},
	} {
		if got := fmt.Sprint(parseHops(c.out, 3)); got != c.want {
			t.Errorf("%s: %s, want %s", name, got, c.want)
		}
	}
	if got := parseHops([]byte(" 1  *\n 2  10.0.0.1  1 ms\n"), 3); len(got) != 0 {
		t.Errorf("a missing first hop: %v", got)
	}
}
