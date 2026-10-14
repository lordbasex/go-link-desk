// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { Fragment, lazy, Suspense, useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { DEFAULT_SIGNAL_URL } from "../config";
import { t, useLang } from "../i18n";
import { adviceOf, cgnatOf, ipKind, outerRouterOf, parseNative, routersOf, runTest, verdictOf, type Advice, type Native, type Report, type StepEvent, type StepId, type StepState, type Verdict } from "../network/probe";
import { SiteFooter, SiteNav } from "./LandingPage";
import "../styles/landing.css";
import "../styles/network.css";

const RelayMap = lazy(() => import("../network/RelayMap"));

const STEPS: readonly StepId[] = ["signal", "lan", "routers", "udp", "public", "nat", "relay"];

/** **bold** in the texts; nothing else is markup. */
export function rich(text: string): ReactNode[] {
  return text.split(/\*\*(.+?)\*\*/g).map((part, i) => (i % 2 ? <b key={i}>{part}</b> : <Fragment key={i}>{part}</Fragment>));
}

type Run = { state: "idle" | "running" | "done" | "error"; steps: Partial<Record<StepId, StepState>>; report: Report | null; active: StepId | null };

/**
 * desk.go-link.org/network: does this connection go direct or through the
 * relay? It runs the discovery a session makes (probe.ts) and draws it:
 * the path from this device to the servers, a gauge, the steps live, and
 * what to do about it. The apps open it with what only they can see
 * (local address, the router's outside address) in the #fragment, and the
 * test starts by itself.
 */
