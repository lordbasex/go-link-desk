// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { describe, expect, it, vi } from "vitest";
import { ServerError, linkProof, linkProofBound } from "@go-link-desk/shared";
import { ASK_TIMEOUT_MS, Session, type Door, type SessionOptions, type SessionView } from "./session";
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
  getTransceivers() {
    return [];
  }
  close() {}
}

function start(client: FakeClient) {
  const views: SessionView[] = [];
  const s = new Session({
    client: client.asClient(),
    door: { kind: "join", target: { code: "915355636" } },
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
      door: { kind: "join", target: { code: "915355636" } },
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

    // Typing, both ways: the host's shows until its message comes.
    control({ type: "typing", on: true });
    expect(s.current.hostTyping).toBe(true);
    control({ type: "chat", text: "listo" });
    expect(s.current.hostTyping).toBe(false);
    s.sendTyping(true);
    expect(sent.at(-1)).toEqual({ type: "typing", on: true });
  });
});

describe("Screenshots and recording", () => {
  const live = async () => {
    const client = new FakeClient();
    const { s } = start(client);
    await vi.waitFor(() => expect(s.current.status.kind).toBe("waiting"));
    const control = (s as unknown as { onControl(m: unknown): void }).onControl.bind(s);
    const sent: Record<string, unknown>[] = [];
    (s as unknown as { stream: { sendControl(m: unknown): void } }).stream.sendControl = (m) => sent.push(m as Record<string, unknown>);
    return { s, control, sent };
  };

  it("asks nothing without the permission", async () => {
    const { s, sent } = await live();
    expect(await s.askScreenshot()).toBe(false);
    expect(await s.startRecording()).toBe(false);
    expect(sent).toEqual([]);
  });

  it("saves and records only once the host said yes", async () => {
    const { s, control, sent } = await live();
    control({ type: "permissions", permissions: { view: true, record: true } });

    const shot = s.askScreenshot();
    expect(sent.at(-1)).toEqual({ type: "screenshot", id: 1 });
    control({ type: "screenshot", id: 1, ok: true });
    expect(await shot).toBe(true);

    const rec = s.startRecording();
    expect(sent.at(-1)).toEqual({ type: "recording", id: 2, on: true });
    expect(s.current.recording).toBe(false);
    control({ type: "recording", id: 2, on: true });
    expect(await rec).toBe(true);
    expect(s.current.recording).toBe(true);

    s.stopRecording();
    expect(sent.at(-1)).toEqual({ type: "recording", id: 0, on: false });
    expect(s.current.recording).toBe(false);
  });

  it("stops when the host stops it or takes the permission away", async () => {
    const { s, control } = await live();
    control({ type: "permissions", permissions: { view: true, record: true } });
    const rec = s.startRecording();
    control({ type: "recording", id: 1, on: true });
    await rec;
    control({ type: "recording", id: 0, on: false });
    expect(s.current.recording).toBe(false);

    const again = s.startRecording();
    control({ type: "recording", id: 2, on: true });
    await again;
    control({ type: "permissions", permissions: { view: true } });
    expect(s.current.recording).toBe(false);
  });

  it("takes no answer as a no, and turns down a yes that came too late", async () => {
    vi.useFakeTimers();
    try {
      const { s, control, sent } = await live();
      control({ type: "permissions", permissions: { view: true, record: true } });
      const rec = s.startRecording();
      await vi.advanceTimersByTimeAsync(ASK_TIMEOUT_MS);
      expect(await rec).toBe(false);
      control({ type: "recording", id: 1, on: true });
      expect(s.current.recording).toBe(false);
      expect(sent.at(-1)).toEqual({ type: "recording", id: 0, on: false });
    } finally {
      vi.useRealTimers();
    }
  });
});

