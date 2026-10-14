// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { describe, expect, it, vi } from "vitest";
import { HostStream, MAX_NAME, endReason, parseDecision, videoCodecs } from "../src/stream";
import type { Envelope } from "../src/protocol";
import type { SignalClient } from "../src/signal-client";

describe("parseDecision", () => {
  it("reads an acceptance with its permissions", () => {
    expect(parseDecision({ kind: "accepted", permissions: { view: true, control: "yes" } })).toEqual({
      kind: "accepted",
      permissions: { view: true, control: false, audio: false, clipboard: false, files: false, record: false },
    });
  });
  it("reads a rejection, and an unknown reason as declined", () => {
    expect(parseDecision({ kind: "rejected", reason: "busy" })).toEqual({ kind: "rejected", reason: "busy" });
    expect(parseDecision({ kind: "rejected", reason: "<b>" })).toEqual({ kind: "rejected", reason: "declined" });
  });
  it("ignores everything else", () => {
    expect(parseDecision({ kind: "offer", sdp: "x" })).toBeNull();
    expect(parseDecision(null)).toBeNull();
    expect(parseDecision("accepted")).toBeNull();
  });
});

describe("endReason", () => {
  it("keeps known reasons", () => {
    expect(endReason("network")).toBe("network");
    expect(endReason("whatever")).toBe("host");
  });
});

/** A SignalClient stand-in: records what is sent, delivers what the test says. */
function fakeClient() {
  const listeners = new Set<(env: Envelope) => void>();
  const sent: Envelope[] = [];
  return {
    sent,
    iceServers: [],
    send: (env: Envelope) => (sent.push(env), true),
    onMessage: (fn: (env: Envelope) => void) => (listeners.add(fn), () => listeners.delete(fn)),
    deliver: (env: Envelope) => listeners.forEach((fn) => fn(env)),
  };
}

/** Just enough of RTCPeerConnection to record the order of calls. */
function fakePC(log: string[]) {
  return class {
    remoteDescription: unknown = null;
    connectionState = "new";
    onicecandidate = null;
    ontrack = null;
    ondatachannel = null;
    onconnectionstatechange = null;
    async setRemoteDescription(d: { sdp: string }) {
      await new Promise((r) => setTimeout(r, 5));
      this.remoteDescription = d;
      log.push(`offer ${d.sdp}`);
    }
    async addIceCandidate(c: { candidate: string }) {
      log.push(`candidate ${c.candidate}`);
    }
    async createAnswer() {
      return { type: "answer", sdp: "ans" };
    }
    async setLocalDescription() {}
    getTransceivers() {
      return [];
    }
    close() {
      log.push("close");
    }
  } as unknown as typeof RTCPeerConnection;
}

describe("HostStream", () => {
  it("knocks, hears the decision and answers the offer in order", async () => {
    const client = fakeClient();
    const log: string[] = [];
    const decisions: unknown[] = [];
    const s = new HostStream({
      client: client as unknown as SignalClient,
      hostPeerId: "H",
      onDecision: (d) => decisions.push(d),
      RTCPeerConnectionImpl: fakePC(log),
    });
    s.start();
    s.knock(`  ${"A".repeat(60)} `, "Chrome 141 on macOS", ["vp8"]);
    expect(client.sent[0]).toEqual({ type: "signal", to: "H", payload: { kind: "knock", name: "A".repeat(MAX_NAME), ua: "Chrome 141 on macOS", codecs: ["vp8"] } });

    // Someone else's signal is not the host's.
    client.deliver({ type: "signal", from: "X", payload: { kind: "accepted" } });
    client.deliver({ type: "signal", from: "H", payload: { kind: "accepted", permissions: { view: true } } });
    // An ICE restart: the new candidate must wait for its offer.
    client.deliver({ type: "signal", from: "H", payload: { kind: "offer", sdp: "o1" } });
    client.deliver({ type: "signal", from: "H", payload: { kind: "candidate", candidate: { candidate: "c1" } } });
    await vi.waitFor(() => expect(log).toEqual(["offer o1", "candidate c1"]));
    expect(decisions).toHaveLength(1);
    expect(client.sent[1]).toEqual({ type: "signal", to: "H", payload: { kind: "answer", sdp: "ans" } });
    s.close();
    expect(log).toContain("close");
  });
});

describe("videoCodecs", () => {
  it("names what the browser decodes", () => {
    const caps = {
      codecs: [
        { mimeType: "video/VP8", clockRate: 90000 },
        { mimeType: "video/H264", clockRate: 90000, sdpFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f" },
        { mimeType: "video/H264", clockRate: 90000, sdpFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=640c1f" },
        { mimeType: "video/rtx", clockRate: 90000 },
      ],
      headerExtensions: [],
    } as RTCRtpCapabilities;
    expect(videoCodecs(caps).sort()).toEqual(["h264", "h264-high", "vp8"]);
    expect(videoCodecs({ codecs: [], headerExtensions: [] })).toEqual(["vp8"]);
    expect(videoCodecs(null)).toEqual(["vp8"]);
  });
});

describe("recvonlyMids", () => {
  it("finds the host's receive-only lines, where a call's camera and microphone go", async () => {
    const { recvonlyMids } = await import("../src/stream");
    const sdp = ["v=0", "m=video 9 UDP/TLS/RTP/SAVPF 114", "a=mid:0", "a=sendonly", "m=audio 9 UDP/TLS/RTP/SAVPF 111", "a=mid:1", "a=sendonly", "m=video 9 UDP/TLS/RTP/SAVPF 106", "a=mid:4", "a=recvonly", "m=audio 9 UDP/TLS/RTP/SAVPF 111", "a=mid:5", "a=recvonly", "m=application 9 UDP/DTLS/SCTP webrtc-datachannel", "a=mid:6", ""].join("\r\n");
    expect([...recvonlyMids(sdp)]).toEqual(["4", "5"]);
  });
});
