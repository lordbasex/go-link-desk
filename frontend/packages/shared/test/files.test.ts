// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { describe, expect, it } from "vitest";
import { DOWNLOAD_CHUNK, FilesClient, UPLOAD_CHUNK, memorySink, type FilesChannel } from "../src/files";

/** A files channel wired to a tiny host in memory. */
function fakeHost(files: Record<string, Uint8Array>) {
  const listeners: Record<string, ((e: MessageEvent) => void)[]> = { message: [], bufferedamountlow: [], close: [] };
  const received: Uint8Array[] = [];
  let upload: { id: string; name: string; size: number } | null = null;
  const reply = (v: unknown) => queueMicrotask(() => listeners.message!.forEach((fn) => fn(new MessageEvent("message", { data: JSON.stringify(v) }))));
  const ch: FilesChannel & { sent: (string | ArrayBuffer)[] } = {
    readyState: "open",
    bufferedAmount: 0,
    bufferedAmountLowThreshold: 0,
    binaryType: "blob",
    sent: [],
    addEventListener: (t, fn) => listeners[t]!.push(fn),
    removeEventListener: (t, fn) => (listeners[t] = listeners[t]!.filter((f) => f !== fn)),
    send(data) {
      this.sent.push(data);
      if (typeof data !== "string") {
        received.push(new Uint8Array(data));
        return;
      }
      const m = JSON.parse(data);
      if (m.type === "begin") upload = m;
      if (m.type === "end" && upload) reply({ type: "upload_result", id: m.id, ok: true, name: upload.name });
      if (m.type === "list") reply({ type: "listing", id: m.id, path: m.path, entries: [{ name: "Documents", dir: true, size: 0, mtime: 1 }] });
      if (m.type === "download") {
        const f = files[m.path];
        reply(f ? { type: "download_ready", id: m.id, name: m.path, size: f.length } : { type: "download_error", id: m.id, error: "not found" });
      }
      if (m.type === "read") {
        const f = files["big.bin"]!;
        const piece = f.slice(m.offset, m.offset + m.length);
        const b = new ArrayBuffer(8 + piece.length);
        new DataView(b).setBigUint64(0, BigInt(m.offset));
        new Uint8Array(b, 8).set(piece);
        queueMicrotask(() => listeners.message!.forEach((fn) => fn(new MessageEvent("message", { data: b }))));
      }
    },
  };
  return { ch, received };
}

describe("FilesClient", () => {
  it("uploads in pieces and gets the saved name", async () => {
    const { ch, received } = fakeHost({});
    const c = new FilesClient(ch);
    const data = new Uint8Array(UPLOAD_CHUNK * 2 + 5).map((_, i) => i % 251);
    const file = Object.assign(new Blob([data]), { name: "a.bin" });
    const progress: number[] = [];
    expect(await c.upload(file, (n) => progress.push(n))).toBe("a.bin");
    expect(received.map((r) => r.length)).toEqual([UPLOAD_CHUNK, UPLOAD_CHUNK, 5]);
    expect(progress.at(-1)).toBe(data.length);
    expect(Buffer.concat(received.map((r) => Buffer.from(r))).equals(Buffer.from(data))).toBe(true);
  });

  it("lists and downloads with several reads in flight, in order", async () => {
    const big = new Uint8Array(DOWNLOAD_CHUNK * 3 + 10).map((_, i) => i % 7);
    const { ch } = fakeHost({ "big.bin": big });
    const c = new FilesClient(ch);
    expect((await c.list(""))[0]).toMatchObject({ name: "Documents", dir: true });
    const d = await c.openDownload("big.bin");
    expect(d.size).toBe(big.length);
    let blob: Blob | null = null;
    await c.download(d, memorySink((b) => (blob = b)), () => undefined);
    expect(new Uint8Array(await blob!.arrayBuffer())).toEqual(big);
    await expect(c.openDownload("missing")).rejects.toThrow("not found");
  });
});
