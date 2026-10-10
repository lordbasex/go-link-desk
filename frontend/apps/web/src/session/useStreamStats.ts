// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useEffect, useState } from "react";
import type { HostStream, StreamStats } from "@go-link-desk/shared";

/**
 * The browser's view of the stream, read every second. Every other read
 * also goes to the host as client_report, which moves how long the
 * browser may buffer the video (smoother on a bad network, no delay on a
 * good one).
 */
export function useStreamStats(stream: HostStream | null, live: boolean): StreamStats | null {
  const [stats, setStats] = useState<StreamStats | null>(null);
  useEffect(() => {
    if (!stream || !live) return;
    let alive = true;
    let n = 0;
    const read = async () => {
      const s = await stream.stats().catch(() => null);
      if (!alive || !s) return;
      setStats(s);
      if (++n % 2 === 0 && s.window) {
        stream.sendControl({
          type: "client_report",
          metrics: {
            freeze_ms: s.window.freezeMs,
            video_loss_pct: s.window.videoLossPct,
            jitter_ms: s.window.jitterMs,
            hidden: document.visibilityState === "hidden" ? 1 : 0,
          },
        });
      }
    };
    void read();
    const timer = setInterval(() => void read(), 1000);
    return () => {
      alive = false;
      clearInterval(timer);
    };
  }, [stream, live]);
  return stats;
}
