// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

/** What a files channel needs of an RTCDataChannel (tests stand in for it). */
export interface FilesChannel {
  readonly readyState: string;
  bufferedAmount: number;
  bufferedAmountLowThreshold: number;
  binaryType: string;
  send(data: string | ArrayBuffer): void;
  addEventListener(type: "message" | "bufferedamountlow" | "close", fn: (e: MessageEvent) => void): void;
  removeEventListener(type: "message" | "bufferedamountlow" | "close", fn: (e: MessageEvent) => void): void;
}

/** One entry of a host folder. */
export interface FileEntry {
  name: string;
  dir: boolean;
  size: number;
  mtime: number; // unix seconds
}

/** Where a download's bytes go: a file on disk (File System Access), or memory. */
export interface DownloadSink {
  write(chunk: Uint8Array): Promise<void>;
  close(): Promise<void>;
  abort(): Promise<void>;
}

/** Upload pieces: small enough to interleave with the session's other messages. */
export const UPLOAD_CHUNK = 16 << 10;
/** The most an upload keeps queued in the channel: the session's control and input pass it. */
export const UPLOAD_WINDOW = 1 << 20;
/** A download read, the host's MaxFileChunk. */
export const DOWNLOAD_CHUNK = 60 << 10;
/** Reads a download keeps in flight. */
export const DOWNLOAD_IN_FLIGHT = 16;

export class FilesError extends Error {
  constructor(
    message: string,
    readonly code: string = "",
  ) {
    super(message);
  }
}

type Reply = Record<string, unknown>;

/**
 * Files with the host over the session's "files" channel: send a file
 * (paced by the channel's buffer, so the session keeps flowing), list a
 * folder of the host's home, and download a file pulled piece by piece.
 * One upload and one download at a time, as the host takes them.
 */
export class FilesClient {
  private seq = 0;
  private waiting = new Map<string, (r: Reply) => void>();
  private onBinary: ((b: ArrayBuffer) => void) | null = null;
  private readonly onMessage = (e: MessageEvent) => {
    if (typeof e.data !== "string") {
      this.onBinary?.(e.data as ArrayBuffer);
      return;
    }
    let m: Reply;
    try {
      m = JSON.parse(e.data) as Reply;
    } catch {
      return;
    }
    const id = typeof m.id === "string" ? m.id : "";
    this.waiting.get(id)?.(m);
  };

  constructor(private readonly ch: FilesChannel) {
    ch.binaryType = "arraybuffer";
    ch.bufferedAmountLowThreshold = UPLOAD_WINDOW / 2;
    ch.addEventListener("message", this.onMessage);
  }

  close(): void {
    this.ch.removeEventListener("message", this.onMessage);
    for (const fn of this.waiting.values()) fn({ type: "files_error", error: "closed" });
    this.waiting.clear();
  }

  private nextId(prefix: string): string {
    return `${prefix}${++this.seq}`;
  }

  /** Waits for the next reply to id, the first whose type is in types. */
  private reply(id: string, types: string[]): Promise<Reply> {
    return new Promise((resolve) => {
      this.waiting.set(id, (m) => {
        if (types.includes(String(m.type)) || m.type === "files_error" || m.type === "download_error") {
          this.waiting.delete(id);
          resolve(m);
        }
      });
    });
  }

  private drained(): Promise<void> {
    if (this.ch.bufferedAmount <= UPLOAD_WINDOW) return Promise.resolve();
    return new Promise((resolve) => {
      const go = () => {
        this.ch.removeEventListener("bufferedamountlow", go);
        resolve();
      };
      this.ch.addEventListener("bufferedamountlow", go);
    });
  }