export function NetworkPage({ test = runTest }: { test?: typeof runTest }) {
  useLang();
  const n = t.network;
  const [native] = useState<Native | null>(() => (typeof window === "undefined" ? null : parseNative(window.location.hash)));
  const [run, setRun] = useState<Run>({ state: "idle", steps: {}, report: null, active: null });
  const [tab, setTab] = useState<Advice>("one");
  const busy = useRef(false);

  const start = useCallback(async () => {
    if (busy.current) return;
    busy.current = true;
    setRun({ state: "running", steps: {}, report: native ? { signalOk: false, udp: false, ipv6: false, mapping: "unknown", relayOk: false, native } : null, active: null });
    const onStep = (e: StepEvent) =>
      setRun((r) => ({ ...r, steps: { ...r.steps, [e.id]: e.state }, report: e.report, active: e.state === "run" ? e.id : r.active === e.id ? null : r.active }));
    try {
      const report = await test(DEFAULT_SIGNAL_URL, native, onStep);
      setRun((r) => ({ ...r, state: report.signalOk ? "done" : "error", report, active: null }));
      setTab(adviceOf(report));
    } catch {
      setRun((r) => ({ ...r, state: "error", active: null }));
    } finally {
      busy.current = false;
    }
  }, [native, test]);

  useEffect(() => {
    if (native) void start();
  }, [native, start]);

  const report = run.report;
  const verdict: Verdict | null = run.state === "done" && report ? verdictOf(report) : null;

  return (
    <div className="lp nt">
      <div className="lp-wrap">
        <SiteNav />
        <header className="nt-hero">
          <p className="lp-eyebrow">{n.eyebrow}</p>
          <h1>
            {n.titleA} <em>{n.titleEm}</em> {n.titleB}
          </h1>
          <p className="nt-lead">{n.lead}</p>
        </header>

        <div className="nt-test">
          <section className="nt-panel nt-path-panel" aria-label={n.pathTitle}>
            <h2 className="nt-panel-title">{n.pathTitle}</h2>
            <PathDiagram run={run} native={native} />
            <p className="nt-callout">{rich(native ? n.hintApp : n.hintWeb)}</p>
          </section>
          <section className="nt-panel nt-gauge-panel" aria-live="polite">
            <Gauge running={run.state === "running"} verdict={verdict} />
            <p className={`nt-verdict ${verdict ?? ""}`}>{verdict ? n.gauge[verdict] : " "}</p>
            <button type="button" className={`nt-go ${run.state === "idle" ? "pulse" : ""}`} onClick={() => void start()} disabled={run.state === "running"}>
              {run.state === "running" ? "…" : run.state === "idle" ? n.start : n.again}
            </button>
          </section>
          <section className="nt-panel nt-steps-panel">
            <Steps run={run} />
          </section>
        </div>

        {run.state === "error" && <p className="nt-error">{n.error}</p>}
        {verdict && report && <Result verdict={verdict} />}

        <section className="nt-block">
          <div className="lp-head">
            <p className="lp-eyebrow">{n.fix.eyebrow}</p>
            <h2>{n.fix.title}</h2>
            <p>{n.fix.intro}</p>
          </div>
          <div className="nt-tabs" role="tablist">
            {(["double", "one", "cgnat", "udp"] as const).map((k) => (
              <button key={k} type="button" role="tab" aria-selected={tab === k} onClick={() => setTab(k)}>
                {n.fix.tabs[k]}
              </button>
            ))}
          </div>
          <div className="nt-panel" role="tabpanel">
            <Fix advice={tab} report={report} />
          </div>
        </section>

        <section className="nt-block">
          <div className="lp-head">
            <p className="lp-eyebrow">{n.why.eyebrow}</p>
            <h2>{n.why.title}</h2>
          </div>
          <div className="nt-why">
            {n.why.cards.map((c) => (
              <div key={c.title} className="nt-panel">
                <h3>{c.title}</h3>
                <p>{c.text}</p>
              </div>
            ))}
          </div>
        </section>

        <section className="nt-block nt-relays">
          <div>
            <div className="lp-head">
              <p className="lp-eyebrow">{n.relays.eyebrow}</p>
              <h2>{n.relays.title}</h2>
              <p>{n.relays.text}</p>
            </div>
            <ul className="nt-relay-list">
              <li>
                <span>
                  <span className="nt-dot" />
                  {n.relays.virginia}
                </span>
                <span className="lp-muted">{n.relays.active}</span>
              </li>
              <li>
                <span>
                  <span className="nt-dot soon" />
                  {n.relays.saopaulo}
                </span>
                <span className="lp-muted">{n.relays.soon}</span>
              </li>
              <li>
                <span>
                  <span className="nt-dot you" />
                  {n.relays.yours}
                </span>
                <a href="https://github.com/lordbasex/go-link-desk/issues/new" target="_blank" rel="noopener">
                  {n.relays.contact}
                </a>
              </li>
            </ul>
          </div>
          <div className="nt-panel nt-map">
            <Suspense fallback={<div className="nt-map-placeholder" />}>
              <RelayMap />
            </Suspense>
          </div>
        </section>

        <p className="nt-privacy">{n.privacy}</p>
      </div>
      <SiteFooter />
    </div>
  );
}

/** Narrow screens draw the path top to bottom. */
function useNarrow(): boolean {
  const q = "(max-width: 640px)";
  const [narrow, setNarrow] = useState(() => typeof window !== "undefined" && !!window.matchMedia?.(q).matches);
  useEffect(() => {
    const m = window.matchMedia?.(q);
    if (!m) return;
    const on = () => setNarrow(m.matches);
    m.addEventListener("change", on);
    return () => m.removeEventListener("change", on);
  }, []);
  return narrow;
}

type Box = { x: number; y: number; w: number; h: number };
type NodeId = "device" | "router" | "mid" | "signal" | "relay";

/** Where each box and wire goes, wide (left to right) or narrow (top to bottom). */
function layout(narrow: boolean) {
  if (!narrow) {
    const box: Record<NodeId, Box> = {
      device: { x: 10, y: 105, w: 110, h: 90 },
      router: { x: 190, y: 105, w: 110, h: 90 },
      mid: { x: 370, y: 105, w: 110, h: 90 },
      signal: { x: 560, y: 30, w: 130, h: 84 },
      relay: { x: 560, y: 186, w: 130, h: 84 },
    };
    return {
      view: "0 0 700 300",
      box,
      wire: {
        a: "M120 150 H190",
        b: "M300 150 H370",
        sig: "M480 150 C520 150 520 72 560 72",
        rel: "M480 150 C520 150 520 228 560 228",
      },
    };
  }
  const box: Record<NodeId, Box> = {
    device: { x: 100, y: 10, w: 140, h: 80 },
    router: { x: 100, y: 130, w: 140, h: 80 },
    mid: { x: 100, y: 250, w: 140, h: 80 },
    signal: { x: 10, y: 380, w: 150, h: 80 },
    relay: { x: 180, y: 380, w: 150, h: 80 },
  };
  return {
    view: "0 0 340 470",
    box,
    wire: {
      a: "M170 90 V130",
      b: "M170 210 V250",
      sig: "M170 330 C170 355 85 355 85 380",
      rel: "M170 330 C170 355 255 355 255 380",
    },
  };
}