describe("go-link-desk-terminal", () => {
  type Inner = { onControl(m: unknown): void; stream: { sendControl(m: unknown): void; opts: { onTerminalChannel(ch: unknown): void } } };
  const channel = () => {
    const typed: Uint8Array[] = [];
    return { typed, ch: { readyState: "open", onmessage: null as ((e: { data: unknown }) => void) | null, send: (b: Uint8Array) => typed.push(b) } };
  };
  const bytes = (s: string) => new TextEncoder().encode(s);
  const framed = (id: number, s: string) => {
    const b = bytes(s);
    const f = new Uint8Array(4 + b.length);
    new DataView(f.buffer).setUint32(0, id);
    f.set(b, 4);
    return f;
  };

  it("an older one (1.4): one shell, its first output kept for the page, and its end ends the session", async () => {
    const client = new FakeClient();
    const { s } = start(client);
    await vi.waitFor(() => expect(s.current.status.kind).toBe("waiting"));
    const inner = s as unknown as Inner;
    const sent: unknown[] = [];
    inner.stream.sendControl = (m) => sent.push(m);
    inner.onControl({ type: "permissions", permissions: { view: true, control: true } });
    inner.onControl({ type: "welcome", host: { name: "pi", os: "linux" }, source: "terminal" });
    expect(s.current.host).toEqual({ name: "pi", os: "linux", terminal: true, terminalTabs: false });

    const { typed, ch } = channel();
    inner.stream.opts.onTerminalChannel(ch);
    expect(s.hasTerminal).toBe(true);
    ch.onmessage?.({ data: bytes("pi@pi:~$ ").buffer });
    const seen: string[] = [];
    const detach = s.attachTerminal((id, b) => seen.push(`${id}:${new TextDecoder().decode(b)}`));
    ch.onmessage?.({ data: bytes("ls\r\n").buffer });
    expect(seen).toEqual(["0:pi@pi:~$ ", "0:ls\r\n"]);
    detach();

    s.sendTerminal(0, bytes("exit\r"));
    expect(new TextDecoder().decode(typed[0])).toBe("exit\r");
    s.sendTerminalSize(120.4, 40);
    expect(sent.at(-1)).toEqual({ type: "terminal_size", cols: 120, rows: 40 });
    inner.onControl({ type: "permissions", permissions: { view: true } });
    s.sendTerminal(0, bytes("rm -rf ~\r"));
    expect(typed).toHaveLength(1); // may only watch

    inner.onControl({ type: "terminal_exit", code: 3 });
    expect(s.current.terminalExit).toBe(3);
  });

  it("terminals with names (1.5): each one's bytes after its id, opened, renamed and closed by message; exit closes only that one", async () => {
    const client = new FakeClient();
    const { s } = start(client);
    await vi.waitFor(() => expect(s.current.status.kind).toBe("waiting"));
    const inner = s as unknown as Inner;
    const sent: unknown[] = [];
    inner.stream.sendControl = (m) => sent.push(m);
    inner.onControl({ type: "permissions", permissions: { view: true, control: true } });
    inner.onControl({ type: "welcome", host: { name: "pi", os: "linux" }, source: "terminal", features: ["terminal-tabs"] });
    expect(s.current.host?.terminalTabs).toBe(true);
    inner.onControl({ type: "terminal_tabs", tabs: [{ id: 1, name: "shell", started: 5 }, { id: 7, name: "logs", started: 9 }], max: 16 });
    expect(s.current.terminalTabs).toEqual([{ id: 1, name: "shell", started: 5 }, { id: 7, name: "logs", started: 9 }]);

    const { typed, ch } = channel();
    inner.stream.opts.onTerminalChannel(ch);
    ch.onmessage?.({ data: framed(7, "tail -f").buffer });
    const seen: string[] = [];
    s.attachTerminal((id, b) => seen.push(`${id}:${new TextDecoder().decode(b)}`));
    ch.onmessage?.({ data: framed(1, "$ ").buffer });
    expect(seen).toEqual(["7:tail -f", "1:$ "]);

    s.sendTerminal(7, bytes("q"));
    expect([...typed[0]!]).toEqual([0, 0, 0, 7, 113]);
    s.openTerminal("build");
    s.renameTerminal(7, "  syslog ");
    s.closeTerminal(1);
    expect(sent.slice(-3)).toEqual([
      { type: "terminal_open", name: "build" },
      { type: "terminal_rename", id: 7, name: "syslog" },
      { type: "terminal_close", id: 1 },
    ]);
    inner.onControl({ type: "terminal_exit", id: 7, code: 0 });
    expect(s.current.terminalExit).toBeNull();
  });
});