  /** Sends a file; resolves with the name the host saved it as. signal stops it. */
  async upload(file: Blob & { name: string }, onProgress: (sent: number) => void, signal?: AbortSignal): Promise<string> {
    const id = this.nextId("u");
    const result = this.reply(id, ["upload_result"]);
    let early: Reply | null = null;
    void result.then((r) => (early = r));
    this.ch.send(JSON.stringify({ type: "begin", id, name: file.name, size: file.size }));
    for (let off = 0; off < file.size; off += UPLOAD_CHUNK) {
      if (signal?.aborted) {
        this.ch.send(JSON.stringify({ type: "abort", id }));
        throw new FilesError("stopped", "aborted");
      }
      if (early) break; // the host refused it: no point sending the rest
      await this.drained();
      if (this.ch.readyState !== "open") throw new FilesError("the connection closed");
      this.ch.send(await file.slice(off, off + UPLOAD_CHUNK).arrayBuffer());
      onProgress(Math.min(off + UPLOAD_CHUNK, file.size));
    }
    if (!early) this.ch.send(JSON.stringify({ type: "end", id }));
    const r = await result;
    if (r.ok !== true) throw new FilesError(String(r.error ?? "failed"), String(r.code ?? ""));
    return String(r.name ?? file.name);
  }

  /** Lists a folder of the host's home ("" is home itself). */
  async list(path: string): Promise<FileEntry[]> {
    const id = this.nextId("l");
    const r = this.reply(id, ["listing"]);
    this.ch.send(JSON.stringify({ type: "list", id, path }));
    const m = await r;
    if (m.type !== "listing") throw new FilesError(String(m.error ?? "failed"), String(m.code ?? ""));
    return (Array.isArray(m.entries) ? m.entries : []).slice(0, 5000).map((e: Record<string, unknown>) => ({
      name: String(e.name ?? ""),
      dir: e.dir === true,
      size: typeof e.size === "number" ? e.size : 0,
      mtime: typeof e.mtime === "number" ? e.mtime : 0,
    }));
  }

  /** Opens a download: its name and size, before choosing where it goes. */
  async openDownload(path: string): Promise<{ id: string; name: string; size: number }> {
    const id = this.nextId("d");
    const r = this.reply(id, ["download_ready"]);
    this.ch.send(JSON.stringify({ type: "download", id, path }));
    const m = await r;
    if (m.type !== "download_ready") throw new FilesError(String(m.error ?? "failed"), String(m.code ?? ""));
    return { id, name: String(m.name ?? "file"), size: typeof m.size === "number" ? m.size : 0 };
  }

  /** Pulls an opened download into sink, several reads in flight. */
  async download(d: { id: string; size: number }, sink: DownloadSink, onProgress: (got: number) => void, signal?: AbortSignal): Promise<void> {
    let next = 0; // the next offset to ask for
    let got = 0;
    const ask = () => {
      while (next < d.size && next - got < DOWNLOAD_CHUNK * DOWNLOAD_IN_FLIGHT) {
        this.ch.send(JSON.stringify({ type: "read", id: d.id, offset: next, length: DOWNLOAD_CHUNK }));
        next += DOWNLOAD_CHUNK;
      }
    };
    let writing = Promise.resolve();
    const failed = this.reply(d.id, []); // download_error, if any
    try {
      await new Promise<void>((resolve, reject) => {
        void failed.then((m) => reject(new FilesError(String(m.error ?? "failed"))));
        signal?.addEventListener("abort", () => reject(new FilesError("stopped", "aborted")));
        this.onBinary = (b) => {
          const view = new DataView(b);
          const off = Number(view.getBigUint64(0));
          if (off !== got) return reject(new FilesError("a piece arrived out of order"));
          const data = new Uint8Array(b, 8);
          got += data.byteLength;
          writing = writing.then(() => sink.write(data));
          onProgress(got);
          if (got >= d.size) resolve();
          else ask();
        };
        if (d.size === 0) resolve();
        else ask();
      });
      await writing;
      await sink.close();
      this.ch.send(JSON.stringify({ type: "done", id: d.id }));
    } catch (e) {
      this.ch.send(JSON.stringify({ type: "cancel", id: d.id }));
      await sink.abort();
      throw e;
    } finally {
      this.onBinary = null;
      this.waiting.delete(d.id);
    }
  }
}

/** A sink in memory, saved as a Blob at the end (browsers without the File System Access API). */
export function memorySink(onDone: (blob: Blob) => void): DownloadSink {
  const parts: Uint8Array[] = [];
  return {
    async write(c) {
      parts.push(c);
    },
    async close() {
      onDone(new Blob(parts as BlobPart[]));
    },
    async abort() {
      parts.length = 0;
    },
  };
}
