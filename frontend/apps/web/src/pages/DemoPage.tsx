// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { t } from "../i18n";

/** demo.json, written by the demonstration host (--test-code-file). */
export type DemoCode = { online: boolean; code?: string; link?: string; name?: string; pairing?: string; password?: string; updated?: string };

/** How often the page reads the code again: it changes when the host reconnects. */
const REFRESH_MS = 15_000;

/** The /c/<invite> part of the host's link, kept on this site. */
export function demoPath(link: string | undefined): string | undefined {
  const m = /\/c\/([A-Za-z0-9_-]{22})$/.exec(link ?? "");
  return m ? `/c/${m[1]}` : undefined;
}

/**
 * desk.go-link.org/demo: the code of a computer that is always on and
 * shows a test card, to try go-link Desk (and for app reviewers). The
 * code changes when that computer reconnects; this page always has the
 * current one.
 */
export function DemoPage({ load = loadDemo }: { load?: () => Promise<DemoCode> }) {
  const [demo, setDemo] = useState<DemoCode | null | "error">(null);
  useEffect(() => {
    let stop = false;
    const read = () =>
      load()
        .then((d) => !stop && setDemo(d))
        .catch(() => !stop && setDemo("error"));
    read();
    const id = window.setInterval(read, REFRESH_MS);
    return () => {
      stop = true;
      window.clearInterval(id);
    };
  }, [load]);
  const live = demo && demo !== "error" && demo.online && demo.code ? demo : null;
  const path = live ? demoPath(live.link) : undefined;
  return (
    <section className="demo">
      <div className="demo-main">
        <h1 className="demo-title">{t.demo.title}</h1>
        <p className="demo-intro">{t.demo.intro}</p>
        <div className="demo-card demo-now" aria-live="polite">
          {demo === null && <p className="muted">{t.demo.loading}</p>}
          {demo !== null && !live && <p className="notice">{t.demo.offline}</p>}
          {live && (
            <>
              <p className="muted">{t.demo.codeOf.replace("{name}", live.name || t.demo.title)}</p>
              <p className="demo-code mono" aria-label={t.demo.codeLabel}>
                {live.code}
              </p>
              {path && (
                <Link to={path} className="button button-primary demo-open">
                  {t.demo.open}
                </Link>
              )}
            </>
          )}
        </div>
        <p className="muted demo-note">{t.demo.note}</p>
      </div>
      <div className="demo-side">
        <div className="demo-card">
          <h2>{t.demo.howTitle}</h2>
          <ol>
            {t.demo.how.map((s) => (
              <li key={s}>{s}</li>
            ))}
          </ol>
        </div>
        {live?.pairing && live.password && (
          <div className="demo-card">
            <h2>{t.demo.linkTitle}</h2>
            <p>{t.demo.linkIntro}</p>
            <dl className="demo-link">
              <dt>{t.demo.pairingLabel}</dt>
              <dd className="mono">{live.pairing}</dd>
              <dt>{t.demo.passwordLabel}</dt>
              <dd className="mono">{live.password}</dd>
            </dl>
            <ol>
              {t.demo.linkHow.map((s) => (
                <li key={s}>{s}</li>
              ))}
            </ol>
          </div>
        )}
      </div>
    </section>
  );
}

/** Reads demo.json, the demo computer's current codes. */
export async function loadDemo(): Promise<DemoCode> {
  const r = await fetch("/demo.json", { cache: "no-store" });
  if (!r.ok) throw new Error(String(r.status));
  return (await r.json()) as DemoCode;
}
