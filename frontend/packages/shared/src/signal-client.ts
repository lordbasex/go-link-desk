// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import { PROTOCOL_VERSION, type Envelope, type ICEServer, type MessageType } from "./protocol";

export type ConnectionState = "connecting" | "open" | "closed";

/** Error reply from signalhub, or a local failure (timeout, disconnect). */
export class SignalError extends Error {
  constructor(
    message: string,
    /** true when the text came from the server (see ServerError). */
    readonly fromServer: boolean,
  ) {
    super(message);
    this.name = "SignalError";
  }
}

/**
 * Runs after hello and before the link counts as open: a server that asks
 * the client to prove something first (the device's local panel). A
 * rejection drops the connection, which is then retried.
 */
export type Handshake = (link: { request(env: Envelope, expect: MessageType[]): Promise<Envelope> }) => Promise<void>;

export interface SignalClientOptions {
  url: string;
  handshake?: Handshake;
  /** Injectable for tests; defaults to the global WebSocket. */
  WebSocketImpl?: typeof WebSocket;
  minBackoffMs?: number;
  maxBackoffMs?: number;
  requestTimeoutMs?: number;
}

interface Pending {
  expect: MessageType[];
  resolve: (env: Envelope) => void;
  reject: (err: SignalError) => void;
  timer: ReturnType<typeof setTimeout>;
}

/** Adds ?v=1 unless the URL already names a protocol version. */
export function endpoint(url: string): string {
  const u = new URL(url);
  if (!u.searchParams.has("v")) u.searchParams.set("v", PROTOCOL_VERSION);
  return u.toString();
}

/**
 * Browser client for signalhub. It reconnects with jittered exponential
 * backoff, keeps the ICE servers from hello in memory only, and offers
 * request() for messages that expect one reply.
 *
 * Replies arrive in the order requests were sent (signalhub handles one
 * connection's messages in order), so pending requests form a FIFO queue.
 */
export class SignalClient {
  private readonly opts: Required<Omit<SignalClientOptions, "WebSocketImpl" | "handshake">> & { WebSocketImpl: typeof WebSocket };
  private readonly handshake: Handshake | null;
  private shaking = false; // hello arrived, the handshake is running
  private ws: WebSocket | null = null;
  private stopped = true;
  private backoff: number;
  private retryTimer: ReturnType<typeof setTimeout> | null = null;
  private pending: Pending[] = [];
  private messageListeners = new Set<(env: Envelope) => void>();
  private stateListeners = new Set<(state: ConnectionState) => void>();

  private _state: ConnectionState = "closed";
  private _peerId = "";
  private _iceServers: ICEServer[] = [];

  constructor(options: SignalClientOptions) {
    this.opts = {
      url: options.url,
      WebSocketImpl: options.WebSocketImpl ?? globalThis.WebSocket,
      minBackoffMs: options.minBackoffMs ?? 1000,
      maxBackoffMs: options.maxBackoffMs ?? 30_000,
      requestTimeoutMs: options.requestTimeoutMs ?? 10_000,
    };
    this.handshake = options.handshake ?? null;
    this.backoff = this.opts.minBackoffMs;
  }

  get url(): string {
    return this.opts.url;
  }
  get state(): ConnectionState {
    return this._state;
  }
  /** peer_id assigned in hello; empty while disconnected. */
  get peerId(): string {
    return this._peerId;
  }
  /** STUN/TURN servers from hello, ready for new RTCPeerConnection(). */
  get iceServers(): ICEServer[] {
    return this._iceServers.map((s) => ({ ...s, urls: [...s.urls] }));
  }

  /** Starts connecting (and reconnecting) until close() is called. */
  connect(): void {
    if (!this.stopped) return;
    this.stopped = false;
    this.open();
  }

  /** Closes the link for good and rejects pending requests. */
  close(): void {
    this.stopped = true;
    if (this.retryTimer) clearTimeout(this.retryTimer);
    this.retryTimer = null;
    this.ws?.close();
    this.ws = null;
    this.handleDown();
  }

