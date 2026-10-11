// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import {
  APP,
  HostStream,
  ServerError,
  SignalError,
  parseDecision,
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
  /** What the encoder aims at, and what the congestion control estimates the path carries (0: no estimate yet). */
  targetKbps: number;
  estimateKbps: number;
}

/** One of the host's screens. */
export interface HostDisplay {
  id: number;
  name: string;
  width: number;
  height: number;
  pointsWidth: number;
  pointsHeight: number;
  primary: boolean;
}

/** The host's pointer image (cursor). Sizes in points; hot spot in the image's pixels. */
export interface CursorShape {
  url: string;
  hotX: number;
  hotY: number;
  width: number;
  height: number;
  pixels: number;
}

/** Where the host's pointer is, over the display shown (0..1). */
export interface CursorState {
  x: number;
  y: number;
  visible: boolean;
  shape: CursorShape | null;
}

export type Preset = "speed" | "balanced" | "quality";

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
  displays: HostDisplay[];
  display: number;
  /** Something about the host's picture ("no_screen_permission", "capture_failed"). */
  notice: string | null;
}

export interface SessionOptions {
  client: SignalClient;
  target: InviteTarget;
  name: string;
  browser: string;
  onChange: (view: SessionView) => void;
  /** The host's pointer, 60 times a second while it moves: kept out of onChange so the page does not re-render for it. */
  onCursor?: (c: CursorState) => void;
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
  private view: SessionView = { status: { kind: "joining" }, permissions: null, host: null, sent: null, video: null, displays: [], display: 0, notice: null };
  private cursor: CursorState = { x: 0, y: 0, visible: false, shape: null };
  private shapes = new Map<string, CursorShape>();
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

  /** Tells the host the size the picture is shown at, in device pixels. */
  setVideoWant(width: number, height: number): void {
    this.stream?.sendControl({ type: "video_want", width: Math.round(width), height: Math.round(height) });
  }

  /** Picks the preset, and whether to send the display's native pixels. */
  setQuality(preset: Preset, retina: boolean): void {
    this.stream?.sendControl({ type: "quality", preset, retina });
  }

  /** Shows another of the host's displays. */
  selectDisplay(id: number): void {
    this.stream?.sendControl({ type: "display_select", id });
  }

  /** Draws the pointer into the picture (true) or here, from its shape and position (false). */
  setCursorInVideo(inVideo: boolean): void {
    this.stream?.sendControl({ type: "cursor_mode", in_video: inVideo });
    if (inVideo) this.opts.onCursor?.({ ...this.cursor, visible: false });
  }

  /** Sends a pointer move (unreliable). */
  sendPointer(packet: ArrayBuffer): void {
    this.stream?.sendPointer(packet);
  }

  /** Sends a button, wheel, key, text or release-all packet (reliable). */
  sendInput(packet: ArrayBuffer): void {
    this.stream?.sendInput(packet);
  }

  /** Flips the latency beacon: odd turns it white, even black. */
  flipBeacon(seq: number): void {
    this.stream?.sendControl({ type: "beacon", seq: seq & 0xffff });
  }

  private onControl(msg: unknown): void {
    const m = msg as { type?: unknown; host?: unknown; reason?: unknown } & Record<string, unknown>;
    const n = (k: string) => (typeof m[k] === "number" && Number.isFinite(m[k]) ? (m[k] as number) : 0);
    switch (m.type) {
      case "welcome": {
        const h = (typeof m.host === "object" && m.host !== null ? m.host : {}) as Record<string, unknown>;
        this.set({ host: { name: String(h.name ?? "").slice(0, 64), os: String(h.os ?? "").slice(0, 16) } });
        break;
      }
      case "stream_stats":
        this.set({
          sent: {
            fps: n("fps"), width: n("width"), height: n("height"), kbps: n("kbps"), codec: String(m.codec ?? "").slice(0, 16),
            targetKbps: n("target_kbps"), estimateKbps: n("estimate_kbps"),
          },
        });
        break;
      case "displays":
        this.set({ displays: parseDisplays(m.displays), display: n("current"), notice: this.view.notice });
        break;
      case "permissions": {
        const d = parseDecision({ kind: "accepted", permissions: m.permissions });
        if (d?.kind === "accepted") this.set({ permissions: d.permissions });
        break;
      }
      case "notice":
        this.set({ notice: String(m.code ?? "").slice(0, 40) || null });
        break;
      case "cursor": {
        const id = String(m.id ?? "").slice(0, 32);
        let shape = this.shapes.get(id);
        if (!shape && typeof m.png === "string" && m.png.length < 400_000 && this.shapes.size < 64) {
          shape = { url: `data:image/png;base64,${m.png}`, hotX: n("hot_x"), hotY: n("hot_y"), width: n("width"), height: n("height"), pixels: n("pixels") };
          this.shapes.set(id, shape);
        }
        if (shape) {
          this.cursor = { ...this.cursor, shape };
          this.opts.onCursor?.(this.cursor);
        }
        break;
      }
      case "cursor_pos":
        this.cursor = { ...this.cursor, x: n("x"), y: n("y"), visible: m.visible === true };
        this.opts.onCursor?.(this.cursor);
        break;
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

/** Reads the host's displays message (bounded: a host has a handful). */
export function parseDisplays(v: unknown): HostDisplay[] {
  if (!Array.isArray(v)) return [];
  return v.slice(0, 16).map((d: Record<string, unknown>) => {
    const n = (k: string) => (typeof d[k] === "number" && Number.isFinite(d[k]) ? (d[k] as number) : 0);
    return {
      id: n("id"), name: String(d.name ?? "").slice(0, 64), width: n("width"), height: n("height"),
      pointsWidth: n("points_width"), pointsHeight: n("points_height"), primary: d.primary === true,
    };
  });
}
