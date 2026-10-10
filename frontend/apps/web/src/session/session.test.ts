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
    expect(client.signals()).toEqual([{ kind: "knock", name: "Ana", ua: "Chrome 141 on macOS" }]);
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