  /**
   * Drops the current connection and opens a new one right away. On
   * signalhub a connection belongs to at most one session, so this is how
   * the web leaves a room or unlinks a device.
   */
  reset(): void {
    this.close();
    this.backoff = this.opts.minBackoffMs;
    this.connect();
  }

  onMessage(fn: (env: Envelope) => void): () => void {
    this.messageListeners.add(fn);
    return () => this.messageListeners.delete(fn);
  }

  onState(fn: (state: ConnectionState) => void): () => void {
    this.stateListeners.add(fn);
    return () => this.stateListeners.delete(fn);
  }

  /** Sends one message; returns false while disconnected. */
  send(env: Envelope): boolean {
    if (this._state !== "open" || !this.ws) return false;
    this.ws.send(JSON.stringify(env));
    return true;
  }

  /** Sends one of the handshake's messages (the link is not open yet). */
  private sendShaking(env: Envelope): boolean {
    if (!this.shaking || !this.ws) return false;
    this.ws.send(JSON.stringify(env));
    return true;
  }

  /** Resolves once the link is open (waits for hello). */
  whenOpen(timeoutMs = this.opts.requestTimeoutMs): Promise<void> {
    if (this._state === "open") return Promise.resolve();
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        off();
        reject(new SignalError("signaling server unreachable", false));
      }, timeoutMs);
      const off = this.onState((s) => {
        if (s === "open") {
          clearTimeout(timer);
          off();
          resolve();
        }
      });
    });
  }

  /**
   * Sends env and resolves with the next reply whose type is in expect.
   * An "error" reply rejects with the server's text.
   */
  request(env: Envelope, expect: MessageType[]): Promise<Envelope> {
    // Send synchronously when the link is already open, so several
    // requests keep the order in which they were issued.
    if (this._state === "open") return this.sendRequest(env, expect);
    return this.whenOpen().then(() => this.sendRequest(env, expect));
  }

  private sendRequest(env: Envelope, expect: MessageType[], send = (e: Envelope) => this.send(e)): Promise<Envelope> {
    return new Promise((resolve, reject) => {
      const entry: Pending = {
        expect,
        resolve,
        reject,
        timer: setTimeout(() => {
          this.pending = this.pending.filter((p) => p !== entry);
          reject(new SignalError("request timed out", false));
        }, this.opts.requestTimeoutMs),
      };
      this.pending.push(entry);
      if (!send(env)) {
        clearTimeout(entry.timer);
        this.pending = this.pending.filter((p) => p !== entry);
        reject(new SignalError("not connected", false));
      }
    });
  }

  private setState(state: ConnectionState): void {
    if (this._state === state) return;
    this._state = state;
    for (const fn of [...this.stateListeners]) fn(state);
  }

  private open(): void {
    this.setState("connecting");
    let ws: WebSocket;
    try {
      ws = new this.opts.WebSocketImpl(endpoint(this.opts.url));
    } catch {
      this.scheduleRetry();
      return;
    }
    this.ws = ws;
    ws.onmessage = (ev) => this.handleRaw(ws, ev.data);
    ws.onclose = () => {
      if (this.ws !== ws) return; // a stale socket after reset()
      this.ws = null;
      this.handleDown();
      this.scheduleRetry();
    };
  }

  private handleRaw(ws: WebSocket, data: unknown): void {
    if (this.ws !== ws || typeof data !== "string") return;
    let env: Envelope;
    try {
      const parsed: unknown = JSON.parse(data);
      // A server (maybe a custom one) could send anything: only objects.
      if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) return;
      env = parsed as Envelope;
    } catch {
      return;
    }
    if (env.type === "hello") {
      this._peerId = typeof env.peer_id === "string" ? env.peer_id.slice(0, 64) : "";
      this._iceServers = parseICEServers(env.ice_servers);
      if (this.handshake) {
        this.shake(ws, this.handshake);
        return;
      }
      this.backoff = this.opts.minBackoffMs;
      this.setState("open");
      return;
    }
    const head = this.pending[0];
    if (head && (env.type === "error" || head.expect.includes(env.type))) {
      this.pending.shift();
      clearTimeout(head.timer);
      if (env.type === "error") head.reject(new SignalError(env.error ?? "unknown error", true));
      else head.resolve(env);
    }
    for (const fn of [...this.messageListeners]) fn(env);
  }

  /** Runs the handshake on ws; the link opens only if it succeeds. */
  private shake(ws: WebSocket, handshake: Handshake): void {
    this.shaking = true;
    handshake({ request: (env, expect) => this.sendRequest(env, expect, (e) => this.sendShaking(e)) }).then(
      () => {
        if (this.ws !== ws) return;
        this.shaking = false;
        this.backoff = this.opts.minBackoffMs;
        this.setState("open");
      },
      () => {
        if (this.ws !== ws) return;
        this.shaking = false;
        ws.close(); // retried with backoff by onclose
      },
    );
  }

  private handleDown(): void {
    this.shaking = false;
    this._peerId = "";
    this._iceServers = [];
    const pending = this.pending;
    this.pending = [];
    for (const p of pending) {
      clearTimeout(p.timer);
      p.reject(new SignalError("disconnected", false));
    }
    this.setState(this.stopped ? "closed" : "connecting");
  }

  private scheduleRetry(): void {
    if (this.stopped) return;
    // Full jitter: many clients do not reconnect at the same instant.
    const wait = Math.random() * this.backoff + this.opts.minBackoffMs / 2;
    this.backoff = Math.min(this.backoff * 2, this.opts.maxBackoffMs);
    this.retryTimer = setTimeout(() => {
      this.retryTimer = null;
      if (!this.stopped) this.open();
    }, wait);
  }
}

