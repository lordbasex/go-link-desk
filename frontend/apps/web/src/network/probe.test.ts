// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { describe, expect, it } from "vitest";
import {
  adviceOf,
  cgnatOf,
  ipKind,
  mappingOf,
  nativeFragment,
  parseCandidate,
  parseNative,
  routersOf,
  signalProbe,
  verdictOf,
  type Candidate,
  type Report,
} from "./probe";

const srflx = (address: string, port: number): Candidate => ({ type: "srflx", protocol: "udp", address, port });
const base: Report = { signalOk: true, udp: true, ipv6: false, mapping: "independent", relayOk: true, publicIP: "181.23.153.151" };

describe("parseCandidate", () => {
  it("reads the type, address, port and protocol", () => {
    expect(parseCandidate("candidate:842163049 1 udp 1677729535 181.23.153.151 57525 typ srflx raddr 0.0.0.0 rport 0 generation 0", "stun:x")).toEqual({
      type: "srflx",
      protocol: "udp",
      address: "181.23.153.151",
      port: 57525,
      url: "stun:x",
    });
    expect(parseCandidate("a=candidate:1 1 udp 2122260223 3c1b6f2e-1a2b.local 61234 typ host")?.address).toBe("3c1b6f2e-1a2b.local");
    expect(parseCandidate("candidate:2 1 tcp 1 54.208.64.177 3478 typ relay", undefined, "tcp")?.relayProtocol).toBe("tcp");
  });

  it("ignores anything else", () => {
    expect(parseCandidate("")).toBeNull();
    expect(parseCandidate("candidate:1 1 sctp 1 1.2.3.4 5 typ host")).toBeNull();
  });
});

describe("ipKind", () => {
  it("tells private, CGNAT, public and the rest apart", () => {
    expect(ipKind("192.168.1.2")).toBe("private");
    expect(ipKind("10.0.0.1")).toBe("private");
    expect(ipKind("172.20.1.1")).toBe("private");
    expect(ipKind("172.32.1.1")).toBe("public");
    expect(ipKind("100.72.14.9")).toBe("cgnat");
    expect(ipKind("100.128.0.1")).toBe("public");
    expect(ipKind("181.23.153.151")).toBe("public");
    expect(ipKind("127.0.0.1")).toBe("loopback");
    expect(ipKind("169.254.3.4")).toBe("linklocal");
    expect(ipKind("2800:810:4a2::1")).toBe("public");
    expect(ipKind("fe80::1")).toBe("linklocal");
    expect(ipKind("fd00::1")).toBe("private");
    expect(ipKind("abc.local")).toBe("mdns");
    expect(ipKind("300.1.1.1")).toBe("invalid");
    expect(ipKind("<script>")).toBe("invalid");
  });
});

describe("mappingOf", () => {
  it("is independent when both servers see one port", () => {
    expect(mappingOf([srflx("181.23.153.151", 57525)])).toBe("independent");
    expect(mappingOf([srflx("181.23.153.151", 57525), srflx("181.23.153.151", 57525)])).toBe("independent");
  });

  it("is dependent when each server sees another port", () => {
    expect(mappingOf([srflx("181.23.153.151", 57525), srflx("181.23.153.151", 61208)])).toBe("dependent");
  });

  it("is unknown without an IPv4 answer", () => {
    expect(mappingOf([])).toBe("unknown");
    expect(mappingOf([srflx("2800:810:4a2::1", 5000)])).toBe("unknown");
  });
});

describe("parseNative", () => {
  it("reads what an app sends after #d=", () => {
    const n = { src: "desktop" as const, lan: "192.168.0.12", gw: "192.168.0.1", wan: "192.168.1.2", upnp: true, net: "ethernet" as const };
    expect(parseNative(nativeFragment(n))).toEqual(n);
  });

  it("keeps only checked addresses and known words", () => {
    const evil = "#d=" + btoa(JSON.stringify({ src: "ios", lan: "<img src=x>", wan: "100.72.14.9", net: "satellite", extra: 1 })).replace(/=+$/, "");
    expect(parseNative(evil)).toEqual({ src: "ios", wan: "100.72.14.9" });
    expect(parseNative("#d=" + btoa(JSON.stringify({ src: "other" })))).toBeNull();
    expect(parseNative("#d=%%%")).toBeNull();
    expect(parseNative("")).toBeNull();
  });
});

