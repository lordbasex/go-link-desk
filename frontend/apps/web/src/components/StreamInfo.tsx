// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useEffect, useRef, useState } from "react";
import type { StreamStats } from "@go-link-desk/shared";
import type { HostSent } from "../session/session";
import { t } from "../i18n";
import { InfoIcon } from "./Icons";

/**
 * The session's figures behind a small (i) button that never changes
 * size: the numbers move every second, so they open on demand instead of
 * resizing a chip over the screen. The ring is orange on the TURN relay.
 */
export function StreamInfo({ stats, sent }: { stats: StreamStats | null; sent: HostSent | null }) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const close = (e: MouseEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    };
    const esc = (e: KeyboardEvent) => (e.key === "Escape" || e.code === "Escape") && setOpen(false);
    document.addEventListener("mousedown", close);
    window.addEventListener("keydown", esc);
    return () => {
      document.removeEventListener("mousedown", close);
      window.removeEventListener("keydown", esc);
    };
  }, [open]);
  const path = stats?.path ?? null;
  const dash = "–";
  const rtt = stats?.rttMs ?? null;
  const rows: [string, string][] = [
    [t.stats.latency, rtt === null ? dash : rtt < 1 ? "< 1 ms" : `${rtt} ms`],
    [t.stats.path, path === "relay" ? t.stats.relay : path === "direct" ? t.stats.direct : dash],
    [t.stats.sent, sent ? `${sent.fps.toFixed(1)} fps` : dash],
    [t.stats.received, stats?.fps != null ? `${Math.round(stats.fps)} fps` : dash],
    [t.stats.decoded, stats?.framesDecoded != null ? String(stats.framesDecoded) : dash],
    [t.stats.size, stats?.width && stats.height ? `${stats.width}×${stats.height}` : dash],
    [t.stats.bitrate, stats?.window ? `${stats.window.kbps} kbps` : dash],
    [t.stats.loss, stats?.window ? `${stats.window.videoLossPct} %` : dash],
    [t.stats.codec, stats?.codec ?? sent?.codec ?? dash],
  ];
  return (
    <div className={`stream-info${path === "relay" ? " is-relay" : ""}`} ref={rootRef}>
      <button
        type="button"
        className="icon-button"
        aria-expanded={open}
        aria-label={t.stats.button}
        title={t.stats.button}
        onClick={() => setOpen(!open)}
      >
        <InfoIcon />
      </button>
      {open && (
        <div className="stream-info-panel" role="dialog" aria-label={t.stats.button}>
          <dl>
            {rows.map(([k, v]) => (
              <div key={k} className="stream-info-row">
                <dt>{k}</dt>
                <dd className="mono">{v}</dd>
              </div>
            ))}
          </dl>
          {path === "relay" && <p className="small muted">{t.stats.relayHint}</p>}
        </div>
      )}
    </div>
  );
}
