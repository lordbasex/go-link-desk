// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { describe, expect, it, vi } from "vitest";
import { MAX_RECORDING_MEMORY, ScreenRecorder, captureName, recordingType, type StopReason } from "./capture";

describe("captureName", () => {
  it("names the file after the computer and the time, with nothing a file system refuses", () => {
    const when = new Date(2026, 9, 14, 15, 4, 5);
    expect(captureName("Office Mac", when, "png")).toBe("go-link-desk-office-mac-2026-10-14-150405.png");
    expect(captureName(' a/b\\c:*?"<>|‮ d ', when, "webm")).toBe("go-link-desk-a-b-c-d-2026-10-14-150405.webm");
    expect(captureName("Café de Ana", when, "png")).toBe("go-link-desk-café-de-ana-2026-10-14-150405.png");
    expect(captureName("", when, "mp4")).toBe("go-link-desk-2026-10-14-150405.mp4");
  });
});

describe("recordingType", () => {
  it("takes WebM first, MP4 on Safari, nothing without MediaRecorder", () => {
    expect(recordingType(() => true)).toEqual({ mime: "video/webm;codecs=vp9,opus", ext: "webm" });
    expect(recordingType((t) => t.startsWith("video/mp4"))).toEqual({ mime: "video/mp4;codecs=avc1,mp4a.40.2", ext: "mp4" });
    expect(recordingType(() => false)).toBeNull();
    expect(
      recordingType(() => {
        throw new Error("unknown");
      }),
    ).toBeNull();
  });
});

/** A MediaRecorder that hands out what the test pushes. */
class FakeRecorder {
  static last: FakeRecorder;
  state: "inactive" | "recording" = "inactive";
  ondataavailable: ((e: { data: Blob }) => void) | null = null;
  onstop: (() => void) | null = null;
  onerror: (() => void) | null = null;
  options: unknown;
  constructor(_s: unknown, o: unknown) {
    this.options = o;
    FakeRecorder.last = this;
  }
  start() {
    this.state = "recording";
  }
  stop() {
    this.state = "inactive";
    this.ondataavailable?.({ data: new Blob(["end"]) });
    this.onstop?.();
  }
  push(text: string) {
    this.ondataavailable?.({ data: new Blob([text]) });
  }
}

function sink(inMemory: boolean, failWrite = false) {
  const s = {
    chunks: [] as string[],
    closed: false,
    aborted: false,
    inMemory,
    async write(c: Uint8Array) {
      if (failWrite) throw new Error("disk full");
      s.chunks.push(new TextDecoder().decode(c));
    },
    async close() {
      s.closed = true;
    },
    async abort() {
      s.aborted = true;
    },
  };
  return s;
}

describe("ScreenRecorder", () => {
  const start = (inMemory: boolean, failWrite = false) => {
    const out = sink(inMemory, failWrite);
    const stops: StopReason[] = [];
    const r = new ScreenRecorder({} as MediaStream, "video/webm", out, (why) => stops.push(why), FakeRecorder as unknown as typeof MediaRecorder);
    return { r, out, stops, fake: FakeRecorder.last };
  };

  it("writes every piece, in order, and saves when stopped", async () => {
    const { r, out, stops, fake } = start(false);
    fake.push("a");
    fake.push("b");
    r.stop("stopped");
    r.stop("host"); // the first reason counts
    await vi.waitFor(() => expect(stops).toEqual(["stopped"]));
    expect(out.chunks).toEqual(["a", "b", "end"]);
    expect(out.closed).toBe(true);
  });

  it("stops at the memory limit when the browser keeps it", async () => {
    const { out, stops, fake } = start(true);
    // A piece that says it is 1 GB (without the memory for it).
    fake.ondataavailable?.({ data: { size: MAX_RECORDING_MEMORY, arrayBuffer: async () => new TextEncoder().encode("big").buffer } as unknown as Blob });
    await vi.waitFor(() => expect(stops).toEqual(["size"]));
    expect(out.closed).toBe(true);
  });

  it("says it failed, and keeps nothing, when the file cannot be written", async () => {
    const { r, out, stops, fake } = start(false, true);
    fake.push("a");
    r.stop("stopped");
    await vi.waitFor(() => expect(stops).toEqual(["failed"]));
    expect(out.aborted).toBe(true);
  });

  it("ends by itself when the stream does", async () => {
    const { stops, fake } = start(false);
    fake.push("a");
    fake.stop();
    await vi.waitFor(() => expect(stops).toEqual(["ended"]));
  });
});
