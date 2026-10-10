// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { SignalError, type Envelope, type SignalClient } from "@go-link-desk/shared";

/**
 * A SignalClient stand-in for tests: join answers what the test set, the
 * test delivers the host's messages and reads what was sent.
 */
export class FakeClient {
  sent: Envelope[] = [];
  connected = false;
  closed = false;
  iceServers = [];
  peerId = "me";
  /** What join gets: a joined envelope, or a server error text. */
  joinReply: Envelope | string = { type: "joined", remote: "H", room_id: "R", session_id: "S" };
  private listeners = new Set<(env: Envelope) => void>();

  connect() {
    this.connected = true;
    this.closed = false;
  }
  close() {
    this.closed = true;
  }
  whenOpen() {
    return Promise.resolve();
  }
  send(env: Envelope) {
    this.sent.push(env);
    return true;
  }
  request(env: Envelope): Promise<Envelope> {
    this.sent.push(env);
    const r = this.joinReply;
    return typeof r === "string" ? Promise.reject(new SignalError(r, true)) : Promise.resolve(r);
  }
  onMessage(fn: (env: Envelope) => void) {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }
  deliver(env: Envelope) {
    this.listeners.forEach((fn) => fn(env));
  }
  /** The payloads signalled to the host. */
  signals(): unknown[] {
    return this.sent.filter((e) => e.type === "signal").map((e) => e.payload);
  }
  asClient(): SignalClient {
    return this as unknown as SignalClient;
  }
}