/** Which wires carry packets while a step runs. */
const FLOWS: Partial<Record<StepId, ("a" | "b" | "sig" | "rel")[]>> = {
  signal: ["a", "b", "sig"],
  routers: ["a"],
  udp: ["a", "b", "sig"],
  public: ["a", "b", "sig"],
  nat: ["a", "b", "sig"],
  relay: ["a", "b", "rel"],
};

function PathDiagram({ run, native }: { run: Run; native: Native | null }) {
  const n = t.network;
  const narrow = useNarrow();
  const L = layout(narrow);
  const r = run.report;
  const routers = r ? routersOf(r) : undefined;
  const st = run.steps;
  const lit = (w: "a" | "b" | "sig" | "rel") =>
    w === "rel" ? st.relay === "ok" : w === "sig" ? st.signal === "ok" : st.signal === "ok" || st.udp === "ok";
  const flowing = run.active ? (FLOWS[run.active] ?? []) : [];

  const wanKind = native?.wan ? ipKind(native.wan) : undefined;
  const outer = r ? outerRouterOf(r) : undefined;
  const nodes: Record<NodeId, { title: string; ip: string; sub: string; on: boolean; ok?: boolean }> = {
    device: {
      title: n.nodes.device,
      ip: native?.lan ?? (st.lan ? "?" : "—"),
      sub: native?.lan ? n.sub.lan : n.sub.hidden,
      on: !!st.signal,
    },
    router: {
      title: n.nodes.router,
      ip: native?.wan ?? native?.gw ?? (native ? "—" : "?"),
      sub: wanKind === "cgnat" ? n.sub.wanCgnat : wanKind === "private" ? n.sub.wanPrivate : native?.wan ? n.sub.wanPublic : native?.gw ? n.sub.gateway : n.sub.appOnly,
      on: !!st.routers,
    },
    mid: {
      title: (routers ?? 1) > 1 ? n.nodes.outer : n.nodes.internet,
      ip: (outer ?? r?.publicIP) || "—",
      sub: outer && r?.publicIP ? `↗ ${r.publicIP}` : n.sub.publicIP,
      on: !!r?.publicIP || !!outer,
    },
    signal: {
      title: n.nodes.signal,
      ip: "signal.go-link.org",
      sub: r?.signalMs !== undefined ? n.values.ms.replace("{ms}", String(r.signalMs)) : "—",
      on: !!st.signal,
      ok: st.signal === "ok",
    },
    relay: {
      title: n.nodes.relay,
      ip: n.nodes.relayWhere,
      sub: r?.relayMs !== undefined ? n.values.ms.replace("{ms}", String(r.relayMs)) : "—",
      on: !!st.relay,
      ok: st.relay === "ok",
    },
  };

  return (
    <svg className="nt-path" viewBox={L.view} role="img" aria-label={n.pathTitle}>
      {(Object.keys(L.wire) as (keyof typeof L.wire)[]).map((w) => (
        <path key={w} id={`nt-w-${w}`} className={`nt-wire ${w} ${lit(w) ? "lit" : ""}`} d={L.wire[w]} />
      ))}
      {(Object.keys(nodes) as NodeId[]).map((id) => {
        const b = L.box[id];
        const v = nodes[id];
        const dim = id === "mid" && !r?.publicIP && run.state !== "idle" && st.udp === "bad";
        return (
          <g key={id} className={`nt-node ${v.on ? "on" : ""} ${v.ok ? "ok" : ""} ${dim ? "dim" : ""}`} transform={`translate(${b.x} ${b.y})`}>
            <rect width={b.w} height={b.h} rx="14" />
            <NodeIcon id={id} x={b.w / 2} />
            <text className="nt-ttl" x={b.w / 2} y={b.h - 38} textAnchor="middle">
              {v.title}
            </text>
            <text className="nt-ip" x={b.w / 2} y={b.h - 22} textAnchor="middle">
              {v.ip}
            </text>
            <text className="nt-sub" x={b.w / 2} y={b.h - 8} textAnchor="middle">
              {v.sub}
            </text>
          </g>
        );
      })}
      {st.routers && <NatBadge box={L.box.router} label="NAT" />}
      {(routers ?? 1) > 1 && <NatBadge box={L.box.mid} label={(routers ?? 2) > 2 ? `NAT ${routers}` : "NAT 2"} />}
      {flowing.map((w, i) => (
        <Packets key={`${run.active}-${w}`} wire={w} relay={w === "rel"} delay={i * 0.35} />
      ))}
    </svg>
  );
}