describe("routers, CGNAT and the verdict", () => {
  it("counts a second router when the router's outside is private or not the public address", () => {
    expect(routersOf(base)).toBeUndefined();
    expect(routersOf({ ...base, native: { src: "desktop", wan: "192.168.1.2" } })).toBe(2);
    expect(routersOf({ ...base, native: { src: "desktop", wan: "190.210.4.77" } })).toBe(2);
    expect(routersOf({ ...base, native: { src: "desktop", wan: "181.23.153.151" } })).toBe(1);
  });

  it("calls the provider's 100.64/10 and mobile data CGNAT", () => {
    expect(cgnatOf({ native: { src: "desktop", wan: "100.72.14.9" } })).toBe(true);
    expect(cgnatOf({ native: { src: "android", net: "cellular" } })).toBe(true);
    expect(cgnatOf({ native: { src: "desktop", wan: "192.168.1.2" } })).toBe(false);
    expect(cgnatOf({})).toBe(false);
  });

  it("says direct, depends or relay", () => {
    expect(verdictOf(base)).toBe("direct");
    expect(verdictOf({ ...base, mapping: "dependent" })).toBe("maybe");
    expect(verdictOf({ ...base, native: { src: "desktop", wan: "192.168.1.2" } })).toBe("maybe");
    expect(verdictOf({ ...base, mapping: "dependent", native: { src: "desktop", wan: "100.72.14.9" } })).toBe("relay");
    expect(verdictOf({ ...base, udp: false, mapping: "unknown" })).toBe("relay");
    expect(verdictOf({ ...base, mapping: "unknown", ipv6: true })).toBe("direct");
  });

  it("picks the guide for the case", () => {
    expect(adviceOf(base)).toBe("one");
    expect(adviceOf({ ...base, mapping: "dependent" })).toBe("double");
    expect(adviceOf({ ...base, native: { src: "desktop", wan: "192.168.1.2" } })).toBe("double");
    expect(adviceOf({ ...base, native: { src: "desktop", wan: "100.72.14.9" } })).toBe("cgnat");
    expect(adviceOf({ ...base, udp: false })).toBe("udp");
  });
});

/** A WebSocket that answers hello, then rooms_list, like signalhub. */
class FakeWS {
  onmessage: ((e: { data: string }) => void) | null = null;
  onerror: (() => void) | null = null;
  onclose: (() => void) | null = null;
  sent: string[] = [];
  constructor(public url: string) {
    setTimeout(() => this.onmessage?.({ data: JSON.stringify({ type: "hello", peer_id: "p", ice_servers: [{ urls: ["stun:s:3478"] }, { urls: ["turn:s:3478"], username: "u", credential: "c" }] }) }));
  }
  send(m: string) {
    this.sent.push(m);
    setTimeout(() => this.onmessage?.({ data: JSON.stringify({ type: "rooms" }) }), 1);
  }
  close() {}
}

describe("signalProbe", () => {
  it("takes the servers from hello and times three round trips", async () => {
    let ws: FakeWS | undefined;
    const WS = class extends FakeWS {
      constructor(url: string) {
        super(url);
        ws = this;
      }
    } as unknown as typeof WebSocket;
    const r = await signalProbe("wss://signal.example/ws", 2000, WS);
    expect(ws!.url).toBe("wss://signal.example/ws?v=1");
    expect(ws!.sent).toHaveLength(3);
    expect(JSON.parse(ws!.sent[0]!)).toMatchObject({ type: "rooms_list", app: "go-link-desk" });
    expect(r.iceServers.map((s) => s.urls[0])).toEqual(["stun:s:3478", "turn:s:3478"]);
    expect(r.ms).toBeGreaterThanOrEqual(0);
  });
});
