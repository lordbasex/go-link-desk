// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// The session between a browser and a go-link Desk host. The browser
// knocks; the person at the host accepts or not; only then does the host
// send a WebRTC offer, which the browser answers. Everything travels inside
// signalhub "signal" payloads, which signalhub never reads.

import type { SignalClient } from "./signal-client";
import type { Envelope } from "./protocol";

export interface RTCSignal {
  kind: "offer" | "answer" | "candidate";
  sdp?: string;
  candidate?: RTCIceCandidateInit;
}

/** What the person at the host let this viewer do. */
export interface Permissions {
  view: boolean;
  control: boolean;
  audio: boolean;
  clipboard: boolean;
  files: boolean;
}

/** Why the host turned a knock down. */
export type RejectReason = "declined" | "timeout" | "busy" | "unavailable";

/** Why a session ended (session_end, or the connection itself). */
export type EndReason = "host" | "viewer" | "network" | "signal";

/** The host's answer to a knock. */
export type Decision = { kind: "accepted"; permissions: Permissions } | { kind: "rejected"; reason: RejectReason };

const REJECT_REASONS: readonly string[] = ["declined", "timeout", "busy", "unavailable"];
const END_REASONS: readonly string[] = ["host", "viewer", "network", "signal"];

/** Reads a host's accepted / rejected payload; null for anything else. */
export function parseDecision(payload: unknown): Decision | null {
  if (typeof payload !== "object" || payload === null) return null;
  const p = payload as { kind?: unknown; permissions?: unknown; reason?: unknown };
  if (p.kind === "accepted") {
    const raw = (typeof p.permissions === "object" && p.permissions !== null ? p.permissions : {}) as Record<string, unknown>;
    const on = (k: string) => raw[k] === true;
    return { kind: "accepted", permissions: { view: on("view"), control: on("control"), audio: on("audio"), clipboard: on("clipboard"), files: on("files") } };
  }
  if (p.kind === "rejected") {
    return { kind: "rejected", reason: (REJECT_REASONS.includes(String(p.reason)) ? p.reason : "declined") as RejectReason };
  }
  return null;
}

/** The end reason of a session_end message (unknown ones read as "host"). */
export function endReason(v: unknown): EndReason {
  return (END_REASONS.includes(String(v)) ? v : "host") as EndReason;
}

/** Limits of the knock's fields, as the host cuts them. */
export const MAX_NAME = 40;

