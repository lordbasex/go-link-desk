// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import {
  APP,
  ClipboardAssembler,
  FilesClient,
  HostStream,
  ServerError,
  SignalError,
  clipboardPieces,
  linkProof,
  linkProofBound,
  parseAuthOk,
  parseDecision,
  randomNonce,
  sameString,
  endReason,
  type EndReason,
  type Envelope,
  type InviteTarget,
  type Permissions,
  type RejectReason,
  type SavedDevice,
  type SignalClient,
} from "@go-link-desk/shared";

/**
 * How a session reaches its computer: with the session code or link the
 * person there shares (join), with the pairing code that links this
 * browser for unattended access (claim, which the person there accepts),
 * or with a link made before (reach, which nobody accepts).
 */
export type Door = { kind: "join"; target: InviteTarget } | { kind: "claim"; code: string } | { kind: "reach"; device: SavedDevice };

/** A new link, to keep: the token is the browser's secret. */
export interface NewLink {
  deviceId: string;
  linkId: string;
  token: string;
}

/** Why the unattended password was not taken. */
export type PasswordError = "wrong" | "blocked" | "locked";

/** The host as its welcome message describes it. */
/** One of go-link-desk-terminal's terminals. */
export interface TerminalTab {
  id: number;
  name: string;
  /** When it was opened (unix ms). */
  started: number;
}