/** Packets running along a wire while its step lasts. */
function Packets({ wire, relay, delay }: { wire: string; relay: boolean; delay: number }) {
  return (
    <>
      {[0, 0.45].map((d) => (
        <circle key={d} className={`nt-pkt ${relay ? "relay" : ""}`} r="4.5" cx="0" cy="0">
          <animateMotion dur="0.9s" begin={`${delay + d}s`} repeatCount="indefinite" fill="freeze">
            <mpath href={`#nt-w-${wire}`} />
          </animateMotion>
        </circle>
      ))}
    </>
  );
}

function NatBadge({ box, label }: { box: Box; label: string }) {
  return (
    <g className="nt-nat" transform={`translate(${box.x + box.w / 2 - 24} ${box.y - 9})`}>
      <rect width="48" height="17" rx="8.5" />
      <text x="24" y="12.5" textAnchor="middle">
        {label}
      </text>
    </g>
  );
}

function NodeIcon({ id, x }: { id: NodeId; x: number }) {
  const d: Record<NodeId, string> = {
    device: "M-14 6 h28 v18 h-28 z M-18 29 h36",
    router: "M-16 14 h32 v12 h-32 z M-8 14 l-4 -8 M8 14 l4 -8 M-9 20 h1",
    mid: "M0 6 a11 11 0 1 0 0.01 0 M-11 17 h22 M0 6 c-6 6 -6 16 0 22 c6 -6 6 -16 0 -22",
    signal: "M-12 10 h24 v14 h-24 z M-6 24 v4 h12 v-4 M-6 15 h12",
    relay: "M-14 18 h28 M6 11 l8 7 l-8 7 M-14 18",
  };
  return <path className="nt-ico" d={d[id]} transform={`translate(${x} 0)`} />;
}

function Gauge({ running, verdict }: { running: boolean; verdict: Verdict | null }) {
  const n = t.network.gauge;
  const angle = verdict === "direct" ? -58 : verdict === "maybe" ? 0 : verdict === "relay" ? 58 : -90;
  return (
    <svg className="nt-gauge" viewBox="0 0 320 190" aria-hidden="true">
      <path className="nt-arc track" d="M30 160 A130 130 0 0 1 290 160" />
      <path className="nt-arc direct" d="M30 160 A130 130 0 0 1 95 47.4" />
      <path className="nt-arc maybe" d="M103 42.6 A130 130 0 0 1 217 42.6" />
      <path className="nt-arc relay" d="M225 47.4 A130 130 0 0 1 290 160" />
      <text className="nt-glabel" x="40" y="182">
        {n.direct}
      </text>
      <text className="nt-glabel" x="160" y="22" textAnchor="middle">
        {n.maybe}
      </text>
      <text className="nt-glabel" x="280" y="182" textAnchor="end">
        {n.relay}
      </text>
      <line className={`nt-needle ${running ? "sweep" : ""}`} x1="160" y1="160" x2="160" y2="52" style={running ? undefined : { transform: `rotate(${angle}deg)` }} />
      <circle className="nt-hub" cx="160" cy="160" r="7" />
    </svg>
  );
}

