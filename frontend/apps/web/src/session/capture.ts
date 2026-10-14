// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
// Screenshots and recordings of the computer's screen, made here in the
// browser from the picture it already gets: nothing more is sent, and the
// host only learns that it happens (session.askScreenshot / startRecording),
// so the person there sees it.

import type { DownloadSink } from "@go-link-desk/shared";

/** A recording stops by itself at whichever comes first. */
export const MAX_RECORDING_MS = 2 * 60 * 60 * 1000;
/** The most a recording kept in memory (no file picker) may grow to. */
export const MAX_RECORDING_MEMORY = 1 << 30;
/** The recording's bit rate: screen text stays sharp. */
const RECORDING_BPS = 6_000_000;

/** A file name: "go-link Desk - Office Mac - 2026-10-14 15.04.05.png". */
export function captureName(host: string, when: Date, ext: string): string {
  const clean = host
    .replace(/[\p{Cc}\p{Cf}/\\:*?"<>|]+/gu, " ")
    .replace(/\s+/g, " ")
    .trim()
    .slice(0, 40)
    .trim();
  const p = (n: number) => String(n).padStart(2, "0");
  const stamp = `${when.getFullYear()}-${p(when.getMonth() + 1)}-${p(when.getDate())} ${p(when.getHours())}.${p(when.getMinutes())}.${p(when.getSeconds())}`;
  return `go-link Desk - ${clean ? `${clean} - ` : ""}${stamp}.${ext}`;
}

/** The first recording format this browser makes: WebM (Chromium, Firefox), else MP4 (Safari). */
export function recordingType(supported: (type: string) => boolean = (t) => typeof MediaRecorder !== "undefined" && MediaRecorder.isTypeSupported(t)): { mime: string; ext: string } | null {
  const candidates: [string, string][] = [
    ["video/webm;codecs=vp9,opus", "webm"],
    ["video/webm;codecs=vp8,opus", "webm"],
    ["video/webm", "webm"],
    ["video/mp4;codecs=avc1,mp4a.40.2", "mp4"],
    ["video/mp4", "mp4"],
  ];
  for (const [mime, ext] of candidates) {
    try {
      if (supported(mime)) return { mime, ext };
    } catch {
      // a browser that throws for a type it does not know
    }
  }
  return null;
}

/** The host's pointer over a screenshot, in the picture's pixels. */
export interface CursorDraw {
  url: string;
  x: number;
  y: number;
  width: number;
  height: number;
}

/** A PNG of what the video shows now, with the pointer drawn over it; null without a picture. */
export async function screenshot(video: HTMLVideoElement, cursor: CursorDraw | null): Promise<Blob | null> {
  const w = video.videoWidth;
  const h = video.videoHeight;
  if (!w || !h) return null;
  const canvas = document.createElement("canvas");
  canvas.width = w;
  canvas.height = h;
  const ctx = canvas.getContext("2d");
  if (!ctx) return null;
  ctx.drawImage(video, 0, 0, w, h);
  if (cursor) {
    try {
      const img = new Image();
      img.src = cursor.url;
      await img.decode();
      ctx.drawImage(img, cursor.x, cursor.y, cursor.width, cursor.height);
    } catch {
      // the picture without the pointer
    }
  }
  return new Promise((resolve) => canvas.toBlob((b) => resolve(b), "image/png"));
}

/** Why a recording stopped. */
export type StopReason = "stopped" | "time" | "size" | "host" | "ended" | "failed";

/**
 * Records a stream (the screen, and the computer's sound when this viewer
 * hears it) into a sink, a piece every second, so a long recording that
 * goes to a file never sits in memory.
 */
export class ScreenRecorder {
  private rec: MediaRecorder;
  private written = 0;
  private writing: Promise<void> = Promise.resolve();
  private reason: StopReason | null = null;
  private failed = false;
  private finished = false;
  private timer: ReturnType<typeof setTimeout>;
  readonly started = Date.now();

  constructor(
    stream: MediaStream,
    mime: string,
    private readonly sink: DownloadSink & { inMemory: boolean },
    private readonly onStop: (reason: StopReason) => void,
    Recorder: typeof MediaRecorder = MediaRecorder,
  ) {
    this.rec = new Recorder(stream, { mimeType: mime, videoBitsPerSecond: RECORDING_BPS, audioBitsPerSecond: 128_000 });
    this.rec.ondataavailable = (e) => {
      if (!e.data.size || this.failed) return;
      this.written += e.data.size;
      const data = e.data;
      this.writing = this.writing.then(async () => {
        if (this.failed) return;
        try {
          await this.sink.write(new Uint8Array(await data.arrayBuffer()));
        } catch {
          this.failed = true;
          this.stop("failed");
        }
      });
      if (this.sink.inMemory && this.written >= MAX_RECORDING_MEMORY) this.stop("size");
    };
    this.rec.onerror = () => this.stop("failed");
    this.rec.onstop = () => {
      this.reason ??= "ended"; // the stream ended by itself
      void this.finish();
    };
    this.timer = setTimeout(() => this.stop("time"), MAX_RECORDING_MS);
    this.rec.start(1000);
  }

  /** Bytes recorded so far. */
  get size(): number {
    return this.written;
  }

  /** Stops (the first reason counts); the file is saved, then onStop is told. */
  stop(reason: StopReason): void {
    if (this.reason) return;
    this.reason = reason;
    clearTimeout(this.timer);
    if (this.rec.state === "inactive") void this.finish();
    else this.rec.stop();
  }

  private async finish(): Promise<void> {
    if (this.finished) return;
    this.finished = true;
    clearTimeout(this.timer);
    await this.writing;
    try {
      if (this.failed || !this.written) await this.sink.abort();
      else await this.sink.close();
    } catch {
      this.failed = true;
    }
    this.onStop(this.failed ? "failed" : (this.reason ?? "stopped"));
  }
}
