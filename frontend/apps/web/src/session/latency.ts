// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

/**
 * Measures the session's glass-to-glass latency with the beacon: the
 * viewer flips a square in the top-left corner of the host's picture
 * (on the host's screen, or drawn by its test card) and times how long the
 * change takes to show in the video here. That covers the whole path:
 * capture, encoder, network, decoder and display. The control message's
 * own trip there (half the round trip) is taken out.
 */
export interface BeaconTarget {
  flipBeacon(seq: number): void;
}

/** The corner the beacon covers, in the video's pixels (it is 16 or more). */
const SAMPLE = { x: 2, y: 2, size: 6 };

/** Average luma of the beacon's corner in the current video frame (0-255), or null. */
export function beaconLuma(video: HTMLVideoElement, ctx: CanvasRenderingContext2D): number | null {
  if (video.videoWidth < 16 || video.videoHeight < 16) return null;
  ctx.drawImage(video, SAMPLE.x, SAMPLE.y, SAMPLE.size, SAMPLE.size, 0, 0, SAMPLE.size, SAMPLE.size);
  const d = ctx.getImageData(0, 0, SAMPLE.size, SAMPLE.size).data;
  let sum = 0;
  for (let i = 0; i < d.length; i += 4) sum += 0.2126 * d[i]! + 0.7152 * d[i + 1]! + 0.0722 * d[i + 2]!;
  return sum / (d.length / 4);
}

type FrameVideo = HTMLVideoElement & {
  requestVideoFrameCallback?: (cb: (now: number) => void) => number;
};

/** Waits for the next shown frame (requestVideoFrameCallback, else the next paint). */
function nextFrame(video: FrameVideo): Promise<number> {
  return new Promise((resolve) => {
    if (video.requestVideoFrameCallback) video.requestVideoFrameCallback((now) => resolve(now));
    else requestAnimationFrame((now) => resolve(now));
  });
}

/** Waits until the corner turns white (on) or black, or the time runs out. */
async function waitForBeacon(video: FrameVideo, ctx: CanvasRenderingContext2D, on: boolean, timeoutMs: number): Promise<number | null> {
  const end = performance.now() + timeoutMs;
  while (performance.now() < end) {
    const at = await Promise.race([nextFrame(video), new Promise<number>((r) => setTimeout(() => r(-1), 250))]);
    const luma = beaconLuma(video, ctx);
    if (luma !== null && (on ? luma > 180 : luma < 60)) return at > 0 ? at : performance.now();
  }
  return null;
}

/** The median of the samples (null when none). */
export function median(xs: number[]): number | null {
  if (xs.length === 0) return null;
  const s = [...xs].sort((a, b) => a - b);
  const m = Math.floor(s.length / 2);
  return s.length % 2 ? s[m]! : (s[m - 1]! + s[m]!) / 2;
}

/**
 * Runs the measurement: samples flips, the beacon left black at the end.
 * Returns the median latency in ms, or null when the beacon never showed
 * (a host without one).
 */
export async function measureLatency(video: HTMLVideoElement, target: BeaconTarget, rttMs: number, samples = 7): Promise<number | null> {
  const canvas = document.createElement("canvas");
  canvas.width = canvas.height = SAMPLE.size;
  const ctx = canvas.getContext("2d", { willReadFrequently: true });
  if (!ctx) return null;
  const results: number[] = [];
  let seq = 0;
  // Start from black, so the first flip to white is a real change.
  target.flipBeacon(seq);
  await waitForBeacon(video, ctx, false, 1500);
  for (let i = 0; i < samples * 2; i++) {
    seq++;
    const on = seq % 2 === 1;
    const t0 = performance.now();
    target.flipBeacon(seq);
    const t1 = await waitForBeacon(video, ctx, on, 2000);
    if (t1 === null) {
      if (results.length === 0) return null;
      continue;
    }
    results.push(Math.max(0, t1 - t0 - rttMs / 2));
    if (results.length >= samples) break;
    await new Promise((r) => setTimeout(r, 150));
  }
  if (seq % 2 === 1) target.flipBeacon(seq + 1);
  const m = median(results);
  return m === null ? null : Math.round(m);
}