function stepValue(id: StepId, r: Report | null, state: StepState | undefined): { value: string; note?: string } {
  const n = t.network;
  if (!state || state === "run" || !r) return { value: "" };
  const ms = (v?: number) => (v === undefined ? n.values.noAnswer : n.values.ms.replace("{ms}", String(v)));
  switch (id) {
    case "signal":
      return { value: r.signalOk ? ms(r.signalMs) : n.values.noAnswer };
    case "lan":
      return r.native?.lan ? { value: r.native.lan } : { value: n.values.hidden, note: n.notes.lan };
    case "routers": {
      const k = routersOf(r);
      if (k === undefined) return { value: n.values.appOnly };
      return k > 1 || cgnatOf(r) ? { value: String(k), note: n.notes.routers } : { value: String(k) };
    }
    case "udp":
      return r.udp ? { value: n.values.allowed } : { value: n.values.blocked, note: n.notes.udp };
    case "public":
      return { value: r.publicIP ?? n.values.noAnswer, note: r.ipv6 ? n.notes.ipv6 : undefined };
    case "nat":
      if (r.mapping === "independent") return { value: n.values.keeps };
      if (r.mapping === "dependent") return { value: n.values.changes, note: n.notes.dependent };
      return { value: n.values.noAnswer };
    case "relay":
      return r.relayOk
        ? { value: n.values.via.replace("{proto}", (r.relayProtocol ?? "udp").toUpperCase()).replace("{ms}", String(r.relayMs ?? "?")) }
        : { value: n.values.noAnswer };
  }
}

function Steps({ run }: { run: Run }) {
  const n = t.network;
  return (
    <ol className="nt-steps">
      {STEPS.map((id) => {
        const state = run.steps[id];
        const { value, note } = stepValue(id, run.report, state);
        return (
          <li key={id} className={state ?? ""}>
            <span className="nt-mk" aria-hidden="true">
              {state === "ok" ? "✓" : state === "bad" ? "✕" : state === "warn" ? "!" : state === "run" ? "" : "·"}
            </span>
            <span>{n.steps[id]}</span>
            <span className="nt-val">{value}</span>
            {note && <span className="nt-note">{note}</span>}
          </li>
        );
      })}
    </ol>
  );
}

function Result({ verdict }: { verdict: Verdict }) {
  const r = t.network.result;
  return (
    <section className={`nt-panel nt-result ${verdict}`} aria-live="polite">
      <p className="lp-eyebrow">{r.eyebrow}</p>
      <h2>{r[verdict].title}</h2>
      <p>{r[verdict].text}</p>
      <p className="nt-callout">{rich(r.notFault)}</p>
    </section>
  );
}

/** A router's menu as lines: "Label: value" shows the value highlighted. */
function Rule({ lines, wan }: { lines: readonly string[]; wan: string }) {
  return (
    <pre className="nt-rule">
      {lines.map((line, i) => {
        const l = line.replace("{wan}", wan);
        const at = l.indexOf(": ");
        return (
          <Fragment key={i}>
            {at < 0 ? (
              <span className="k">{l}</span>
            ) : (
              <>
                <span className="k">{l.slice(0, at + 1)}</span> <span className="v">{l.slice(at + 2)}</span>
              </>
            )}
            {"\n"}
          </Fragment>
        );
      })}
    </pre>
  );
}

function Fix({ advice, report }: { advice: Advice; report: Report | null }) {
  const f = t.network.fix;
  const g = f[advice];
  const wan = report?.native?.wan && ipKind(report.native.wan) !== "public" ? report.native.wan : f.double.wanUnknown;
  const outer = report ? outerRouterOf(report) : undefined;
  const lines = advice === "double" && outer ? [`${f.double.modemAt}: http://${outer}`, ...f.double.rule] : "rule" in g ? g.rule : [];
  return (
    <div className="nt-fix">
      <div>
        <h3>{g.title}</h3>
        <ol>
          {g.steps.map((s) => (
            <li key={s}>{rich(s)}</li>
          ))}
        </ol>
      </div>
      <div>{"rule" in g ? <Rule lines={lines} wan={wan} /> : <p className="nt-callout">{rich(g.ask)}</p>}</div>
    </div>
  );
}