export interface HostInfo {
  name: string;
  os: string;
  /** go-link-desk-terminal: a shell instead of the screen. */
  terminal: boolean;
  /** Its terminals have names and outlive the viewer (1.5 and later); older ones send one shell. */
  terminalTabs?: boolean;
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

/** One line of the session's chat. */
export interface ChatLine {
  fromHost: boolean;
  text: string;
  at: number; // ms since the epoch
}

/** A video call with the person at the host. */
export interface CallState {
  state: "idle" | "ringing" | "active" | "declined" | "ended";
  /** The person at the host has their camera and microphone on. */
  hostVideo: boolean;
  hostAudio: boolean;
}

/** The most characters of a chat message (the host's MaxChat). */
export const MAX_CHAT = 2000;
/** How long "is typing" stays without news from the host. */
export const TYPING_SHOWN_MS = 6000;
/** While writing, "typing" goes again this often; a pause this long is "stopped". */
export const TYPING_EVERY_MS = 3000;
export const TYPING_PAUSE_MS = 4000;

/** The chat lines a session keeps. */
const MAX_CHAT_LINES = 200;
/** The most of the shell's output kept before the page's terminal takes it. */
const MAX_TERMINAL_QUEUE = 1 << 20;
/** How long the host has to answer a screenshot or recording request. */
export const ASK_TIMEOUT_MS = 5000;

/** Where a session is, from the knock to its end. */
export type SessionStatus =
  | { kind: "joining" }
  | { kind: "waiting" }
  /** A linked computer asks for its unattended password. */
  | { kind: "password"; error: PasswordError | null; left: number; checking: boolean }
  | { kind: "connecting" }
  | { kind: "live" }
  | { kind: "reconnecting" }
  | { kind: "rejected"; reason: RejectReason }
  | { kind: "ended"; reason: EndReason }
  | { kind: "notFound" }
  | { kind: "rateLimited" }
  | { kind: "unreachable" }
  /** The computer no longer has this browser's link (revoked, or this browser unlinked). */
  | { kind: "unlinked" }
  /** The linked computer is not on (or go-link Desk is closed there). */
  | { kind: "offline" }
  /** Too many wrong passwords: turned away for now. */
  | { kind: "passwordRefused"; locked: boolean; retryAfter: number }
  /** Whoever answered for the linked computer could not prove it is that computer. */
  | { kind: "untrusted" };

export interface SessionView {
  status: SessionStatus;
  permissions: Permissions | null;
  host: HostInfo | null;
  sent: HostSent | null;
  video: MediaStream | null;
  /** The computer's sound (silent unless the viewer may hear it). */
  audio: MediaStream | null;
  /** What was copied on the host, newest first by id. */
  hostClipboard: { id: number; text: string } | null;
  chat: ChatLine[];
  /** The person at the host is writing a chat message. */
  hostTyping: boolean;
  call: CallState;
  /** The host's camera and microphone in a call. */
  callVideo: MediaStream | null;
  callAudio: MediaStream | null;
  displays: HostDisplay[];
  display: number;
  /** Something about the host's picture ("no_screen_permission", "capture_failed"). */
  notice: string | null;
  /** The host shows that this viewer records its screen (it said yes). */
  recording: boolean;
  /** go-link-desk-terminal's shell ended with this code (the session ends after it). */
  terminalExit: number | null;
  /** go-link-desk-terminal's terminals, oldest first (empty for an older one: one shell, id 0). */
  terminalTabs: TerminalTab[];
  /** The computer's terminals were open before this viewer came (back). */
  terminalResumed: boolean;
}

export interface SessionOptions {
  client: SignalClient;
  door: Door;
  name: string;
  browser: string;
  onChange: (view: SessionView) => void;
  /** The host's pointer, 60 times a second while it moves: kept out of onChange so the page does not re-render for it. */
  onCursor?: (c: CursorState) => void;
  /** A claim got its link: keep it to come back without a code. */
  onLinked?: (link: NewLink) => void;
  /** The computer no longer has this browser's link: forget it here. */
  onUnlinked?: () => void;
  /** Reach: the computer proved itself bound to the connection for the first time (remember it). */
  onBound?: () => void;
  /** Injectable for tests. */
  RTCPeerConnectionImpl?: typeof RTCPeerConnection;
}

/** The session states that can still change. */
export function isOver(s: SessionStatus): boolean {
  return !["joining", "waiting", "password", "connecting", "live", "reconnecting"].includes(s.kind);
}

/**
 * One visit to a host: join its room with the code or invitation, knock,
 * and if the person at the host accepts, receive the session. It owns the
 * signaling connection: leaving signalhub's session (closing the
 * connection) is how a viewer leaves the room.
 */
export class Session {
  private view: SessionView = {
    status: { kind: "joining" }, permissions: null, host: null, sent: null, video: null, audio: null, hostClipboard: null, chat: [], hostTyping: false, call: { state: "idle", hostVideo: false, hostAudio: false }, callVideo: null, callAudio: null, displays: [], display: 0, notice: null, recording: false, terminalExit: null, terminalTabs: [], terminalResumed: false,
  };
  private clipboardIn = new ClipboardAssembler();
  private clipboardSeq = 0;
  private filesClient: FilesClient | null = null;
  private terminalCh: RTCDataChannel | null = null;
  /** The shell's output until the terminal on the page takes it (its first prompt comes at once). */
  private terminalQueue: Uint8Array[] = [];
  private terminalQueued = 0;
  private terminalSink: ((id: number, bytes: Uint8Array) => void) | null = null;
  private terminalReset: (() => void) | null = null;
  private cursor: CursorState = { x: 0, y: 0, visible: false, shape: null };
  private shapes = new Map<string, CursorShape>();
  private stream: HostStream | null = null;
  private offLeft: (() => void) | null = null;
  private stopped = false;
  /** The peer connection is up (a linked browser connects before it is let in). */
  private connected = false;
  /** The nonce the host must prove it knows the link with (reach). */
  private nonce = "";
  /**
   * Reach: the host proved it holds this browser's link (auth_proof), and
   * then took the token (auth_ok). Until both, nothing it says counts: a
   * fake host or signaling server must never get the password, nor show a
   * screen of its own as if it were the computer.
   */
  private proven = false;
  private authed = false;
  /** Reach: an acceptance that came (over signalhub) before auth_ok (over the connection). */
  private early: Permissions | null = null;

  constructor(private readonly opts: SessionOptions) {}

  get current(): SessionView {
    return this.view;
  }

  /** The WebRTC side, once the host accepted (for statistics). */
  get hostStream(): HostStream | null {
    return this.stream;
  }

