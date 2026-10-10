// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import { afterEach, describe, expect, it, vi } from "vitest";
import { SignalClient, SignalError, testSignalServer, type Envelope } from "../src";

/** Minimal in-memory WebSocket driven by the test. */
class FakeSocket {
  static instances: FakeSocket[] = [];
  onmessage: ((ev: { data: unknown }) => void) | null = null;
  onclose: (() => void) | null = null;
  sent: Envelope[] = [];
  closed = false;
  constructor(readonly url: string) {
    FakeSocket.instances.push(this);
  }
  send(data: string) {
    this.sent.push(JSON.parse(data) as Envelope);
  }
  close() {
    if (this.closed) return;
    this.closed = true;
    this.onclose?.();
  }
  receive(env: Envelope) {
    this.onmessage?.({ data: JSON.stringify(env) });
  }
  static last(): FakeSocket {
    return FakeSocket.instances[FakeSocket.instances.length - 1]!;
  }
}

function newClient() {
  return new SignalClient({
    url: "wss://signal.example/ws",
    WebSocketImpl: FakeSocket as unknown as typeof WebSocket,
    minBackoffMs: 10,
    maxBackoffMs: 20,
    requestTimeoutMs: 200,
  });
}

afterEach(() => {
  FakeSocket.instances = [];
  vi.useRealTimers();
});

describe("SignalClient", () => {
  it("opens on hello and keeps ICE servers in memory", () => {
    const c = newClient();
    const states: string[] = [];
    c.onState((s) => states.push(s));
    c.connect();
    const ws = FakeSocket.last();
    expect(ws.url).toBe("wss://signal.example/ws?v=1");
    ws.receive({ type: "hello", peer_id: "A", ice_servers: [{ urls: ["stun:s"] }] });
    expect(c.state).toBe("open");
    expect(c.peerId).toBe("A");
    expect(c.iceServers).toEqual([{ urls: ["stun:s"] }]);
    expect(states).toEqual(["connecting", "open"]);
    c.close();
    expect(c.iceServers).toEqual([]);
  });

  it("matches replies to requests in order", async () => {
    const c = newClient();
    c.connect();
    const ws = FakeSocket.last();
    ws.receive({ type: "hello", peer_id: "A" });

    const list = c.request({ type: "rooms_list", app: "x" }, ["rooms"]);
    const claim = c.request({ type: "claim", app: "x", code: "1" }, ["paired"]);
    expect(ws.sent.map((m) => m.type)).toEqual(["rooms_list", "claim"]);

    ws.receive({ type: "signal", from: "Z" }); // unrelated message
    ws.receive({ type: "rooms", rooms: [] });
    ws.receive({ type: "error", error: "invalid or expired code" });

    await expect(list).resolves.toMatchObject({ type: "rooms" });
    await expect(claim).rejects.toEqual(new SignalError("invalid or expired code", true));
  });

  it("rejects pending requests on disconnect and reconnects", async () => {
    const c = newClient();
    c.connect();
    const first = FakeSocket.last();
    first.receive({ type: "hello", peer_id: "A" });
    const req = c.request({ type: "join", room_id: "r" }, ["joined"]);
    first.close();
    await expect(req).rejects.toMatchObject({ message: "disconnected", fromServer: false });
    expect(c.state).toBe("connecting");
    await vi.waitFor(() => expect(FakeSocket.instances).toHaveLength(2));
    FakeSocket.last().receive({ type: "hello", peer_id: "B" });
    expect(c.peerId).toBe("B");
    c.close();
  });

  it("times out requests without reply", async () => {
    const c = newClient();
    c.connect();
    FakeSocket.last().receive({ type: "hello", peer_id: "A" });
    await expect(c.request({ type: "rooms_list" }, ["rooms"])).rejects.toMatchObject({ message: "request timed out" });
    c.close();
  });

  it("reset opens a brand new connection", () => {
    const c = newClient();
    c.connect();
    FakeSocket.last().receive({ type: "hello", peer_id: "A" });
    c.reset();
    expect(FakeSocket.instances).toHaveLength(2);
    expect(FakeSocket.instances[0]!.closed).toBe(true);
    expect(c.state).toBe("connecting");
    c.close();
  });
});

describe("testSignalServer", () => {
  it("resolves on hello", async () => {
    const p = testSignalServer("wss://s/ws", 200, FakeSocket as unknown as typeof WebSocket);
    FakeSocket.last().receive({ type: "hello", peer_id: "A" });
    await expect(p).resolves.toBeUndefined();
  });
  it("rejects when the socket closes", async () => {
    const p = testSignalServer("wss://s/ws", 200, FakeSocket as unknown as typeof WebSocket);
    FakeSocket.last().close();
    await expect(p).rejects.toBeInstanceOf(SignalError);
  });
});

describe("hostile servers", () => {
  it("keeps only well formed ICE servers", async () => {
    const { parseICEServers } = await import("../src");
    expect(parseICEServers("nope")).toEqual([]);
    expect(
      parseICEServers([
        { urls: "stun:stun.example:3478" },
        { urls: ["turn:t.example:3478", "http://evil", 5], username: "u", credential: "c" },
        { urls: "javascript:alert(1)" },
        null,
      ]),
    ).toEqual([
      { urls: ["stun:stun.example:3478"] },
      { urls: ["turn:t.example:3478"], username: "u", credential: "c" },
    ]);
    expect(parseICEServers(Array.from({ length: 50 }, () => ({ urls: "stun:x" })))).toHaveLength(10);
  });
});

describe("SignalClient handshake", () => {
  function shakingClient(answer: "ok" | "fail") {
    return new SignalClient({
      url: "ws://192.168.1.20:7373/ws",
      WebSocketImpl: FakeSocket as unknown as typeof WebSocket,
      minBackoffMs: 10,
      maxBackoffMs: 20,
      requestTimeoutMs: 200,
      handshake: (link) =>
        link.request({ type: "panel", app: "x" }, ["paired"]).then((env) => {
          if (answer === "fail" || env.type !== "paired") throw new Error("refused");
        }),
    });
  }

  it("opens only after the handshake, and waiting requests go after it", async () => {
    const c = shakingClient("ok");
    c.connect();
    const ws = FakeSocket.last();
    const join = c.request({ type: "join", app: "x", room_id: "R" }, ["joined"]);
    ws.receive({ type: "hello", peer_id: "P" });
    expect(c.state).toBe("connecting");
    expect(ws.sent.map((e) => e.type)).toEqual(["panel"]);
    ws.receive({ type: "paired", remote: "device" });
    await vi.waitFor(() => expect(c.state).toBe("open"));
    await vi.waitFor(() => expect(ws.sent.map((e) => e.type)).toEqual(["panel", "join"]));
    ws.receive({ type: "joined", room_id: "R" });
    await expect(join).resolves.toMatchObject({ room_id: "R" });
    c.close();
  });

  it("drops the connection when the handshake fails, and tries again", async () => {
    const c = shakingClient("fail");
    c.connect();
    const first = FakeSocket.last();
    first.receive({ type: "hello", peer_id: "P" });
    first.receive({ type: "paired" });
    await vi.waitFor(() => expect(first.closed).toBe(true));
    expect(c.state).toBe("connecting");
    await vi.waitFor(() => expect(FakeSocket.instances.length).toBe(2));
    c.close();
  });

  it("sends nothing of the app's while the handshake runs", () => {
    const c = shakingClient("ok");
    c.connect();
    FakeSocket.last().receive({ type: "hello", peer_id: "P" });
    expect(c.send({ type: "join", app: "x" })).toBe(false);
    c.close();
  });
});