/** "video/H264" -> "H.264", "video/VP8" -> "VP8". */
export function codecName(mime: string): string {
  const name = mime.replace(/^video\//i, "");
  return /^h264$/i.test(name) ? "H.264" : /^h265$/i.test(name) ? "H.265" : name.toUpperCase();
}

export interface StreamStats {
  /** Frames per second being decoded. */
  fps: number | null;
  /** Frames decoded since the session started. */
  framesDecoded: number | null;
  /** The picture's size as received. */
  width: number | null;
  height: number | null;
  /** Round-trip time of the active ICE pair, in milliseconds. */
  rttMs: number | null;
  /** How the media flows: straight to the host, or through the TURN relay. */
  path: "direct" | "relay" | null;
  /** The video codec in use ("H.264", "VP8"). */
  codec: string | null;
  /** How long the frames of the last call waited in the jitter buffer (ms). */
  videoBufferMs?: number | null;
  /** How long the browser took to decode each of them (ms). */
  decodeMs?: number | null;
  /** Since the last call: video packets lost (%), frozen time and
   * freezes, frames dropped, jitter and kilobits per second received. */
  window?: StreamWindow;
}

export interface StreamWindow {
  videoLossPct: number;
  freezeMs: number;
  freezes: number;
  dropped: number;
  jitterMs: number;
  kbps: number;
  seconds: number;
}

export type StreamState = "connecting" | "connected" | "reconnecting" | "failed" | "closed";

export interface HostStreamOptions {
  client: SignalClient;
  hostPeerId: string;
  /** The host's answer to the knock. */
  onDecision?: (d: Decision) => void;
  onTrack?: (stream: MediaStream) => void;
  onState?: (state: StreamState) => void;
  /** Control messages other than ping. */
  onControl?: (message: unknown) => void;
  /** Injectable for tests. */
  RTCPeerConnectionImpl?: typeof RTCPeerConnection;
}

/**
 * One session with a host: knock, then receive its video and control
 * channel. A later offer from the host (an ICE restart after a network
 * change) reuses the same connection.
 */
export class HostStream {
  private pc: RTCPeerConnection | null = null;
  private pending: RTCIceCandidateInit[] = [];
  private offMessage: (() => void) | null = null;
  private closed = false;
  private control: RTCDataChannel | null = null;
  private pendingControl: string[] = [];
  // Signals are applied one after the other: a candidate must never pass
  // the offer it belongs to (with ICE restarts, it would belong to the new
  // credentials and fail against the old ones).
  private queue: Promise<void> = Promise.resolve();

  constructor(private readonly opts: HostStreamOptions) {}

  /**
   * Starts listening to the host. backlog holds signals that arrived
   * before start; they are replayed first.
   */
  start(backlog: Envelope[] = []): void {
    const accept = (env: Envelope) => {
      if (env.type !== "signal" || env.from !== this.opts.hostPeerId) return;
      this.queue = this.queue.then(() => this.handleSignal(env.payload));
    };
    this.offMessage = this.opts.client.onMessage(accept);
    backlog.forEach(accept);
  }

  /** Asks to come in, with the name and browser the host will show. */
  knock(name: string, browser: string): void {
    this.opts.client.send({
      type: "signal",
      to: this.opts.hostPeerId,
      payload: { kind: "knock", name: name.trim().slice(0, MAX_NAME), ua: browser },
    });
  }

  /** Tells the host the viewer hangs up, then closes. */
  hangUp(): void {
    this.sendControl({ type: "session_end" });
    this.close();
  }

  close(): void {
    if (this.closed) return;
    this.closed = true;
    this.offMessage?.();
    // Give a last control message a moment to leave before closing.
    const pc = this.pc;
    const ch = this.control;
    if (pc && ch?.readyState === "open" && ch.bufferedAmount > 0) setTimeout(() => pc.close(), 200);
    else pc?.close();
    this.pc = null;
    this.opts.onState?.("closed");
  }

  /**
   * Sends a JSON message on the reliable "control" channel. Messages sent
   * before the channel opens are kept (up to 20) and flushed on open.
   */
  sendControl(message: unknown): boolean {
    const text = JSON.stringify(message);
    const ch = this.control;
    if (!ch || ch.readyState !== "open") {
      if (this.pendingControl.length < 20) this.pendingControl.push(text);
      return false;
    }
    ch.send(text);
    return true;
  }

  private videoTotals: { emitted: number; buffered: number; decoded: number; decoding: number } | null = null;
  private windowTotals: Record<string, number> | null = null;
  private windowAt = 0;

  async stats(): Promise<StreamStats> {
    const result: StreamStats = { fps: null, framesDecoded: null, width: null, height: null, rttMs: null, path: null, codec: null };
    if (!this.pc) return result;
    const report = await this.pc.getStats();
    const byId = new Map<string, Record<string, unknown>>();
    let selected: Record<string, unknown> | undefined;
    let codecId: string | undefined;
    report.forEach((s: Record<string, unknown>) => {
      byId.set(String(s.id), s);
      if (s.type !== "inbound-rtp" || s.kind !== "video") return;
      const n = (k: string) => (typeof s[k] === "number" ? (s[k] as number) : null);
      // Averages over the frames since the last call, not the whole call.
      const emitted = n("jitterBufferEmittedCount");
      const buffered = n("jitterBufferDelay");
      const decoded = n("framesDecoded");
      const decoding = n("totalDecodeTime");
      const last = this.videoTotals;
      if (emitted !== null && buffered !== null && last && emitted > last.emitted)
        result.videoBufferMs = Math.round(((buffered - last.buffered) / (emitted - last.emitted)) * 1000);
      if (decoded !== null && decoding !== null && last && decoded > last.decoded)
        result.decodeMs = Math.round(((decoding - last.decoding) / (decoded - last.decoded)) * 10000) / 10;
      if (emitted !== null && buffered !== null && decoded !== null && decoding !== null)
        this.videoTotals = { emitted, buffered, decoded, decoding };
      result.framesDecoded = decoded;
      result.fps = n("framesPerSecond");
      result.width = n("frameWidth");
      result.height = n("frameHeight");
      if (typeof s.codecId === "string") codecId = s.codecId;
    });
    // The pair in use: the transport names it (Chrome, Safari), else the
    // nominated one that succeeded (Firefox).
    report.forEach((s: Record<string, unknown>) => {
      if (s.type === "transport" && typeof s.selectedCandidatePairId === "string") selected = byId.get(s.selectedCandidatePairId);
    });
    if (!selected)
      report.forEach((s: Record<string, unknown>) => {
        if (s.type === "candidate-pair" && s.nominated && s.state === "succeeded") selected = s;
      });
    if (selected && typeof selected.currentRoundTripTime === "number") result.rttMs = Math.round(selected.currentRoundTripTime * 1000);
    // Totals of the inbound video, for the window since the last call.
    const totals: Record<string, number> = { lost: 0, recv: 0, freezes: 0, freezeS: 0, dropped: 0, bytes: 0, jitter: 0 };
    report.forEach((s: Record<string, unknown>) => {
      if (s.type !== "inbound-rtp" || s.kind !== "video") return;
      const n = (k: string) => (typeof s[k] === "number" ? (s[k] as number) : 0);
      totals.lost! += n("packetsLost");
      totals.recv! += n("packetsReceived");
      totals.freezes! += n("freezeCount");
      totals.freezeS! += n("totalFreezesDuration");
      totals.dropped! += n("framesDropped");
      totals.bytes! += n("bytesReceived");
      totals.jitter = Math.max(totals.jitter!, n("jitter"));
    });
    const now = Date.now();
    const prev = this.windowTotals;
    if (prev && this.windowAt > 0 && now > this.windowAt) {
      const d = (k: string) => Math.max(0, totals[k]! - prev[k]!);
      const lost = d("lost");
      const recv = d("recv");
      const seconds = (now - this.windowAt) / 1000;
      result.window = {
        videoLossPct: lost + recv > 0 ? Math.round((lost / (lost + recv)) * 1000) / 10 : 0,
        freezeMs: Math.round(d("freezeS") * 1000),
        freezes: d("freezes"),
        dropped: d("dropped"),
        jitterMs: Math.round(totals.jitter! * 10000) / 10,
        kbps: Math.round((d("bytes") * 8) / 1000 / seconds),
        seconds,
      };
    }
    this.windowTotals = totals;
    this.windowAt = now;
    const mime = codecId ? byId.get(codecId)?.mimeType : undefined;
    if (typeof mime === "string") result.codec = codecName(mime);
    if (selected) {
      const kind = (id: unknown) => byId.get(String(id))?.candidateType;
      result.path = kind(selected.localCandidateId) === "relay" || kind(selected.remoteCandidateId) === "relay" ? "relay" : "direct";
    }
    return result;
  }

  private sendSignal(sig: RTCSignal): void {
    this.opts.client.send({ type: "signal", to: this.opts.hostPeerId, payload: sig });
  }

  private ensurePeerConnection(): RTCPeerConnection {
    if (this.pc) return this.pc;
    const Impl = this.opts.RTCPeerConnectionImpl ?? globalThis.RTCPeerConnection;
    // STUN/TURN come from signalhub's hello; nothing is hardcoded.
    const pc = new Impl({ iceServers: this.opts.client.iceServers });
    pc.onicecandidate = (ev) => {
      if (ev.candidate) this.sendSignal({ kind: "candidate", candidate: ev.candidate.toJSON() });
    };
    pc.ontrack = (ev) => {
      if (ev.track.kind === "video") this.opts.onTrack?.(ev.streams[0] ?? new MediaStream([ev.track]));
    };
    pc.ondatachannel = (ev) => {
      const ch = ev.channel;
      if (ch.label !== "control") return;
      this.control = ch;
      const flush = () => {
        for (const text of this.pendingControl.splice(0)) ch.send(text);
      };
      if (ch.readyState === "open") flush();
      else ch.onopen = flush;
      ch.onmessage = (m) => {
        let msg: unknown;
        try {
          msg = JSON.parse(String(m.data));
        } catch {
          return; // ignore malformed control messages
        }
        if (typeof msg !== "object" || msg === null) return;
        // The host measures the peer-to-peer round trip with pings.
        const ping = msg as { type?: unknown; id?: unknown };
        if (ping.type === "ping") {
          if (ch.readyState === "open") ch.send(JSON.stringify({ type: "pong", id: ping.id }));
          return;
        }
        this.opts.onControl?.(msg);
      };
    };
    pc.onconnectionstatechange = () => {
      const s = pc.connectionState;
      if (s === "connected") this.opts.onState?.("connected");
      // The host restarts ICE (a new offer) to bring the session back.
      else if (s === "disconnected" || s === "failed") this.opts.onState?.("reconnecting");
    };
    this.pc = pc;
    this.opts.onState?.("connecting");
    return pc;
  }

  private async handleSignal(payload: unknown): Promise<void> {
    if (this.closed) return;
    const decision = parseDecision(payload);
    if (decision) {
      this.opts.onDecision?.(decision);
      return;
    }
    const sig = payload as RTCSignal | undefined;
    if (!sig || typeof sig !== "object") return;
    if (sig.kind === "offer" && sig.sdp) {
      try {
        const pc = this.ensurePeerConnection();
        await pc.setRemoteDescription({ type: "offer", sdp: sig.sdp });
        for (const c of this.pending.splice(0)) await pc.addIceCandidate(c).catch(() => undefined);
        const answer = await pc.createAnswer();
        await pc.setLocalDescription(answer);
        this.sendSignal({ kind: "answer", sdp: answer.sdp ?? "" });
      } catch {
        this.opts.onState?.("failed");
      }
    } else if (sig.kind === "candidate" && sig.candidate) {
      // A bad candidate is the host's problem, not the session's.
      if (this.pc?.remoteDescription) await this.pc.addIceCandidate(sig.candidate).catch(() => undefined);
      // Before the offer a host sends a handful, never hundreds.
      else if (this.pending.length < 50) this.pending.push(sig.candidate);
    }
  }
}
