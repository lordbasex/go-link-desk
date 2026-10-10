// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import {
  APP,
  HostStream,
  ServerError,
  SignalError,
  endReason,
  type EndReason,
  type Envelope,
  type InviteTarget,
  type Permissions,
  type RejectReason,
  type SignalClient,
} from "@go-link-desk/shared";

/** The host as its welcome message describes it. */
export interface HostInfo {
  name: string;
  os: string;
}

/** What the host says it sends (stream_stats). */
export interface HostSent {
  fps: number;
  width: number;
  height: number;
  kbps: number;
  codec: string;
}

/** Where a session is, from the knock to its end. */
export type SessionStatus =
  | { kind: "joining" }
  | { kind: "waiting" }
  | { kind: "connecting" }
  | { kind: "live" }
  | { kind: "reconnecting" }
  | { kind: "rejected"; reason: RejectReason }
  | { kind: "ended"; reason: EndReason }
  | { kind: "notFound" }
  | { kind: "rateLimited" }
  | { kind: "unreachable" };

export interface SessionView {
  status: SessionStatus;
  permissions: Permissions | null;
  host: HostInfo | null;
  sent: HostSent | null;
  video: MediaStream | null;
}

export interface SessionOptions {
  client: SignalClient;
  target: InviteTarget;
  name: string;
  browser: string;
  onChange: (view: SessionView) => void;
  /** Injectable for tests. */
  RTCPeerConnectionImpl?: typeof RTCPeerConnection;
}

/** The session states that can still change. */
export function isOver(s: SessionStatus): boolean {
  return !["joining", "waiting", "connecting", "live", "reconnecting"].includes(s.kind);
}

/**
 * One visit to a host: join its room with the code or invitation, knock,
 * and if the person at the host accepts, receive the session. It owns the
 * signaling connection: leaving signalhub's session (closing the
 * connection) is how a viewer leaves the room.
 */
export class Session {
  private view: SessionView = { status: { kind: "joining" }, permissions: null, host: null, sent: null, video: null };
  private stream: HostStream | null = null;
  private offLeft: (() => void) | null = null;
  private stopped = false;

  constructor(private readonly opts: SessionOptions) {}

  get current(): SessionView {
    return this.view;
  }

  /** The WebRTC side, once the host accepted (for statistics). */
  get hostStream(): HostStream | null {
    return this.stream;
  }

  async start(): Promise<void> {
    const { client, target } = this.opts;
    client.connect();
    // The host offers right after accepting, maybe before anything else
    // listens: keep its signals from the moment of the join.
    const backlog: Envelope[] = [];
    const offBacklog = client.onMessage((env) => {
      if (env.type === "signal") backlog.push(env);
    });
    let joined: Envelope;
    try {
      await client.whenOpen();
      joined = await client.request({ type: "join", app: APP, ...target }, ["joined"]);
    } catch (err) {
      offBacklog();
      if (this.stopped) return;
      if (err instanceof SignalError && err.fromServer) {
        this.finish({ kind: err.message === ServerError.RateLimited ? "rateLimited" : "notFound" });
      } else {
        this.finish({ kind: "unreachable" });
      }
      return;
    }
    offBacklog();
    if (this.stopped) return;
    const host = joined.remote ?? "";
    // The host leaving signalhub ends everything.
    this.offLeft = client.onMessage((env) => {
      if (env.type === "peer_left" && env.from === host) this.finish({ kind: "ended", reason: "signal" });
    });
    this.stream = new HostStream({
      client,
      hostPeerId: host,
      RTCPeerConnectionImpl: this.opts.RTCPeerConnectionImpl,
      onDecision: (d) => {
        if (d.kind === "rejected") this.finish({ kind: "rejected", reason: d.reason });
        else this.set({ status: { kind: "connecting" }, permissions: d.permissions });
      },
      onTrack: (video) => this.set({ video }),
      onState: (s) => {
        if (isOver(this.view.status) || this.view.status.kind === "waiting") return;
        if (s === "connected") this.set({ status: { kind: "live" } });
        else if (s === "reconnecting") this.set({ status: { kind: "reconnecting" } });
        else if (s === "failed") this.finish({ kind: "ended", reason: "network" });
      },
      onControl: (msg) => this.onControl(msg),
    });
    this.stream.start(backlog);
    this.set({ status: { kind: "waiting" } });
    this.stream.knock(this.opts.name, this.opts.browser);
  }

  /** The viewer leaves (Disconnect, or the page closes). */
  hangUp(): void {
    if (!isOver(this.view.status)) this.stream?.hangUp();
    this.finish({ kind: "ended", reason: "viewer" });
  }

  /** Stops without a word (the page goes away). */
  stop(): void {
    this.stopped = true;
    this.hangUp();
  }

  private onControl(msg: unknown): void {
    const m = msg as { type?: unknown; host?: unknown; reason?: unknown } & Record<string, unknown>;
    switch (m.type) {
      case "welcome": {
        const h = (typeof m.host === "object" && m.host !== null ? m.host : {}) as Record<string, unknown>;
        this.set({ host: { name: String(h.name ?? "").slice(0, 64), os: String(h.os ?? "").slice(0, 16) } });
        break;
      }
      case "stream_stats": {
        const n = (k: string) => (typeof m[k] === "number" && Number.isFinite(m[k]) ? (m[k] as number) : 0);
        this.set({ sent: { fps: n("fps"), width: n("width"), height: n("height"), kbps: n("kbps"), codec: String(m.codec ?? "").slice(0, 16) } });
        break;
      }
      case "session_end":
        this.finish({ kind: "ended", reason: endReason(m.reason) });
        break;
    }
  }

  private set(patch: Partial<SessionView>): void {
    this.view = { ...this.view, ...patch };
    if (!this.stopped) this.opts.onChange(this.view);
  }

  /** Ends the visit: the session closes and the viewer leaves the room. */
  private finish(status: SessionStatus): void {
    if (isOver(this.view.status)) return;
    this.offLeft?.();
    this.offLeft = null;
    this.stream?.close();
    this.opts.client.close();
    this.set({ status, video: null });
  }
}
