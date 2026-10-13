// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package netprobe

import (
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// ssdpAddr is where UPnP devices listen for searches.
var ssdpAddr = "239.255.255.250:1900"

const maxDescription = 256 << 10 // a device description is a few kB

// The services that know the router's outside address.
var wanServices = []string{
	"urn:schemas-upnp-org:service:WANIPConnection:2",
	"urn:schemas-upnp-org:service:WANIPConnection:1",
	"urn:schemas-upnp-org:service:WANPPPConnection:1",
}

// UPnPExternal finds the router's UPnP Internet Gateway Device and asks it
// for its outside address (GetExternalIPAddress, which only reads). Only
// a device on the local network is asked: an answer whose description is
// elsewhere, or not plain HTTP, is ignored. gw, when known, is preferred.
func UPnPExternal(ctx context.Context, gw netip.Addr) (netip.Addr, error) {
	locs, err := ssdpSearch(ctx)
	if err != nil {
		return netip.Addr{}, err
	}
	// The gateway's own description first.
	for i, l := range locs {
		if u, err := url.Parse(l); err == nil && gw.IsValid() && u.Hostname() == gw.String() {
			locs[0], locs[i] = locs[i], locs[0]
			break
		}
	}
	client := &http.Client{
		Timeout:       3 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	var last error = errors.New("netprobe: no UPnP gateway")
	for _, l := range locs {
		a, err := askExternal(ctx, client, l)
		if err == nil {
			return a, nil
		}
		last = err
	}
	return netip.Addr{}, last
}

// ssdpSearch sends M-SEARCH for gateways and collects the LOCATION of
// every answer until the deadline (2 s at most).
func ssdpSearch(ctx context.Context) ([]string, error) {
	c, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, err
	}
	defer c.Close()
	dst, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		return nil, err
	}
	for _, st := range []string{"urn:schemas-upnp-org:device:InternetGatewayDevice:1", "urn:schemas-upnp-org:device:InternetGatewayDevice:2"} {
		msg := "M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 1\r\nST: " + st + "\r\n\r\n"
		if _, err := c.WriteTo([]byte(msg), dst); err != nil {
			return nil, err
		}
	}
	end := deadline(ctx, 2*time.Second)
	if max := time.Now().Add(2 * time.Second); end.After(max) {
		end = max
	}
	_ = c.SetReadDeadline(end)
	seen := map[string]bool{}
	var locs []string
	buf := make([]byte, 2048)
	for {
		n, from, err := c.ReadFrom(buf)
		if err != nil {
			break
		}
		loc := ssdpLocation(buf[:n])
		if loc == "" || seen[loc] || !localDescription(loc, from) {
			continue
		}
		seen[loc] = true
		locs = append(locs, loc)
		if len(locs) >= 4 {
			break
		}
	}
	if len(locs) == 0 {
		return nil, errors.New("netprobe: no UPnP gateway answered")
	}
	return locs, nil
}

// ssdpLocation is the LOCATION header of an SSDP answer.
func ssdpLocation(b []byte) string {
	r := bufio.NewReader(bytes.NewReader(b))
	line, err := r.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "HTTP/1.1 200") {
		return ""
	}
	for {
		line, err := r.ReadString('\n')
		k, v, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok && strings.EqualFold(strings.TrimSpace(k), "location") {
			return strings.TrimSpace(v)
		}
		if err != nil {
			return ""
		}
	}
}

// localDescription: plain HTTP, on the device that answered, on a private
// address. A router never puts its description anywhere else.
func localDescription(loc string, from net.Addr) bool {
	u, err := url.Parse(loc)
	if err != nil || u.Scheme != "http" || u.User != nil {
		return false
	}
	host, err := netip.ParseAddr(u.Hostname())
	if err != nil || !host.IsPrivate() {
		return false
	}
	if a, ok := from.(*net.UDPAddr); ok {
		if f, ok := netip.AddrFromSlice(a.IP.To4()); ok && f != host {
			return false
		}
	}
	return true
}

// description is the part of a device description that matters: every
// service, nested devices included.
type description struct {
	URLBase string `xml:"URLBase"`
	Device  device `xml:"device"`
}

type device struct {
	Services []service `xml:"serviceList>service"`
	Devices  []device  `xml:"deviceList>device"`
}

type service struct {
	Type    string `xml:"serviceType"`
	Control string `xml:"controlURL"`
}

// controlURL finds a WAN connection service in a description and returns
// its control URL, resolved against the description's address.
func controlURL(loc string, body []byte) (string, string, error) {
	var d description
	if err := xml.Unmarshal(body, &d); err != nil {
		return "", "", err
	}
	var all []service
	var walk func(device)
	walk = func(dv device) {
		all = append(all, dv.Services...)
		for _, sub := range dv.Devices {
			walk(sub)
		}
	}
	walk(d.Device)
	base, err := url.Parse(loc)
	if err != nil {
		return "", "", err
	}
	if d.URLBase != "" {
		if b, err := url.Parse(d.URLBase); err == nil && b.Host == base.Host {
			base = b
		}
	}
	for _, want := range wanServices {
		for _, s := range all {
			if strings.TrimSpace(s.Type) != want {
				continue
			}
			ctl, err := base.Parse(strings.TrimSpace(s.Control))
			if err != nil || ctl.Host != base.Host || ctl.Scheme != "http" {
				continue
			}
			return ctl.String(), want, nil
		}
	}
	return "", "", errors.New("netprobe: no WAN connection service")
}

// askExternal reads the description, then calls GetExternalIPAddress.
func askExternal(ctx context.Context, client *http.Client, loc string) (netip.Addr, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, loc, nil)
	if err != nil {
		return netip.Addr{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return netip.Addr{}, err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDescription))
	resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusOK {
		return netip.Addr{}, errors.New("netprobe: no device description")
	}
	ctl, st, err := controlURL(loc, body)
	if err != nil {
		return netip.Addr{}, err
	}
	soap := `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:GetExternalIPAddress xmlns:u="` + st + `"/></s:Body></s:Envelope>`
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, ctl, strings.NewReader(soap))
	if err != nil {
		return netip.Addr{}, err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPAction", `"`+st+`#GetExternalIPAddress"`)
	resp, err = client.Do(req)
	if err != nil {
		return netip.Addr{}, err
	}
	defer resp.Body.Close()
	body, err = io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil || resp.StatusCode != http.StatusOK {
		return netip.Addr{}, errors.New("netprobe: GetExternalIPAddress failed")
	}
	return parseExternal(body)
}

// parseExternal reads NewExternalIPAddress from the SOAP answer.
func parseExternal(body []byte) (netip.Addr, error) {
	dec := xml.NewDecoder(bytes.NewReader(body))
	for {
		tok, err := dec.Token()
		if err != nil {
			return netip.Addr{}, errors.New("netprobe: no external address in the answer")
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "NewExternalIPAddress" {
			var v string
			if err := dec.DecodeElement(&v, &se); err != nil {
				return netip.Addr{}, err
			}
			a, err := netip.ParseAddr(strings.TrimSpace(v))
			if err != nil || !a.Is4() || a.IsUnspecified() {
				return netip.Addr{}, errors.New("netprobe: the router has no outside address")
			}
			return a, nil
		}
	}
}