describe("Linked computers", () => {
  const device = {
    deviceId: "c2c5f76f-1a25-45d8-abe7-91f51d11f75d",
    linkId: "3f9c0a1b2c3d4e5f",
    token: "q8VY3n1s0bPq0n8oJ1m3Zr2yX7eW5vT4uS6rQ9pO0nM", // gitleaks:allow (test value)
    signalUrl: "wss://x/ws",
    name: "mac",
    label: "",
    os: "darwin",
    favorite: false,
    savedAt: 0,
    lastUsed: 0,
    thumb: "",
  };

  function startDoor(client: FakeClient, door: Door, extra: Partial<SessionOptions> = {}) {
    client.joinReply = { type: "paired", remote: "H", session_id: "S" };
    const s = new Session({
      client: client.asClient(),
      door,
      name: "Ana",
      browser: "Firefox",
      onChange: () => undefined,
      RTCPeerConnectionImpl: NoPC as unknown as typeof RTCPeerConnection,
      ...extra,
    });
    void s.start();
    const inner = s as unknown as {
      onControl(m: unknown): void;
      stream: { sendControl(m: unknown): boolean; pendingControl: string[]; fingerprints(): { host: string; viewer: string } };
    };
    const sent: unknown[] = [];
    return {
      s,
      sent,
      control: (m: unknown) => inner.onControl(m),
      // The connection's certificates as this viewer sees them.
      certificates: (host: string, viewer: string) => {
        inner.stream.fingerprints = () => ({ host, viewer });
      },
      queued: () => inner.stream.pendingControl.map((x) => JSON.parse(x) as Record<string, string>),
      spy: () => {
        inner.stream.sendControl = (m) => (sent.push(m), true);
      },
    };
  }

  it("come back with reach: the computer proves itself before the token, then the password", async () => {
    const client = new FakeClient();
    const { s, sent, control, queued, spy } = startDoor(client, { kind: "reach", device });
    await vi.waitFor(() => expect(s.current.status.kind).toBe("waiting"));
    expect(client.sent[0]).toEqual({ type: "reach", app: "go-link-desk", device_id: device.deviceId });
    expect(client.signals()[0]).toMatchObject({ kind: "knock", name: "Ana" });
    const challenge = queued()[0]!;
    expect(challenge).toMatchObject({ type: "auth_challenge", link_id: device.linkId });
    expect(JSON.stringify(queued())).not.toContain(device.token); // never before the proof
    spy();

    control({ type: "auth_proof", link_id: device.linkId, proof: linkProof(device.token, challenge.nonce!) });
    expect(sent).toEqual([{ type: "auth", link_id: device.linkId, token: device.token }]);
    // The password is asked only once the computer took the token.
    control({ type: "password_required" });
    expect(s.current.status.kind).toBe("waiting");
    control({ type: "auth_ok", device_id: device.deviceId, link_id: device.linkId });

    control({ type: "password_required" });
    expect(s.current.status).toEqual({ kind: "password", error: null, left: 0, checking: false });
    s.sendPassword("guess");
    expect(sent.at(-1)).toEqual({ type: "password", password: "guess" });
    control({ type: "password_result", ok: false, reason: "wrong", left: 4 });
    expect(s.current.status).toEqual({ kind: "password", error: "wrong", left: 4, checking: false });
    s.sendPassword("right one");
    control({ type: "password_result", ok: true });
    expect(s.current.status.kind).toBe("waiting");
    client.deliver({ type: "signal", from: "H", payload: { kind: "accepted", permissions: { view: true, control: true } } });
    await vi.waitFor(() => expect(s.current.status.kind).toBe("connecting"));
    expect(s.current.permissions?.control).toBe(true);
  });

  it("never give the password to, nor take a screen from, a computer that skipped its proof", async () => {
    const client = new FakeClient();
    const { s, sent, control, spy } = startDoor(client, { kind: "reach", device });
    await vi.waitFor(() => expect(s.current.status.kind).toBe("waiting"));
    spy();
    control({ type: "auth_ok", device_id: device.deviceId, link_id: device.linkId });
    control({ type: "password_required" });
    expect(s.current.status.kind).toBe("waiting");
    s.sendPassword("secret");
    expect(sent).toEqual([]);
    // An acceptance without the proof lets nothing in.
    client.deliver({ type: "signal", from: "H", payload: { kind: "accepted", permissions: { view: true, control: true } } });
    await new Promise((r) => setTimeout(r, 20));
    expect(s.current.status.kind).toBe("waiting");
    expect(s.current.permissions).toBeNull();
  });

  it("wait for auth_ok when the acceptance overtakes it", async () => {
    const client = new FakeClient();
    const { s, control, queued, spy } = startDoor(client, { kind: "reach", device });
    await vi.waitFor(() => expect(s.current.status.kind).toBe("waiting"));
    const nonce = queued()[0]!.nonce!;
    spy();
    control({ type: "auth_proof", link_id: device.linkId, proof: linkProof(device.token, nonce) });
    client.deliver({ type: "signal", from: "H", payload: { kind: "accepted", permissions: { view: true, files: true } } });
    await new Promise((r) => setTimeout(r, 20));
    expect(s.current.status.kind).toBe("waiting");
    control({ type: "auth_ok", device_id: device.deviceId, link_id: device.linkId });
    expect(s.current.status.kind).toBe("connecting");
    expect(s.current.permissions?.files).toBe(true);
  });

  it("take the proof bound to the connection, and from then on only that one", async () => {
    const client = new FakeClient();
    const bound = vi.fn();
    const { s, sent, control, queued, spy, certificates } = startDoor(client, { kind: "reach", device }, { onBound: bound });
    await vi.waitFor(() => expect(s.current.status.kind).toBe("waiting"));
    const nonce = queued()[0]!.nonce!;
    spy();
    certificates("sha-256 HO:ST", "sha-256 VI:EW");
    control({ type: "auth_proof", link_id: device.linkId, proof: linkProof(device.token, nonce), proof2: linkProofBound(device.token, nonce, "sha-256 HO:ST", "sha-256 VI:EW") });
    expect(sent).toEqual([{ type: "auth", link_id: device.linkId, token: device.token }]);
    expect(bound).toHaveBeenCalledOnce();
  });

  it("refuse a proof relayed from another connection, and the plain proof once bound", async () => {
    // Someone in the middle has a certificate of its own on each side: the
    // real computer's proof2 names other fingerprints than this viewer sees.
    for (const [door, proof2] of [
      [{ kind: "reach", device }, (n: string) => linkProofBound(device.token, n, "sha-256 HO:ST", "sha-256 MI:TM")],
      [{ kind: "reach", device: { ...device, bound: true } }, () => undefined],
    ] as const) {
      const client = new FakeClient();
      const { s, sent, control, queued, spy, certificates } = startDoor(client, door as Door);
      await vi.waitFor(() => expect(s.current.status.kind).toBe("waiting"));
      const nonce = queued()[0]!.nonce!;
      spy();
      certificates("sha-256 MI:TM", "sha-256 VI:EW");
      control({ type: "auth_proof", link_id: device.linkId, proof: linkProof(device.token, nonce), proof2: proof2(nonce) });
      expect(sent).toEqual([]);
      expect(s.current.status.kind).toBe("untrusted");
    }
  });

  it("send nothing to a computer that cannot prove itself", async () => {
    const client = new FakeClient();
    const { s, sent, control, spy } = startDoor(client, { kind: "reach", device });
    await vi.waitFor(() => expect(s.current.status.kind).toBe("waiting"));
    spy();
    control({ type: "auth_proof", link_id: device.linkId, proof: "forged" });
    expect(sent).toEqual([]);
    expect(s.current.status.kind).toBe("untrusted");
  });

  it("forget a link the computer no longer has", async () => {
    const client = new FakeClient();
    const unlinked = vi.fn();
    const { s, control } = startDoor(client, { kind: "reach", device }, { onUnlinked: unlinked });
    await vi.waitFor(() => expect(s.current.status.kind).toBe("waiting"));
    control({ type: "auth_failed", error: "gone" });
    expect(s.current.status.kind).toBe("unlinked");
    expect(unlinked).toHaveBeenCalledOnce();
  });

  it("say when the computer is off", async () => {
    const client = new FakeClient();
    client.joinReply = ServerError.DeviceOffline;
    const s = new Session({ client: client.asClient(), door: { kind: "reach", device }, name: "Ana", browser: "x", onChange: () => undefined, RTCPeerConnectionImpl: NoPC as unknown as typeof RTCPeerConnection });
    void s.start();
    await vi.waitFor(() => expect(s.current.status.kind).toBe("offline"));
  });

  it("link with the pairing code once the person there accepts", async () => {
    const client = new FakeClient();
    const linked = vi.fn();
    const { s, sent, control, spy } = startDoor(client, { kind: "claim", code: "113134323" }, { onLinked: linked });
    await vi.waitFor(() => expect(s.current.status.kind).toBe("waiting"));
    expect(client.sent[0]).toEqual({ type: "claim", app: "go-link-desk", code: "113134323" });
    spy();
    client.deliver({ type: "signal", from: "H", payload: { kind: "accepted", permissions: { view: true } } });
    await vi.waitFor(() => expect(sent).toEqual([{ type: "auth" }]));
    control({ type: "auth_ok", device_id: device.deviceId, link_id: device.linkId, token: device.token });
    expect(linked).toHaveBeenCalledWith({ deviceId: device.deviceId, linkId: device.linkId, token: device.token });
  });

  it("ignore link messages in a session with a code", async () => {
    const client = new FakeClient();
    const unlinked = vi.fn();
    const { s, control } = startDoor(client, { kind: "join", target: { code: "915355636" } }, { onUnlinked: unlinked });
    await vi.waitFor(() => expect(s.current.status.kind).toBe("waiting"));
    control({ type: "unlinked" });
    control({ type: "password_required" });
    expect(s.current.status.kind).toBe("waiting");
    expect(unlinked).not.toHaveBeenCalled();
  });
});
