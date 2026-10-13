// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { endpoint, parseICEServers, type ICEServer } from "@go-link-desk/shared";

/**
 * The connection test (desk.go-link.org/network): the same discovery a
 * session makes (signalhub, STUN, TURN), without anyone on the other side.
 * Everything runs in the browser; nothing is stored.
 *
 * Pure functions (parsing, classifying) are exported for the tests; the
 * probes use WebSocket and RTCPeerConnection.
 */

/** A second STUN server, only to compare the port the router gives each one. */
export const COMPARE_STUN = "stun:stun.l.google.com:19302";

export type CandidateType = "host" | "srflx" | "prflx" | "relay";

export type Candidate = {
  type: CandidateType;
  address: string;
  port: number;
  protocol: "udp" | "tcp";
  /** Which server answered (srflx), when the browser says. */
  url?: string;
  /** How the browser reaches the relay (relay candidates): udp, tcp or tls. */
  relayProtocol?: string;
};

/** One a=candidate line, or null when it is not one. */
export function parseCandidate(line: string, url?: string, relayProtocol?: string): Candidate | null {
  const m = /^(?:a=)?candidate:\S+ \d+ (udp|tcp) \d+ (\S+) (\d+) typ (host|srflx|prflx|relay)\b/i.exec(line.trim());
  if (!m) return null;
  const c: Candidate = { protocol: m[1]!.toLowerCase() as "udp" | "tcp", address: m[2]!, port: Number(m[3]), type: m[4] as CandidateType };
  if (url) c.url = url;
  if (relayProtocol) c.relayProtocol = relayProtocol;
  return c;
}

export type IPKind = "private" | "cgnat" | "public" | "loopback" | "linklocal" | "mdns" | "invalid";

/** What kind of address this is. IPv6 is public unless link-local, unique local or loopback. */
export function ipKind(ip: string): IPKind {
  if (/\.local$/i.test(ip)) return "mdns";
  const v4 = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(ip);
  if (v4) {
    const [a, b] = [Number(v4[1]), Number(v4[2])];
    if ([a, b, Number(v4[3]), Number(v4[4])].some((n) => n > 255)) return "invalid";
    if (a === 10 || (a === 172 && b >= 16 && b <= 31) || (a === 192 && b === 168)) return "private";
    if (a === 100 && b >= 64 && b <= 127) return "cgnat";
    if (a === 127) return "loopback";
    if (a === 169 && b === 254) return "linklocal";
    if (a === 0 || a >= 224) return "invalid";
    return "public";
  }
  if (/^[0-9a-f:]+$/i.test(ip) && ip.includes(":")) {
    const l = ip.toLowerCase();
    if (l === "::1") return "loopback";
    if (l.startsWith("fe8") || l.startsWith("fe9") || l.startsWith("fea") || l.startsWith("feb")) return "linklocal";
    if (l.startsWith("fc") || l.startsWith("fd")) return "private";
    return "public";
  }
  return "invalid";
}

const isV4 = (ip: string) => /^\d{1,3}(\.\d{1,3}){3}$/.test(ip);

/**
 * How the router maps this computer's port toward different servers:
 * - independent: the same public port for every destination (most home
 *   routers), so the other side can come in directly;
 * - dependent: a new port per destination ("symmetric NAT"), which the other
 *   side cannot guess;
 * - unknown: no answer from STUN (UDP blocked) or nothing to compare.
 * The browser drops a second srflx candidate equal to the first, so one
 * address from two servers means "independent".
 */
export type Mapping = "independent" | "dependent" | "unknown";

export function mappingOf(candidates: readonly Candidate[]): Mapping {
  const srflx = candidates.filter((c) => c.type === "srflx" && c.protocol === "udp" && isV4(c.address));
  if (srflx.length === 0) return "unknown";
  const seen = new Set(srflx.map((c) => `${c.address}:${c.port}`));
  return seen.size > 1 ? "dependent" : "independent";
}

/** What the go-link Desk apps add in the page's #fragment (never sent to a server). */
export type Native = {
  src: "desktop" | "ios" | "android";
  /** This device's address on its own network. */
  lan?: string;
  /** Its router. */
  gw?: string;
  /** The router's own outside address, as the router says (UPnP or NAT-PMP). */
  wan?: string;
  /** The router answers UPnP (IGD) or NAT-PMP / PCP. */
  upnp?: boolean;
  pmp?: boolean;
  net?: "wifi" | "cellular" | "ethernet" | "other";
};

