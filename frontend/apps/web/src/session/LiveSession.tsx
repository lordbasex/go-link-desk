// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useCallback, useEffect, useRef, useState } from "react";
import { t } from "../i18n";
import { ExpandIcon, HangUpIcon } from "../components/Icons";
import { StreamInfo } from "../components/StreamInfo";
import { Select } from "../components/ui/Select";
import { useFullscreen } from "../components/useFullscreen";
import { measureLatency } from "./latency";
import type { CursorState, Preset, Session, SessionView } from "./session";
import { useStreamStats } from "./useStreamStats";

/** How the picture fits the window: whole and centred, one pixel for one pixel, or filling it. */
export type ViewMode = "fit" | "original" | "stretch";

const PREFS_KEY = "go-link-desk.view";

interface Prefs {
  preset: Preset;
  retina: boolean;
  view: ViewMode;
  cursorInVideo: boolean;
}

const DEFAULT_PREFS: Prefs = { preset: "balanced", retina: false, view: "fit", cursorInVideo: false };

function loadPrefs(): Prefs {
  try {
    const p = JSON.parse(window.localStorage.getItem(PREFS_KEY) ?? "{}") as Partial<Prefs>;
    return {
      preset: p.preset === "speed" || p.preset === "quality" ? p.preset : "balanced",
      retina: p.retina === true,
      view: p.view === "original" || p.view === "stretch" ? p.view : "fit",
      cursorInVideo: p.cursorInVideo === true,
    };
  } catch {
    return DEFAULT_PREFS;
  }
}

function savePrefs(p: Prefs): void {
  try {
    window.localStorage.setItem(PREFS_KEY, JSON.stringify(p));
  } catch {
    // storage blocked: the choice lasts for this visit
  }
}

/** Where the picture is drawn inside the video element (CSS pixels), for a view mode. */
export function contentRect(boxW: number, boxH: number, videoW: number, videoH: number, mode: ViewMode): { x: number; y: number; w: number; h: number } {
  if (mode === "stretch" || mode === "original" || videoW <= 0 || videoH <= 0) return { x: 0, y: 0, w: boxW, h: boxH };
  const scale = Math.min(boxW / videoW, boxH / videoH);
  const w = videoW * scale;
  const h = videoH * scale;
  return { x: (boxW - w) / 2, y: (boxH - h) / 2, w, h };
}