  async start(): Promise<void> {
    const { client, door } = this.opts;
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
      if (door.kind === "join") joined = await client.request({ type: "join", app: APP, ...door.target }, ["joined"]);
      else if (door.kind === "claim") joined = await client.request({ type: "claim", app: APP, code: door.code }, ["paired"]);
      else joined = await client.request({ type: "reach", app: APP, device_id: door.device.deviceId }, ["paired"]);
    } catch (err) {
      offBacklog();
      if (this.stopped) return;
      if (err instanceof SignalError && err.fromServer) {
        if (err.message === ServerError.RateLimited) this.finish({ kind: "rateLimited" });
        else if (err.message === ServerError.DeviceOffline) this.finish({ kind: "offline" });
        else this.finish({ kind: "notFound" });
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
        if (d.kind === "rejected") {
          this.finish({ kind: "rejected", reason: d.reason });
          return;
        }
        // A linked computer lets the browser in only once it proved itself
        // and took the token; its acceptance may overtake auth_ok (they
        // travel different ways), so it waits for it.
        if (door.kind === "reach" && !this.authed) {
          this.early = d.permissions;
          return;
        }
        this.admitted(d.permissions);
      },
      onTrack: (video) => this.set({ video }),
      onAudio: (audio) => this.set({ audio }),
      onFilesChannel: (ch) => {
        this.filesClient?.close();
        this.filesClient = new FilesClient(ch as unknown as ConstructorParameters<typeof FilesClient>[0]);
      },
      onTerminalChannel: (ch) => {
        // A new connection: the computer sends each terminal's history again.
        if (this.terminalCh && this.view.host?.terminalTabs) {
          this.terminalQueue = [];
          this.terminalQueued = 0;
          this.terminalReset?.();
        }
        this.terminalCh = ch;
        ch.onmessage = (e) => {
          if (typeof e.data === "string") return; // the shell's output is bytes
          const bytes = new Uint8Array(e.data as ArrayBuffer);
          if (this.terminalSink) this.toTerminal(bytes);
          else if (this.terminalQueued + bytes.length <= MAX_TERMINAL_QUEUE) {
            this.terminalQueue.push(bytes);
            this.terminalQueued += bytes.length;
          }
        };
      },
      onCallTrack: (kind, stream) => this.set(kind === "video" ? { callVideo: stream } : { callAudio: stream }),
      onState: (s) => {
        this.connected = s === "connected";
        if (isOver(this.view.status)) return;
        const kind = this.view.status.kind;
        if (kind === "waiting" || kind === "password") {
          if (s === "failed") this.finish({ kind: "ended", reason: "network" });
          return;
        }
        if (s === "connected") this.set({ status: { kind: "live" } });
        else if (s === "reconnecting") this.set({ status: { kind: "reconnecting" } });
        else if (s === "failed") this.finish({ kind: "ended", reason: "network" });
      },
      onControl: (msg) => this.onControl(msg),
    });
    this.stream.start(backlog);
    this.set({ status: { kind: "waiting" } });
    this.stream.knock(this.opts.name, this.opts.browser);
    // A linked browser asks the computer to prove itself first: the token
    // goes only to the computer it was linked to (sent on open).
    if (door.kind === "reach") {
      this.nonce = randomNonce();
      this.stream.sendControl({ type: "auth_challenge", link_id: door.device.linkId, nonce: this.nonce });
    }
  }

  /** The host let this viewer in: a linked browser on a connection already up. */
  private admitted(permissions: Permissions): void {
    this.set({ status: { kind: this.connected ? "live" : "connecting" }, permissions });
    // Accepted with the pairing code: now ask for the link's token.
    if (this.opts.door.kind === "claim") this.stream?.sendControl({ type: "auth" });
  }

  /** Sends the unattended password the computer asked for. */
  sendPassword(password: string): void {
    if (this.view.status.kind !== "password" || !password) return;
    this.set({ status: { ...this.view.status, checking: true } });
    this.stream?.sendControl({ type: "password", password: password.slice(0, 512) });
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

  /** Puts text on the host's clipboard (with the clipboard permission); paste also pastes it there. False if it is too long. */
  sendClipboard(text: string, paste: boolean): boolean {
    const pieces = clipboardPieces(++this.clipboardSeq, text, paste);
    if (!pieces) return false;
    for (const p of pieces) this.stream?.sendControl(p);
    return true;
  }

  /** Asks the person at the host for a video call, saying what this side sends. */
  requestCall(video: boolean, audio: boolean): void {
    this.set({ call: { state: "ringing", hostVideo: false, hostAudio: false } });
    this.stream?.sendControl({ type: "call_request", video, audio });
  }

  /** Sends this viewer's camera and microphone (null or disabled: off), and tells the host. */
  async setCallMedia(video: MediaStreamTrack | null, audio: MediaStreamTrack | null): Promise<void> {
    await this.stream?.setCallMedia(video, audio);
    this.stream?.sendControl({ type: "call_media", video: !!video && video.enabled, audio: !!audio && audio.enabled });
  }

  /** Hangs up the call. */
  endCall(): void {
    void this.stream?.setCallMedia(null, null);
    this.stream?.sendControl({ type: "call_end" });
    this.set({ call: { state: "ended", hostVideo: false, hostAudio: false } });
  }

  /** The host's files (with the files permission), once the channel is open. */
  get files(): FilesClient | null {
    return this.view.permissions?.files ? this.filesClient : null;
  }

  /** Whether the host offered go-link-desk-terminal's shell. */
  get hasTerminal(): boolean {
    return this.terminalCh !== null;
  }

  /**
   * Hands the terminals' output to sink, by terminal id (0: an older
   * computer's one shell), what came before first; onReset is told when it
   * all comes again (a new connection). Returns how to stop.
   */
  attachTerminal(sink: (id: number, bytes: Uint8Array) => void, onReset?: () => void): () => void {
    this.terminalSink = sink;
    this.terminalReset = onReset ?? null;
    for (const b of this.terminalQueue.splice(0)) this.toTerminal(b);
    this.terminalQueued = 0;
    return () => {
      if (this.terminalSink === sink) {
        this.terminalSink = null;
        this.terminalReset = null;
      }
    };
  }

  /** One message of the terminal channel: a terminal's id and its bytes (1.5), or the one shell's bytes. */
  private toTerminal(b: Uint8Array): void {
    if (!this.view.host?.terminalTabs) return this.terminalSink?.(0, b);
    if (b.length < 4) return;
    const id = ((b[0]! << 24) | (b[1]! << 16) | (b[2]! << 8) | b[3]!) >>> 0;
    this.terminalSink?.(id, b.subarray(4));
  }

  /** Types into a terminal (only with the control permission; the computer checks too). */
  sendTerminal(id: number, bytes: Uint8Array): void {
    const ch = this.terminalCh;
    if (ch?.readyState !== "open" || !this.view.permissions?.control) return;
    if (!this.view.host?.terminalTabs) {
      ch.send(bytes as Uint8Array<ArrayBuffer>);
      return;
    }
    const f = new Uint8Array(4 + bytes.length);
    new DataView(f.buffer).setUint32(0, id >>> 0);
    f.set(bytes, 4);
    ch.send(f);
  }

  /** Opens another terminal on the computer (a name, or its own). */
  openTerminal(name = ""): void {
    this.stream?.sendControl({ type: "terminal_open", name: name.slice(0, 40) });
  }

  /** Closes a terminal: its shell ends. */
  closeTerminal(id: number): void {
    this.stream?.sendControl({ type: "terminal_close", id });
  }

  /** Renames a terminal. */
  renameTerminal(id: number, name: string): void {
    const n = name.trim().slice(0, 40);
    if (n) this.stream?.sendControl({ type: "terminal_rename", id, name: n });
  }

  /** Tells go-link-desk-terminal the terminal's size here. */
  sendTerminalSize(cols: number, rows: number): void {
    if (cols > 0 && rows > 0) this.stream?.sendControl({ type: "terminal_size", cols: Math.min(1000, Math.round(cols)), rows: Math.min(1000, Math.round(rows)) });
  }

  /** Writes to the person at the host. */
  sendChat(text: string): void {
    const clean = text.trim().slice(0, MAX_CHAT);
    if (!clean || isOver(this.view.status)) return;
    this.stream?.sendControl({ type: "chat", text: clean });
    this.addChat({ fromHost: false, text: clean, at: Date.now() });
  }

  /** Tells the host this side is writing a chat message (on), or stopped. */
  sendTyping(on: boolean): void {
    if (isOver(this.view.status)) return;
    this.stream?.sendControl({ type: "typing", on });
  }

  private addChat(line: ChatLine): void {
    this.set({ chat: [...this.view.chat, line].slice(-MAX_CHAT_LINES) });
  }

  private typingTimer: ReturnType<typeof setTimeout> | null = null;

  /** The host is writing (on), or stopped; without news for a while, it stopped. */
  private hostTyping(on: boolean): void {
    if (this.typingTimer) clearTimeout(this.typingTimer);
    this.typingTimer = on ? setTimeout(() => this.set({ hostTyping: false }), TYPING_SHOWN_MS) : null;
    if (this.view.hostTyping !== on) this.set({ hostTyping: on });
  }

  private asks = new Map<number, (yes: boolean) => void>();
  private askSeq = 0;

  /** Sends a question the host answers with the same id; no answer in time is a no. */
  private ask(message: Record<string, unknown>): Promise<boolean> {
    if (!this.stream || isOver(this.view.status)) return Promise.resolve(false);
    const id = (this.askSeq = (this.askSeq % 0xffff_ffff) + 1);
    return new Promise((resolve) => {
      const timer = setTimeout(() => this.answer(id, false), ASK_TIMEOUT_MS);
      this.asks.set(id, (yes) => {
        clearTimeout(timer);
        resolve(yes);
      });
      this.stream?.sendControl({ ...message, id });
    });
  }

  private answer(id: number, yes: boolean): void {
    const done = this.asks.get(id);
    this.asks.delete(id);
    done?.(yes);
  }

  /**
   * Asks the host before saving a picture of its screen: true once the
   * person there is shown it (only with the record permission).
   */
  askScreenshot(): Promise<boolean> {
    if (!this.view.permissions?.record) return Promise.resolve(false);
    return this.ask({ type: "screenshot" });
  }

  /**
   * Asks the host to show that this viewer records its screen: true once
   * the person there sees it, and only then may the recording start.
   */
  async startRecording(): Promise<boolean> {
    if (!this.view.permissions?.record) return false;
    const yes = await this.ask({ type: "recording", on: true });
    if (yes && !this.view.permissions?.record) {
      this.stopRecording(); // taken away while asking
      return false;
    }
    return yes;
  }

  /** Tells the host the recording stopped. */
  stopRecording(): void {
    if (!this.view.recording) return;
    this.set({ recording: false });
    if (!isOver(this.view.status)) this.stream?.sendControl({ type: "recording", id: 0, on: false });
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
        const features = Array.isArray(m.features) ? (m.features as unknown[]) : [];
        this.set({
          host: { name: String(h.name ?? "").slice(0, 64), os: String(h.os ?? "").slice(0, 16), terminal: m.source === "terminal", terminalTabs: features.includes("terminal-tabs") },
        });
        break;
      }
      case "terminal_tabs": {
        const tabs = Array.isArray(m.tabs) ? (m.tabs as Record<string, unknown>[]) : [];
        this.set({
          ...(m.resumed === true ? { terminalResumed: true } : {}),
          terminalTabs: tabs.slice(0, 32).map((x) => ({
            id: typeof x.id === "number" ? x.id >>> 0 : 0,
            name: String(x.name ?? "").slice(0, 40),
            started: typeof x.started === "number" ? x.started : 0,
          })),
        });
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
        if (d?.kind === "accepted") this.set({ permissions: d.permissions, recording: this.view.recording && d.permissions.record });
        break;
      }
      case "recording": {
        // The answer to startRecording, or the host stopping it (id 0).
        const id = n("id");
        const on = m.on === true && this.view.permissions?.record === true;
        if (on && !this.asks.has(id)) {
          // A yes that came too late: nothing records here.
          this.stream?.sendControl({ type: "recording", id: 0, on: false });
          break;
        }
        if (on || this.view.recording) this.set({ recording: on });
        this.answer(id, on);
        break;
      }
      case "screenshot":
        this.answer(n("id"), m.ok === true);
        break;
      case "terminal_exit":
        // A computer with terminals closes that one only; an older one ends the session after.
        if (!this.view.host?.terminalTabs) this.set({ terminalExit: n("code") });
        break;
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
      case "clipboard": {
        const text = this.clipboardIn.add({ id: m.id, part: m.part, parts: m.parts, text: m.text });
        if (text !== null && this.view.permissions?.clipboard) this.set({ hostClipboard: { id: (this.view.hostClipboard?.id ?? 0) + 1, text } });
        break;
      }
      case "call_state": {
        const states = ["idle", "ringing", "active", "declined", "ended"] as const;
        const state = states.find((s) => s === m.state);
        if (state) this.set({ call: { state, hostVideo: m.host_video === true, hostAudio: m.host_audio === true } });
        if (state === "declined" || state === "ended") void this.stream?.setCallMedia(null, null);
        break;
      }
      case "chat": {
        const text = typeof m.text === "string" ? m.text.slice(0, MAX_CHAT) : "";
        if (text) {
          this.hostTyping(false); // the message it was writing came
          this.addChat({ fromHost: true, text, at: Date.now() });
        }
        break;
      }
      case "typing":
        this.hostTyping(m.on === true);
        break;
      case "session_end":
        this.finish({ kind: "ended", reason: endReason(m.reason) });
        break;
      default:
        this.onLinkMessage(m);
    }
  }

  /** The link's messages: the computer's proof, the token, the password. */
  private onLinkMessage(m: Record<string, unknown>): void {
    const door = this.opts.door;
    switch (m.type) {
      case "auth_proof": {
        if (door.kind !== "reach" || !this.nonce) return;
        const { token, linkId } = door.device;
        // proof2 ties the proof to this connection's certificates. Binding
        // is fail-closed: whenever this connection has DTLS fingerprints
        // (every real WebRTC connection does), a valid proof2 is required
        // and a proof2-less auth_proof is refused, no matter the link's
        // prior state. Otherwise someone in the middle could strip proof2 on
        // a link's first reach and fall back to the plain proof to steal the
        // token. The plain proof is accepted only when the connection has no
        // fingerprints at all (a genuinely unbindable, legacy path).
        const fp = this.stream?.fingerprints();
        const canBind = !!fp?.host && !!fp.viewer;
        const bound = canBind && typeof m.proof2 === "string" && sameString(m.proof2, linkProofBound(token, this.nonce, fp.host, fp.viewer));
        const plain = typeof m.proof === "string" && sameString(m.proof, linkProof(token, this.nonce));
        const ok = bound || (plain && !canBind && !door.device.bound);
        if (m.link_id !== linkId || !ok) {
          this.finish({ kind: "untrusted" });
          return;
        }
        if (bound && !door.device.bound) this.opts.onBound?.();
        this.nonce = "";
        this.proven = true;
        this.stream?.sendControl({ type: "auth", link_id: linkId, token });
        break;
      }
      case "auth_ok": {
        const ok = parseAuthOk(m);
        if (door.kind === "claim" && ok?.token) this.opts.onLinked?.({ deviceId: ok.device_id, linkId: ok.link_id, token: ok.token });
        if (door.kind === "reach" && this.proven && ok?.link_id === door.device.linkId) {
          this.authed = true;
          const early = this.early;
          this.early = null;
          if (early) this.admitted(early);
        }
        break;
      }
      case "auth_failed":
      case "unlinked":
        if (door.kind === "join") return;
        this.opts.onUnlinked?.();
        this.finish({ kind: "unlinked" });
        break;
      case "password_required":
        // Only a computer that proved itself and took the token may ask.
        if (door.kind === "reach" && this.authed) this.set({ status: { kind: "password", error: null, left: 0, checking: false } });
        break;
      case "password_result": {
        if (this.view.status.kind !== "password") return;
        if (m.ok === true) {
          this.set({ status: { kind: "waiting" } });
          return;
        }
        const reason = m.reason === "blocked" || m.reason === "locked" ? m.reason : "wrong";
        if (reason === "wrong") {
          this.set({ status: { kind: "password", error: "wrong", left: typeof m.left === "number" ? m.left : 0, checking: false } });
        } else {
          this.finish({ kind: "passwordRefused", locked: reason === "locked", retryAfter: typeof m.retry_after === "number" ? m.retry_after : 0 });
        }
        break;
      }
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
    this.set({ status, video: null, audio: null, callVideo: null, callAudio: null, call: { state: "idle", hostVideo: false, hostAudio: false }, recording: false });
    for (const id of [...this.asks.keys()]) this.answer(id, false);
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