const ipOrUndefined = (v: unknown): string | undefined =>
  typeof v === "string" && v.length <= 45 && ipKind(v) !== "invalid" && ipKind(v) !== "mdns" ? v : undefined;

/**
 * The apps' data, from "#d=<base64url JSON>". Anything unexpected is
 * dropped: the page shows only checked addresses and known words.
 */
export function parseNative(hash: string): Native | null {
  const m = /(?:^#|&)d=([A-Za-z0-9_-]{1,800})/.exec(hash);
  if (!m) return null;
  let raw: unknown;
  try {
    const b64 = m[1]!.replace(/-/g, "+").replace(/_/g, "/");
    raw = JSON.parse(atob(b64 + "=".repeat((4 - (b64.length % 4)) % 4)));
  } catch {
    return null;
  }
  if (typeof raw !== "object" || raw === null) return null;
  const o = raw as Record<string, unknown>;
  if (o.src !== "desktop" && o.src !== "ios" && o.src !== "android") return null;
  const n: Native = { src: o.src };
  const lan = ipOrUndefined(o.lan);
  const gw = ipOrUndefined(o.gw);
  const wan = ipOrUndefined(o.wan);
  if (lan) n.lan = lan;
  if (gw) n.gw = gw;
  if (wan) n.wan = wan;
  if (typeof o.upnp === "boolean") n.upnp = o.upnp;
  if (typeof o.pmp === "boolean") n.pmp = o.pmp;
  if (o.net === "wifi" || o.net === "cellular" || o.net === "ethernet" || o.net === "other") n.net = o.net;
  return n;
}

/** Builds the fragment the apps add (tests, and the reference for the apps). */
export function nativeFragment(n: Native): string {
  const b64 = btoa(JSON.stringify(n)).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  return `#d=${b64}`;
}

export type Report = {
  /** Round trip to signalhub, median of a few. */
  signalMs?: number;
  signalOk: boolean;
  /** UDP leaves this network (a STUN server answered). */
  udp: boolean;
  publicIP?: string;
  /** A public IPv6 address answered too: no NAT on that path. */
  ipv6: boolean;
  mapping: Mapping;
  relayOk: boolean;
  relayProtocol?: "udp" | "tcp" | "tls";
  /** Round trip to the relay (half of browser → relay → browser). */
  relayMs?: number;
  native?: Native;
};

/** Routers between this device and the internet, when the app could tell. */
export function routersOf(r: Pick<Report, "native" | "publicIP">): number | undefined {
  const n = r.native;
  if (!n?.wan) return undefined;
  const k = ipKind(n.wan);
  if (k === "private" || k === "cgnat") return 2; // the router's outside is another network
  if (k === "public" && r.publicIP && isV4(n.wan) && n.wan !== r.publicIP) return 2;
  return 1;
}

/** The provider shares one address among many customers (100.64.0.0/10), or mobile data. */
export function cgnatOf(r: Pick<Report, "native">): boolean {
  return r.native?.wan ? ipKind(r.native.wan) === "cgnat" : r.native?.net === "cellular";
}

export type Verdict = "direct" | "maybe" | "relay";

/**
 * - relay: no UDP (only the relay over TCP works), or a symmetric NAT
 *   behind the provider's (CGNAT);
 * - maybe: a symmetric NAT, or routers in cascade: direct only if the
 *   other side's network is simple;
 * - direct: one router that keeps the port (or only IPv6 answered, which
 *   has no NAT).
 */
export function verdictOf(r: Report): Verdict {
  if (!r.udp) return "relay";
  const cgnat = cgnatOf(r);
  if (r.mapping === "dependent") return cgnat ? "relay" : "maybe";
  if (cgnat || (routersOf(r) ?? 1) > 1) return "maybe";
  if (r.mapping === "independent") return "direct";
  return r.ipv6 ? "direct" : "maybe";
}

/** Which guide fits this network. */
export type Advice = "one" | "double" | "cgnat" | "udp";

export function adviceOf(r: Report): Advice {
  if (!r.udp) return "udp";
  if (cgnatOf(r)) return "cgnat";
  if ((routersOf(r) ?? 1) > 1 || r.mapping === "dependent") return "double";
  return "one";
}

const median = (xs: number[]) => {
  const s = [...xs].sort((a, b) => a - b);
  return s[Math.floor(s.length / 2)]!;
};

/**
 * Connects to signalhub, takes the STUN/TURN servers from hello and times
 * a few round trips (rooms_list: an answer of its own, no room is touched).
 */
export function signalProbe(url: string, timeoutMs = 6000, WS: typeof WebSocket = globalThis.WebSocket): Promise<{ ms: number; iceServers: ICEServer[] }> {
  return new Promise((resolve, reject) => {
    const ws = new WS(endpoint(url));
    const times: number[] = [];
    let iceServers: ICEServer[] = [];
    let sent = 0;
    let done = false;
    const finish = (err?: Error) => {
      if (done) return;
      done = true;
      clearTimeout(timer);
      ws.close();
      if (err) reject(err);
      else resolve({ ms: Math.round(median(times)), iceServers });
    };
    const timer = setTimeout(() => (times.length ? finish() : finish(new Error("signal timeout"))), timeoutMs);
    const ping = () => {
      sent = performance.now();
      ws.send(JSON.stringify({ v: 1, type: "rooms_list", app: "go-link-desk" }));
    };
    ws.onmessage = (ev) => {
      let m: { type?: string; ice_servers?: unknown };
      try {
        m = JSON.parse(String(ev.data));
      } catch {
        return;
      }
      if (m.type === "hello") {
        iceServers = parseICEServers(m.ice_servers);
        ping();
      } else if (m.type === "rooms" || m.type === "error") {
        times.push(performance.now() - sent);
        if (times.length >= 3) finish();
        else ping();
      }
    };
    ws.onerror = () => finish(new Error("signal error"));
    ws.onclose = () => finish(times.length ? undefined : new Error("signal closed"));
  });
}

/** Gathers this browser's candidates against some STUN/TURN servers. */
export async function gather(config: RTCConfiguration, timeoutMs = 5000, PC: typeof RTCPeerConnection = globalThis.RTCPeerConnection): Promise<Candidate[]> {
  const pc = new PC(config);
  const out: Candidate[] = [];
  try {
    pc.createDataChannel("probe");
    const done = new Promise<void>((resolve) => {
      const timer = setTimeout(resolve, timeoutMs);
      pc.onicecandidate = (e) => {
        if (!e.candidate) {
          clearTimeout(timer);
          resolve();
          return;
        }
        const ext = e.candidate as RTCIceCandidate & { url?: string; relayProtocol?: string };
        const c = parseCandidate(e.candidate.candidate, ext.url ?? (e as RTCPeerConnectionIceEvent & { url?: string }).url ?? undefined, ext.relayProtocol);
        if (c) out.push(c);
      };
    });
    await pc.setLocalDescription(await pc.createOffer());
    await done;
  } finally {
    pc.close();
  }
  return out;
}

/**
 * Two connections in this page, both forced through the relay, connected
 * to each other: the data channel's echo time is browser → relay → browser
 * twice, so half of it is this network's round trip to the relay. A few
 * bytes only: no bandwidth test.
 */
export async function relayProbe(turn: ICEServer[], timeoutMs = 9000, PC: typeof RTCPeerConnection = globalThis.RTCPeerConnection): Promise<{ ms: number; protocol?: "udp" | "tcp" | "tls" }> {
  const config: RTCConfiguration = { iceServers: turn, iceTransportPolicy: "relay" };
  const a = new PC(config);
  const b = new PC(config);
  const close = () => {
    a.close();
    b.close();
  };
  a.onicecandidate = (e) => e.candidate && void b.addIceCandidate(e.candidate).catch(() => {});
  b.onicecandidate = (e) => e.candidate && void a.addIceCandidate(e.candidate).catch(() => {});
  try {
    return await new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error("relay timeout")), timeoutMs);
      const ch = a.createDataChannel("echo");
      b.ondatachannel = (e) => {
        e.channel.onmessage = (m) => e.channel.send(m.data);
      };
      const rtts: number[] = [];
      let sent = 0;
      const ping = () => {
        sent = performance.now();
        ch.send("ping");
      };
      ch.onopen = ping;
      ch.onmessage = async () => {
        rtts.push(performance.now() - sent);
        if (rtts.length < 5) return ping();
        clearTimeout(timer);
        resolve({ ms: Math.max(1, Math.round(median(rtts) / 2)), protocol: await relayProtocolOf(a) });
      };
      void (async () => {
        await a.setLocalDescription(await a.createOffer());
        await b.setRemoteDescription(a.localDescription!);
        await b.setLocalDescription(await b.createAnswer());
        await a.setRemoteDescription(b.localDescription!);
      })().catch(reject);
    });
  } finally {
    close();
  }
}