/**
 * The STUN/TURN servers of a hello, checked: at most 10, each with string
 * stun:/turn:/turns: URLs and string credentials. Anything else is dropped
 * rather than breaking RTCPeerConnection.
 */
export function parseICEServers(v: unknown): ICEServer[] {
  if (!Array.isArray(v)) return [];
  return v.slice(0, 10).flatMap((s): ICEServer[] => {
    if (typeof s !== "object" || s === null) return [];
    const o = s as Record<string, unknown>;
    const list = typeof o.urls === "string" ? [o.urls] : Array.isArray(o.urls) ? o.urls : [];
    const urls = list
      .filter((u): u is string => typeof u === "string" && /^(stun|turns?):/i.test(u) && u.length <= 256)
      .slice(0, 10);
    if (urls.length === 0) return [];
    const server: ICEServer = { urls };
    if (typeof o.username === "string") server.username = o.username.slice(0, 256);
    if (typeof o.credential === "string") server.credential = o.credential.slice(0, 256);
    return [server];
  });
}

/**
 * Opens a throwaway connection and waits for hello. Used before saving a
 * custom signaling server.
 */
export function testSignalServer(url: string, timeoutMs = 5000, WebSocketImpl: typeof WebSocket = globalThis.WebSocket): Promise<void> {
  return new Promise((resolve, reject) => {
    let ws: WebSocket;
    try {
      ws = new WebSocketImpl(endpoint(url));
    } catch {
      reject(new SignalError("could not connect", false));
      return;
    }
    let settled = false;
    const finish = (err?: SignalError) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      if (err) reject(err);
      else resolve();
      ws.close();
    };
    const timer = setTimeout(() => finish(new SignalError("no answer from the server", false)), timeoutMs);
    ws.onmessage = (ev) => {
      try {
        if ((JSON.parse(String(ev.data)) as Envelope).type === "hello") finish();
      } catch {
        // ignore anything that is not our protocol
      }
    };
    ws.onclose = () => finish(new SignalError("could not connect", false));
  });
}