export function LiveSession({ view, session, registerCursor }: { view: SessionView; session: Session | null; registerCursor: (fn: ((c: CursorState) => void) | null) => void }) {
  const stageRef = useRef<HTMLDivElement>(null);
  const screenRef = useRef<HTMLDivElement>(null);
  const videoRef = useRef<HTMLVideoElement>(null);
  const cursorRef = useRef<HTMLImageElement>(null);
  const lastCursor = useRef<CursorState | null>(null);
  const fullscreen = useFullscreen(stageRef);
  const [prefs, setPrefs] = useState<Prefs>(loadPrefs);
  const [latency, setLatency] = useState<number | null | "measuring" | "none">(null);
  const [videoSize, setVideoSize] = useState<[number, number]>([0, 0]);
  const live = view.status.kind === "live" || view.status.kind === "reconnecting";
  const stats = useStreamStats(session?.hostStream ?? null, live);
  const display = view.displays.find((d) => d.id === view.display) ?? null;

  const update = (p: Partial<Prefs>) => {
    const next = { ...prefs, ...p };
    setPrefs(next);
    savePrefs(next);
    if (p.preset !== undefined || p.retina !== undefined) session?.setQuality(next.preset, next.retina);
    if (p.cursorInVideo !== undefined) session?.setCursorInVideo(next.cursorInVideo);
  };

  // The host learns the viewer's choices as soon as the session is live.
  useEffect(() => {
    if (!live || !session) return;
    session.setQuality(prefs.preset, prefs.retina);
    if (prefs.cursorInVideo) session.setCursorInVideo(true);
    // Only once per session: later changes go through update().
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [live, session]);

  useEffect(() => {
    const v = videoRef.current;
    if (!v || v.srcObject === view.video) return;
    v.srcObject = view.video;
    // Muted video autoplays everywhere; sound arrives in a later version.
    void Promise.resolve(v.play()).catch(() => undefined);
  }, [view.video]);

  // The host sends the picture at the size it is shown here (device
  // pixels), so nothing is sent that the screen would throw away.
  useEffect(() => {
    const el = screenRef.current;
    if (!el || !session || !live || typeof ResizeObserver === "undefined") return;
    let timer: ReturnType<typeof setTimeout> | null = null;
    const send = () => {
      const dpr = window.devicePixelRatio || 1;
      session.setVideoWant(el.clientWidth * dpr, el.clientHeight * dpr);
    };
    const ro = new ResizeObserver(() => {
      if (timer) clearTimeout(timer);
      timer = setTimeout(send, 300);
    });
    ro.observe(el);
    send();
    return () => {
      ro.disconnect();
      if (timer) clearTimeout(timer);
    };
  }, [session, live]);

  // The host's pointer, drawn here at its place over the picture.
  const placeCursor = useCallback(
    (c: CursorState | null) => {
      const img = cursorRef.current;
      const video = videoRef.current;
      if (!img || !video) return;
      if (!c || !c.visible || !c.shape || prefs.cursorInVideo || !display || display.pointsWidth <= 0) {
        img.style.display = "none";
        return;
      }
      const r = contentRect(video.clientWidth, video.clientHeight, video.videoWidth, video.videoHeight, prefs.view);
      const scale = r.w / display.pointsWidth; // CSS pixels per host point
      const perPixel = c.shape.pixels > 0 ? c.shape.width / c.shape.pixels : 1; // points per image pixel
      const w = c.shape.width * scale;
      const h = c.shape.height * scale;
      img.style.display = "block";
      if (img.src !== c.shape.url) img.src = c.shape.url;
      img.style.width = `${w}px`;
      img.style.height = `${h}px`;
      img.style.transform = `translate(${video.offsetLeft + r.x + c.x * r.w - c.shape.hotX * perPixel * scale}px, ${video.offsetTop + r.y + c.y * r.h - c.shape.hotY * perPixel * scale}px)`;
    },
    [prefs.cursorInVideo, prefs.view, display],
  );

  useEffect(() => {
    registerCursor((c) => {
      lastCursor.current = c;
      placeCursor(c);
    });
    placeCursor(lastCursor.current);
    return () => registerCursor(null);
  }, [registerCursor, placeCursor]);

  const measure = async () => {
    const v = videoRef.current;
    if (!v || !session) return;
    setLatency("measuring");
    const ms = await measureLatency(v, session, stats?.rttMs ?? 0);
    setLatency(ms === null ? "none" : ms);
  };

  const hostName = view.host?.name || t.session.title;
  const notice = view.status.kind === "connecting" ? t.session.connecting : view.status.kind === "reconnecting" ? t.session.reconnecting : null;
  const hostNotice = view.notice === "no_screen_permission" ? t.session.noScreenPermission : view.notice === "capture_failed" ? t.session.captureFailed : null;
  return (
    <div className={`stage${fullscreen.pseudo ? " is-pseudo-fullscreen" : ""}`} ref={stageRef}>
      <div className="stage-bar">
        <span className="stage-title">{t.session.screenOf.replace("{name}", hostName)}</span>
        <div className="stage-tools">
          {view.displays.length > 1 && (
            <Select<string>
              className="select-sm"
              ariaLabel={t.session.display}
              value={String(view.display)}
              options={view.displays.map((d, i) => ({ value: String(d.id), label: d.name || t.session.displayN.replace("{n}", String(i + 1)), detail: `${d.width}×${d.height}` }))}
              onChange={(v) => session?.selectDisplay(Number(v))}
            />
          )}
          <Select<Preset>
            className="select-sm"
            ariaLabel={t.session.quality}
            value={prefs.preset}
            options={[
              { value: "quality", label: t.session.presets.quality },
              { value: "balanced", label: t.session.presets.balanced },
              { value: "speed", label: t.session.presets.speed },
            ]}
            onChange={(preset) => update({ preset })}
          />
          <Select<ViewMode>
            className="select-sm"
            ariaLabel={t.session.view}
            value={prefs.view}
            options={[
              { value: "fit", label: t.session.views.fit },
              { value: "original", label: t.session.views.original },
              { value: "stretch", label: t.session.views.stretch },
            ]}
            onChange={(v) => update({ view: v })}
          />
          <label className="stage-check">
            <input type="checkbox" checked={prefs.retina} onChange={(e) => update({ retina: e.target.checked })} />
            {t.session.retina}
          </label>
          <label className="stage-check">
            <input type="checkbox" checked={prefs.cursorInVideo} onChange={(e) => update({ cursorInVideo: e.target.checked })} />
            {t.session.cursorInVideo}
          </label>
          <StreamInfo stats={stats} sent={view.sent} latency={latency} onMeasure={() => void measure()} />
          <button type="button" className="icon-button" aria-label={t.session.fullscreen} title={t.session.fullscreen} onClick={fullscreen.toggle}>
            <ExpandIcon />
          </button>
          <button type="button" className="button button-danger" onClick={() => session?.hangUp()}>
            <HangUpIcon />
            {t.session.disconnect}
          </button>
        </div>
      </div>
      {hostNotice && (
        <p className="stage-banner" role="status">
          {hostNotice}
        </p>
      )}
      <div className={`stage-screen view-${prefs.view}`} ref={screenRef}>
        <video
          ref={videoRef}
          className="screen"
          autoPlay
          muted
          playsInline
          aria-label={t.session.screenOf.replace("{name}", hostName)}
          onResize={(e) => {
            setVideoSize([e.currentTarget.videoWidth, e.currentTarget.videoHeight]);
            placeCursor(lastCursor.current);
          }}
          style={
            prefs.view === "original" && videoSize[0] > 0
              ? { width: `${videoSize[0] / (window.devicePixelRatio || 1)}px`, height: `${videoSize[1] / (window.devicePixelRatio || 1)}px` }
              : undefined
          }
        />
        <img ref={cursorRef} className="remote-cursor" alt="" draggable={false} />
        {notice && (
          <p className="stage-notice" role="status">
            {notice}
          </p>
        )}
      </div>
    </div>
  );
}