async function relayProtocolOf(pc: RTCPeerConnection): Promise<"udp" | "tcp" | "tls" | undefined> {
  const stats = await pc.getStats();
  let pairLocal: string | undefined;
  stats.forEach((s) => {
    if (s.type === "candidate-pair" && (s as RTCIceCandidatePairStats).nominated && (s as RTCIceCandidatePairStats).state === "succeeded")
      pairLocal = (s as RTCIceCandidatePairStats).localCandidateId;
  });
  const local = pairLocal ? (stats.get(pairLocal) as { relayProtocol?: string } | undefined) : undefined;
  const p = local?.relayProtocol;
  return p === "udp" || p === "tcp" || p === "tls" ? p : undefined;
}

/** The test's steps, in the order the page shows them. */
export type StepId = "signal" | "lan" | "routers" | "udp" | "public" | "nat" | "relay";
export type StepState = "run" | "ok" | "warn" | "bad";
export type StepEvent = { id: StepId; state: StepState; report: Report };

/**
 * Runs the whole test, telling each step as it starts and ends. The STUN
 * servers come from hello (ours) plus one to compare ports; the relay is
 * the TURN of hello.
 */
export async function runTest(
  signalURL: string,
  native: Native | null,
  onStep: (e: StepEvent) => void,
  deps: { WS?: typeof WebSocket; PC?: typeof RTCPeerConnection } = {},
): Promise<Report> {
  const r: Report = { signalOk: false, udp: false, ipv6: false, mapping: "unknown", relayOk: false };
  if (native) r.native = native;
  const step = (id: StepId, state: StepState) => onStep({ id, state, report: { ...r } });

  step("signal", "run");
  let ice: ICEServer[] = [];
  try {
    const s = await signalProbe(signalURL, 6000, deps.WS);
    r.signalOk = true;
    r.signalMs = s.ms;
    ice = s.iceServers;
    step("signal", "ok");
  } catch {
    step("signal", "bad");
  }

  step("lan", "run");
  step("lan", native?.lan ? "ok" : "warn");
  step("routers", "run");
  const routers = routersOf(r);
  step("routers", routers === undefined ? "warn" : routers > 1 ? "warn" : "ok");

  step("udp", "run");
  const stun = ice.flatMap((s) => s.urls.filter((u) => /^stun:/i.test(u)));
  const cands = await gather({ iceServers: [{ urls: [...(stun.length ? stun.slice(0, 1) : ["stun:signal.go-link.org:3478"])] }, { urls: [COMPARE_STUN] }] }, 5000, deps.PC).catch(
    () => [] as Candidate[],
  );
  const srflx = cands.filter((c) => c.type === "srflx");
  r.udp = srflx.length > 0;
  r.ipv6 = srflx.some((c) => !isV4(c.address) && ipKind(c.address) === "public");
  r.publicIP = srflx.find((c) => isV4(c.address))?.address ?? srflx[0]?.address;
  r.mapping = mappingOf(cands);
  step("udp", r.udp ? "ok" : "bad");
  step("public", "run");
  step("public", r.publicIP ? "ok" : "bad");
  step("routers", routersOf(r) === undefined ? "warn" : (routersOf(r) ?? 1) > 1 ? "warn" : "ok");
  step("nat", "run");
  step("nat", r.mapping === "independent" ? "ok" : r.mapping === "dependent" ? "warn" : "bad");

  step("relay", "run");
  const turn = ice.filter((s) => s.urls.some((u) => /^turns?:/i.test(u)));
  if (turn.length) {
    try {
      const rel = await relayProbe(turn, 9000, deps.PC);
      r.relayOk = true;
      r.relayMs = rel.ms;
      if (rel.protocol) r.relayProtocol = rel.protocol;
    } catch {
      // the relay did not answer
    }
  }
  step("relay", r.relayOk ? "ok" : "bad");
  return r;
}
