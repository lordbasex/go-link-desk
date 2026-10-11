// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { describe, expect, it, vi } from "vitest";
import { ServerError } from "@go-link-desk/shared";
import { Session, type SessionView } from "./session";
import { FakeClient } from "./test-utils";

/** A peer connection that never connects: enough to see the session's steps. */
class NoPC {
  remoteDescription = null;
  connectionState = "new";
  onicecandidate = null;
  ontrack = null;
  ondatachannel = null;
  onconnectionstatechange: (() => void) | null = null;
  async setRemoteDescription(d: unknown) {
    this.remoteDescription = d as null;
  }
  async addIceCandidate() {}
  async createAnswer() {
    return { type: "answer", sdp: "ans" };
  }
  async setLocalDescription() {}
  close() {}
}

function start(client: FakeClient) {
  const views: SessionView[] = [];
  const s = new Session({
    client: client.asClient(),
    target: { code: "915355636" },
    name: "Ana",
    browser: "Chrome 141 on macOS",
    onChange: (v) => views.push(v),
    RTCPeerConnectionImpl: NoPC as unknown as typeof RTCPeerConnection,
  });
  void s.start();
  return { s, views, status: () => s.current.status };
}

describe("Session", () => {
  it("joins with the code and knocks", async () => {
    const client = new FakeClient();
    const { status } = start(client);
    await vi.waitFor(() => expect(status().kind).toBe("waiting"));
    expect(client.sent[0]).toEqual({ type: "join", app: "go-link-desk", code: "915355636" });
    expect(client.signals()).toEqual([{ kind: "knock", name: "Ana", ua: "Chrome 141 on macOS", codecs: ["vp8"] }]);
  });

  it("connects once accepted, and learns who the host is", async () => {
    const client = new FakeClient();
    const { s, status } = start(client);
    await vi.waitFor(() => expect(status().kind).toBe("waiting"));
    client.deliver({ type: "signal", from: "H", payload: { kind: "accepted", permissions: { view: true } } });
    await vi.waitFor(() => expect(status().kind).toBe("connecting"));
    expect(s.current.permissions?.view).toBe(true);
    client.deliver({ type: "signal", from: "H", payload: { kind: "offer", sdp: "o" } });
    await vi.waitFor(() => expect(client.signals()).toContainEqual({ kind: "answer", sdp: "ans" }));
  });

  it.each([
    ["declined", "declined"],
    ["busy", "busy"],
  ])("ends when the host says %s", async (reason, want) => {
    const client = new FakeClient();
    const { status } = start(client);
    await vi.waitFor(() => expect(status().kind).toBe("waiting"));
    client.deliver({ type: "signal", from: "H", payload: { kind: "rejected", reason } });
    await vi.waitFor(() => expect(status()).toEqual({ kind: "rejected", reason: want }));
    expect(client.closed).toBe(true); // it left the room
  });

  it("ends when the host leaves", async () => {
    const client = new FakeClient();
    const { status } = start(client);
    await vi.waitFor(() => expect(status().kind).toBe("waiting"));
    client.deliver({ type: "peer_left", from: "someone else" });
    expect(status().kind).toBe("waiting");
    client.deliver({ type: "peer_left", from: "H" });
    expect(status()).toEqual({ kind: "ended", reason: "signal" });
  });

  it.each([
    [ServerError.InvalidInvite, "notFound"],
    [ServerError.RateLimited, "rateLimited"],
  ])("explains a failed join (%s)", async (err, kind) => {
    const client = new FakeClient();
    client.joinReply = err;
    const { status } = start(client);
    await vi.waitFor(() => expect(status().kind).toBe(kind));
  });

  it("hangs up", async () => {
    const client = new FakeClient();
    const { s, status } = start(client);
    await vi.waitFor(() => expect(status().kind).toBe("waiting"));
    s.hangUp();
    expect(status()).toEqual({ kind: "ended", reason: "viewer" });
    expect(client.closed).toBe(true);
  });
});

describe("Session control messages", () => {
  it("reads the displays, notices and the pointer", async () => {
    const client = new FakeClient();
    const cursors: unknown[] = [];
    const s = new Session({
      client: client.asClient(),
      target: { code: "915355636" },
      name: "Ana",
      browser: "x",
      onChange: () => undefined,
      onCursor: (c) => cursors.push(c),
      RTCPeerConnectionImpl: NoPC as unknown as typeof RTCPeerConnection,
    });
    void s.start();
    await vi.waitFor(() => expect(s.current.status.kind).toBe("waiting"));
    // The control channel is HostStream's; its messages reach the session through onControl.
    const control = (s as unknown as { onControl(m: unknown): void }).onControl.bind(s);
    control({ type: "displays", current: 2, displays: [{ id: 1, name: "Built-in", width: 2560, height: 1600, points_width: 1440, points_height: 900, primary: true }, { id: 2, name: "LG", width: 1920, height: 1080, points_width: 1920, points_height: 1080 }] });
    expect(s.current.display).toBe(2);
    expect(s.current.displays.map((d) => d.pointsWidth)).toEqual([1440, 1920]);
    control({ type: "notice", code: "no_screen_permission" });
    expect(s.current.notice).toBe("no_screen_permission");
    control({ type: "cursor", id: "abc", png: "AAAA", hot_x: 8, hot_y: 8, width: 17, height: 23, pixels: 34 });
    control({ type: "cursor_pos", x: 0.5, y: 0.25, visible: true });
    expect(cursors.at(-1)).toMatchObject({ x: 0.5, y: 0.25, visible: true, shape: { hotX: 8, width: 17, pixels: 34 } });
    // A known shape comes back without its image.
    control({ type: "cursor", id: "abc", hot_x: 8, hot_y: 8, width: 17, height: 23 });
    expect(cursors).toHaveLength(3);
  });

  it("takes the host's clipboard only with the permission, and chats both ways", async () => {
    const client = new FakeClient();
    const { s } = start(client);
    await vi.waitFor(() => expect(s.current.status.kind).toBe("waiting"));
    const control = (s as unknown as { onControl(m: unknown): void }).onControl.bind(s);
    const sent: unknown[] = [];
    (s as unknown as { stream: { sendControl(m: unknown): void } }).stream.sendControl = (m) => sent.push(m);

    control({ type: "clipboard", id: 1, part: 0, parts: 1, text: "secret" });
    expect(s.current.hostClipboard).toBeNull(); // no permission yet
    control({ type: "permissions", permissions: { view: true, clipboard: true } });
    control({ type: "clipboard", id: 2, part: 0, parts: 2, text: "ho" });
    expect(s.current.hostClipboard).toBeNull();
    control({ type: "clipboard", id: 2, part: 1, parts: 2, text: "la" });
    expect(s.current.hostClipboard).toEqual({ id: 1, text: "hola" });

    expect(s.sendClipboard("pegar", true)).toBe(true);
    expect(sent.at(-1)).toEqual({ type: "clipboard", id: 1, part: 0, parts: 1, text: "pegar", paste: true });

    control({ type: "chat", text: "¿en qué te ayudo?" });
    s.sendChat("  no anda el wifi  ");
    expect(sent.at(-1)).toEqual({ type: "chat", text: "no anda el wifi" });
    expect(s.current.chat.map((l) => [l.fromHost, l.text])).toEqual([
      [true, "¿en qué te ayudo?"],
      [false, "no anda el wifi"],
    ]);
  });
});
