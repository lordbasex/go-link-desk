// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { Suspense, lazy, useCallback, useEffect, useRef, useState } from "react";
import { MAX_THUMB } from "@go-link-desk/shared";
import { t } from "../i18n";
import { CameraIcon, ChatIcon, ChevronIcon, CloseIcon, CommandIcon, ExpandIcon, FolderIcon, MonitorIcon, RecordIcon, ShrinkIcon, SoundIcon, TerminalIcon } from "../components/Icons";
import { BarMenu, MenuItem, MenuSection, MenuSeparator } from "../components/BarMenu";
import { StreamHud, StreamInfo } from "../components/StreamInfo";
import { useFullscreen } from "../components/useFullscreen";
import { CallPanel } from "./CallPanel";
import { ScreenRecorder, captureName, recordingType, screenshot, type CursorDraw, type StopReason } from "./capture";
import { ChatPanel } from "./ChatPanel";
import { playChatSound } from "./chatSound";
import { FilesPanel } from "./FilesPanel";
import { HostClipboard } from "./hostClipboard";
import { InputCapture } from "./input";
import { measureLatency } from "./latency";
import { downloadBlob, sinkFor } from "./save";
import type { CursorState, Preset, Session, SessionView, TerminalTab } from "./session";
import { useStreamStats } from "./useStreamStats";
import type { TerminalApi } from "./TerminalStage";

// xterm.js comes only with a computer that offers terminals.
const TerminalStage = lazy(() => import("./TerminalStage"));

const TERM_FONT_KEY = "go-link-desk.terminal-font";
const TERM_FONTS = [11, 12, 13, 14, 16, 18, 20, 24];

function loadTermFont(): number {
  try {
    const n = Number(window.localStorage.getItem(TERM_FONT_KEY));
    return TERM_FONTS.includes(n) ? n : 14;
  } catch {
    return 14;
  }
}

/** Special keys a terminal takes, as the bytes they send. */
const TERMINAL_KEYS: [string, string][] = [
  ["Esc", "\x1b"],
  ["Tab", "\t"],
  ["Ctrl+C", "\x03"],
  ["Ctrl+D", "\x04"],
  ["Ctrl+Z", "\x1a"],
  ["Ctrl+L", "\x0c"],
  ["↑", "\x1b[A"],
  ["↓", "\x1b[B"],
];

/** How the picture fits the window: whole and centred, one pixel for one pixel, or filling it. */
export type ViewMode = "fit" | "original" | "stretch";

const PREFS_KEY = "go-link-desk.view";

interface Prefs {
  preset: Preset;
  retina: boolean;
  view: ViewMode;
  cursorInVideo: boolean;
  ctrlIsCmd: boolean;
  muted: boolean;
  hud: boolean;
}

const DEFAULT_PREFS: Prefs = { preset: "balanced", retina: false, view: "fit", cursorInVideo: false, ctrlIsCmd: false, muted: false, hud: false };

function loadPrefs(): Prefs {
  try {
    const p = JSON.parse(window.localStorage.getItem(PREFS_KEY) ?? "{}") as Partial<Prefs>;
    return {
      preset: p.preset === "speed" || p.preset === "quality" ? p.preset : "balanced",
      retina: p.retina === true,
      view: p.view === "original" || p.view === "stretch" ? p.view : "fit",
      cursorInVideo: p.cursorInVideo === true,
      ctrlIsCmd: p.ctrlIsCmd === true,
      muted: p.muted === true,
      hud: p.hud === true,
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

/** A small JPEG of what the video shows (for the list of computers), or null. */
export function snapshot(video: HTMLVideoElement, width = 240): string | null {
  if (!video.videoWidth || !video.videoHeight) return null;
  try {
    const canvas = document.createElement("canvas");
    canvas.width = width;
    canvas.height = Math.round((video.videoHeight / video.videoWidth) * width);
    const ctx = canvas.getContext("2d");
    if (!ctx) return null;
    ctx.drawImage(video, 0, 0, canvas.width, canvas.height);
    const url = canvas.toDataURL("image/jpeg", 0.6);
    return url.startsWith("data:image/jpeg;base64,") && url.length <= MAX_THUMB ? url : null;
  } catch {
    return null;
  }
}

export function LiveSession({
  view,
  session,
  registerCursor,
  onSnapshot,
}: {
  view: SessionView;
  session: Session | null;
  registerCursor: (fn: ((c: CursorState) => void) | null) => void;
  /** Told a thumbnail of the screen now and then while the session is live. */
  onSnapshot?: (url: string) => void;
}) {
  const stageRef = useRef<HTMLDivElement>(null);
  const screenRef = useRef<HTMLDivElement>(null);
  const videoRef = useRef<HTMLVideoElement>(null);
  const snapRef = useRef(onSnapshot);
  snapRef.current = onSnapshot;
  const showing = view.status.kind === "live";
  useEffect(() => {
    if (!showing) return;
    const take = () => {
      const url = videoRef.current && snapshot(videoRef.current);
      if (url) snapRef.current?.(url);
    };
    const first = setTimeout(take, 2000);
    const every = setInterval(take, 15000);
    return () => {
      clearTimeout(first);
      clearInterval(every);
    };
  }, [showing]);
  const cursorRef = useRef<HTMLImageElement>(null);
  const lastCursor = useRef<CursorState | null>(null);
  const fullscreen = useFullscreen(stageRef);
  const [prefs, setPrefs] = useState<Prefs>(loadPrefs);
  const [latency, setLatency] = useState<number | null | "measuring" | "none">(null);
  const [videoSize, setVideoSize] = useState<[number, number]>([0, 0]);
  const live = view.status.kind === "live" || view.status.kind === "reconnecting";
  const stats = useStreamStats(session?.hostStream ?? null, live);
  const display = view.displays.find((d) => d.id === view.display) ?? null;
  const control = view.permissions?.control === true;
  // go-link-desk-terminal: terminals where the screen goes, in the same session.
  const terminal = view.host?.terminal === true;
  const termApi = useRef<TerminalApi | null>(null);
  const tabs = view.host?.terminalTabs ? view.terminalTabs : terminal ? [{ id: 0, name: view.host?.name || "shell", started: 0 }] : [];
  const [activeTab, setActiveTab] = useState(0);
  const [activity, setActivity] = useState<number[]>([]);
  const [termFont, setTermFont] = useState(loadTermFont);
  const [renaming, setRenaming] = useState<number | null>(null);
  const opening = useRef(false);
  const knownTabs = useRef<number[]>([]);
  const shownTab = tabs.find((x) => x.id === activeTab) ? activeTab : (tabs[0]?.id ?? 0);
  const prefsRef = useRef(prefs);
  prefsRef.current = prefs;
  const displaysRef = useRef(view.displays);
  displaysRef.current = view.displays;
  const displayRef = useRef(view.display);
  displayRef.current = view.display;
  const keysRef = useRef<HTMLTextAreaElement>(null);
  const sharesClipboardRef = useRef(false);
  sharesClipboardRef.current = view.permissions?.clipboard === true;
  const captureRef = useRef<InputCapture | null>(null);
  const [locked, setLocked] = useState(false);
  const [typing, setTyping] = useState(false);
  // A finger drives the pointer (touchpad mode): the host's pointer is drawn, and a Keyboard button opens the screen keyboard.
  const [touch, setTouch] = useState(false);
  const hostIsMac = view.host?.os === "darwin";
  const hears = view.permissions?.audio === true;
  const sharesClipboard = view.permissions?.clipboard === true;
  const audioRef = useRef<HTMLAudioElement>(null);
  // The browser refused to start the sound without a click (autoplay).
  const [soundBlocked, setSoundBlocked] = useState(false);
  const [chatOpen, setChatOpen] = useState(false);
  const [callOpen, setCallOpen] = useState(false);
  const closeCall = useCallback(() => setCallOpen(false), []);
  const [filesOpen, setFilesOpen] = useState(false);
  const [dropped, setDropped] = useState<File[]>([]);
  const [dragging, setDragging] = useState(false);
  const mayFiles = view.permissions?.files === true;
  const mayRecord = view.permissions?.record === true;
  const recRef = useRef<ScreenRecorder | null>(null);
  const [recSince, setRecSince] = useState<number | null>(null);
  const [, setTick] = useState(0);
  const [captureNote, setCaptureNote] = useState<string | null>(null);
  const noteTimer = useRef(0);
  // While recording, the host draws its pointer into the picture, so the recording has it.
  const cursorInVideo = prefs.cursorInVideo || recSince !== null;
  const [readChat, setReadChat] = useState(0);
  const [clipboardWaiting, setClipboardWaiting] = useState(false);
  const hostClipboard = useRef<HostClipboard | null>(null);
  if (!hostClipboard.current) hostClipboard.current = new HostClipboard(setClipboardWaiting);
  const unread = chatOpen ? 0 : view.chat.slice(readChat).filter((l) => l.fromHost).length;
  // A message from the host plays the chat's sound (unless it is off).
  const heard = useRef(view.chat.length);
  useEffect(() => {
    const fresh = view.chat.slice(heard.current);
    heard.current = view.chat.length;
    if (fresh.some((l) => l.fromHost)) playChatSound();
  }, [view.chat]);

  const update = (p: Partial<Prefs>) => {
    const next = { ...prefs, ...p };
    setPrefs(next);
    savePrefs(next);
    if (p.preset !== undefined || p.retina !== undefined) session?.setQuality(next.preset, next.retina);
    if (p.cursorInVideo !== undefined) session?.setCursorInVideo(next.cursorInVideo || recRef.current !== null);
    if (p.ctrlIsCmd !== undefined) captureRef.current?.setOptions({ ctrlIsCmd: next.ctrlIsCmd, pasteToHost: sharesClipboard });
  };

  // The mouse and keyboard, while the host lets this viewer control.
  useEffect(() => {
    const surface = screenRef.current;
    const keys = keysRef.current;
    if (!control || !live || !session || !surface || !keys || terminal) return;
    const cap = new InputCapture(
      surface,
      keys,
      {
        rect: () => {
          const v = videoRef.current;
          if (!v) return null;
          const b = v.getBoundingClientRect();
          const r = contentRect(b.width, b.height, v.videoWidth, v.videoHeight, prefsRef.current.view);
          return { left: b.left + r.x, top: b.top + r.y, width: r.w, height: r.h };
        },
        display: () => Math.max(0, displaysRef.current.findIndex((d) => d.id === displayRef.current)),
        pointsPerPixel: () => {
          const v = videoRef.current;
          const d = displaysRef.current.find((x) => x.id === displayRef.current);
          if (!v || !d || d.pointsWidth <= 0) return 1;
          const b = v.getBoundingClientRect();
          const r = contentRect(b.width, b.height, v.videoWidth, v.videoHeight, prefsRef.current.view);
          return r.w > 0 ? d.pointsWidth / r.w : 1;
        },
      },
      session,
      { ctrlIsCmd: prefsRef.current.ctrlIsCmd, pasteToHost: sharesClipboardRef.current },
    );
    cap.onTouchMode = setTouch;
    cap.attach();
    captureRef.current = cap;
    keys.focus({ preventScroll: true });
    const onLock = () => setLocked(document.pointerLockElement === surface);
    document.addEventListener("pointerlockchange", onLock);
    return () => {
      document.removeEventListener("pointerlockchange", onLock);
      cap.detach();
      captureRef.current = null;
      if (document.pointerLockElement === surface) document.exitPointerLock();
    };
  }, [control, live, session, terminal]);

  // Full screen takes the system's shortcuts too (Keyboard Lock, Chromium):
  // Cmd+Tab, Alt+Tab and Esc go to the host; holding Esc leaves.
  useEffect(() => {
    type KL = { keyboard?: { lock?: () => Promise<void>; unlock?: () => void } };
    const kb = (navigator as unknown as KL).keyboard;
    if (!control || !fullscreen.active || fullscreen.pseudo || !kb?.lock) return;
    void kb.lock().catch(() => undefined);
    return () => kb.unlock?.();
  }, [control, fullscreen.active, fullscreen.pseudo]);

  const toggleLock = () => {
    const surface = screenRef.current as (HTMLElement & { requestPointerLock(o?: { unadjustedMovement?: boolean }): Promise<void> | void }) | null;
    if (!surface) return;
    if (document.pointerLockElement === surface) {
      document.exitPointerLock();
      return;
    }
    // unadjustedMovement: the raw mouse, without this system's acceleration on top of the host's.
    void Promise.resolve(surface.requestPointerLock({ unadjustedMovement: true })).catch(() => surface.requestPointerLock());
  };

  // The floating toolbar, as the iPhone app's: hidden by hand with its
  // chevron; in full screen hidden until the pointer comes to the top
  // (or a finger taps the handle), and kept while one of its menus is open.
  const [barHidden, setBarHidden] = useState(false);
  const [peek, setPeek] = useState(false);
  const [menus, setMenus] = useState(0);
  const peekTimer = useRef(0);
  const fsBar = fullscreen.active;
  const showBar = fsBar ? peek || menus > 0 : !barHidden;
  const keepPeek = () => {
    window.clearTimeout(peekTimer.current);
    setPeek(true);
  };
  const dropPeek = () => {
    window.clearTimeout(peekTimer.current);
    peekTimer.current = window.setTimeout(() => setPeek(false), 900);
  };
  const onMenu = (open: boolean) => setMenus((n) => Math.max(0, n + (open ? 1 : -1)));
  useEffect(() => {
    setPeek(false);
    setMenus(0);
  }, [fsBar]);
  useEffect(() => () => window.clearTimeout(peekTimer.current), []);

  const special = (v: string) => {
    const cap = captureRef.current;
    if (!cap) return;
    if (v === "type") {
      setTyping(true);
      return;
    }
    const combo = SPECIAL_KEYS[v];
    if (combo) cap.sendCombo(combo);
    keysRef.current?.focus({ preventScroll: true });
  };

  // The host learns the viewer's choices as soon as the session is live.
  useEffect(() => {
    if (!live || !session) return;
    session.setQuality(prefs.preset, prefs.retina);
    if (prefs.cursorInVideo) session.setCursorInVideo(true);
    // Only once per session: later changes go through update().
  }, [live, session]);

  useEffect(() => {
    const v = videoRef.current;
    if (!v || v.srcObject === view.video) return;
    v.srcObject = view.video;
    // Muted video autoplays everywhere; the sound has its own element.
    void Promise.resolve(v.play()).catch(() => undefined);
  }, [view.video]);

  // The computer's sound, while this viewer may hear it. Browsers let a
  // page start sound only after a click: the Connect click usually
  // counts, otherwise the page asks for one.
  const playSound = useCallback(() => {
    const a = audioRef.current;
    if (!a || !a.srcObject) return;
    void Promise.resolve(a.play()).then(
      () => setSoundBlocked(false),
      () => setSoundBlocked(true),
    );
  }, []);
  useEffect(() => {
    const a = audioRef.current;
    if (!a) return;
    if (a.srcObject !== view.audio) a.srcObject = view.audio;
    a.muted = prefs.muted || !hears;
    if (view.audio && hears && !prefs.muted) playSound();
    else setSoundBlocked(false);
  }, [view.audio, hears, prefs.muted, playSound]);

  // The clipboard shared with the host: what is copied there lands here.
  useEffect(() => {
    if (view.hostClipboard && sharesClipboard) hostClipboard.current?.offer(view.hostClipboard.text);
  }, [view.hostClipboard, sharesClipboard]);
  useEffect(() => {
    if (!sharesClipboard) hostClipboard.current?.clear();
    captureRef.current?.setOptions({ ctrlIsCmd: prefsRef.current.ctrlIsCmd, pasteToHost: sharesClipboard });
  }, [sharesClipboard]);

  // The host sends the picture at the size it is shown here (device
  // pixels), so nothing is sent that the screen would throw away.
  useEffect(() => {
    const el = screenRef.current;
    if (!el || !session || !live || terminal || typeof ResizeObserver === "undefined") return;
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
  }, [session, live, terminal]);

  // The host's pointer, drawn here at its place over the picture. While
  // this viewer controls, its own pointer takes the host's shape instead
  // (no delay at all), unless the pointer is locked (a game).
  const placeCursor = useCallback(
    (c: CursorState | null) => {
      const img = cursorRef.current;
      const video = videoRef.current;
      if (!img || !video) return;
      const scale0 = display && display.pointsWidth > 0 ? contentRect(video.clientWidth, video.clientHeight, video.videoWidth, video.videoHeight, prefs.view).w / display.pointsWidth : 1;
      if (control && !locked && !touch && !cursorInVideo && c?.shape) {
        img.style.display = "none";
        const res = c.shape.pixels > 0 ? c.shape.pixels / (c.shape.width * scale0) : 1;
        const hx = Math.round((c.shape.hotX / res) * 100) / 100;
        const hy = Math.round((c.shape.hotY / res) * 100) / 100;
        video.style.cursor = `image-set(url("${c.shape.url}") ${res.toFixed(3)}x) ${Math.round(hx)} ${Math.round(hy)}, default`;
        return;
      }
      video.style.cursor = control && !cursorInVideo ? "default" : "";
      if (!c || !c.visible || !c.shape || cursorInVideo || !display || display.pointsWidth <= 0) {
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
    [cursorInVideo, prefs.view, display, control, locked, touch],
  );

  useEffect(() => {
    registerCursor((c) => {
      lastCursor.current = c;
      placeCursor(c);
    });
    placeCursor(lastCursor.current);
    return () => registerCursor(null);
  }, [registerCursor, placeCursor]);

  const say = (text: string | null) => {
    window.clearTimeout(noteTimer.current);
    setCaptureNote(text);
    if (text) noteTimer.current = window.setTimeout(() => setCaptureNote(null), 8000);
  };
  useEffect(() => () => window.clearTimeout(noteTimer.current), []);

  // The host's pointer over a screenshot, in the picture's pixels (unless it is in the picture already).
  const cursorDraw = (): CursorDraw | null => {
    const v = videoRef.current;
    const c = lastCursor.current;
    if (!v || !c || !c.visible || !c.shape || cursorInVideo || !display || display.pointsWidth <= 0) return null;
    const scale = v.videoWidth / display.pointsWidth; // picture pixels per host point
    const perPixel = c.shape.pixels > 0 ? c.shape.width / c.shape.pixels : 1;
    return {
      url: c.shape.url,
      x: c.x * v.videoWidth - c.shape.hotX * perPixel * scale,
      y: c.y * v.videoHeight - c.shape.hotY * perPixel * scale,
      width: c.shape.width * scale,
      height: c.shape.height * scale,
    };
  };

  // A screenshot: the picture as it is now, saved once the host shows it.
  const takeScreenshot = async () => {
    const v = videoRef.current;
    if (!session || (!v && !terminal)) return;
    const when = new Date();
    const canvas = terminal ? termApi.current?.canvas() : null;
    const blob = terminal
      ? canvas
        ? await new Promise<Blob | null>((r) => canvas.toBlob(r, "image/png"))
        : new Blob([termApi.current?.text(false) ?? ""], { type: "text/plain" })
      : await screenshot(v!, cursorDraw());
    if (!blob) return;
    if (!(await session.askScreenshot())) {
      say(t.capture.refused);
      return;
    }
    const name = captureName(hostName, when, terminal && !canvas ? "txt" : "png");
    downloadBlob(blob, name);
    say(t.capture.saved.replace("{name}", name));
  };

  const recordingStopped = (reason: StopReason, name: string) => {
    recRef.current = null;
    setRecSince(null);
    session?.stopRecording();
    if (!terminal && !prefsRef.current.cursorInVideo) session?.setCursorInVideo(false);
    const notes: Record<StopReason, string> = {
      stopped: t.capture.saved.replace("{name}", name),
      host: t.capture.stoppedHost,
      time: t.capture.stoppedTime,
      size: t.capture.stoppedSize,
      ended: t.capture.stoppedEnded,
      failed: t.capture.failed,
    };
    say(notes[reason]);
  };

  // A recording: where it goes is asked first (while the click counts),
  // then the host, and it starts only once the person there sees it.
  const startRecording = async (cast = false) => {
    const v = videoRef.current;
    if (!session || (!v && !terminal) || recRef.current) return;
    if (cast) return startCast();
    const canvas = terminal ? termApi.current?.canvas() : null;
    if (terminal && !canvas) {
      say(t.capture.unsupported);
      return;
    }
    const type = recordingType();
    if (!type) {
      say(t.capture.unsupported);
      return;
    }
    const name = captureName(hostName, new Date(), type.ext);
    const sink = await sinkFor(name);
    if (!sink) return;
    if (!(await session.startRecording())) {
      await sink.abort();
      say(t.capture.refused);
      return;
    }
    const tracks = canvas ? canvas.captureStream(30).getVideoTracks() : [...((v!.srcObject as MediaStream | null)?.getVideoTracks() ?? [])];
    const sound = audioRef.current?.srcObject as MediaStream | null;
    if (hears && sound) tracks.push(...sound.getAudioTracks());
    if (!terminal && !prefsRef.current.cursorInVideo) session.setCursorInVideo(true);
    try {
      recRef.current = new ScreenRecorder(new MediaStream(tracks), type.mime, sink, (reason) => recordingStopped(reason, name));
    } catch {
      session.stopRecording();
      if (!terminal && !prefsRef.current.cursorInVideo) session.setCursorInVideo(false);
      await sink.abort();
      say(t.capture.failed);
      return;
    }
    setRecSince(Date.now());
    say(null);
  };

  // A terminal recorded as text: an asciicast of what is shown, saved when it stops.
  const startCast = async () => {
    const api = termApi.current;
    if (!session || !api) return;
    if (!(await session.startRecording())) {
      say(t.capture.refused);
      return;
    }
    const name = captureName(hostName, new Date(), "cast");
    const cast = api.cast();
    let done = false;
    recRef.current = {
      stop: (reason: StopReason) => {
        if (done) return;
        done = true;
        downloadBlob(new Blob([cast.stop()], { type: "application/x-asciicast" }), name);
        recordingStopped(reason, name);
      },
    } as unknown as ScreenRecorder;
    setRecSince(Date.now());
    say(null);
  };

  // The host stopped it (or took the permission away), or the session ended.
  useEffect(() => {
    if (recRef.current && (!view.recording || !live)) recRef.current.stop(live ? "host" : "ended");
  }, [view.recording, live]);
  useEffect(() => () => recRef.current?.stop("ended"), []);
  useEffect(() => {
    if (recSince === null) return;
    const every = window.setInterval(() => setTick((n) => n + 1), 1000);
    return () => window.clearInterval(every);
  }, [recSince]);
  const recTime = recSince === null ? "" : clock(Date.now() - recSince);

  // A terminal opened here is shown at once; one that closed gives way to
  // another; the first list after coming back says what was still open.
  const [resumed, setResumed] = useState<string | null>(null);
  const tabIds = tabs.map((x) => x.id).join(",");
  useEffect(() => {
    if (!terminal) return;
    const ids = tabs.map((x) => x.id);
    const fresh = ids.filter((id) => !knownTabs.current.includes(id));
    if (knownTabs.current.length === 0 && view.terminalResumed && ids.length > 0) {
      const name = view.host?.name || t.session.title;
      setResumed(ids.length === 1 ? t.terminal.resumedOne.replace("{name}", name) : t.terminal.resumed.replace("{n}", String(ids.length)).replace("{name}", name));
      window.setTimeout(() => setResumed(null), 7000);
    }
    if (opening.current && fresh.length > 0) {
      opening.current = false;
      showTab(fresh[fresh.length - 1]!);
    }
    knownTabs.current = ids;
    setActivity((a) => a.filter((id) => ids.includes(id)));
    // tabIds stands for the list.
  }, [terminal, tabIds]);
  const showTab = (id: number) => {
    setActiveTab(id);
    setActivity((a) => a.filter((x) => x !== id));
  };
  const openTab = () => {
    opening.current = true;
    session?.openTerminal();
  };
  // One size bigger or smaller; 0 is the normal size.
  const setFont = (step: number) => {
    const i = TERM_FONTS.indexOf(termFont);
    const next = step === 0 ? 14 : (TERM_FONTS[Math.min(TERM_FONTS.length - 1, Math.max(0, (i < 0 ? 3 : i) + step))] ?? termFont);
    setTermFont(next);
    try {
      window.localStorage.setItem(TERM_FONT_KEY, String(next));
    } catch {
      // storage blocked: the size lasts for this visit
    }
  };
  const copyText = async (selectionOnly: boolean) => {
    const text = termApi.current?.text(selectionOnly) ?? "";
    if (!text) return;
    try {
      await navigator.clipboard.writeText(text);
      say(t.terminal.copied);
    } catch {
      // the browser refused: nothing to do
    }
  };
  const pasteText = async () => {
    try {
      const text = await navigator.clipboard.readText();
      if (text) termApi.current?.type(text);
    } catch {
      // the browser refused: Cmd+V / Ctrl+V in the terminal works too
    }
    termApi.current?.focus();
  };

  const measure = async () => {
    const v = videoRef.current;
    if (!v || !session) return;
    setLatency("measuring");
    const ms = await measureLatency(v, session, stats?.rttMs ?? 0);
    setLatency(ms === null ? "none" : ms);
  };

  const hostName = view.host?.name || t.session.title;
  const notice = view.status.kind === "connecting" ? t.session.connecting : view.status.kind === "reconnecting" ? t.session.reconnecting : null;
  const hostNotice =
    view.notice === "no_screen_permission"
      ? t.session.noScreenPermission
      : view.notice === "capture_failed"
        ? t.session.captureFailed
        : view.notice === "wayland_session"
          ? t.session.waylandSession
          : view.notice === "share_denied"
            ? t.session.shareDenied
            : view.notice === "no_remote_input"
              ? t.session.noRemoteInput
              : view.notice === "no_accessibility" && control
                ? t.session.noAccessibility
                : null;
  return (
    <div className={`stage${fullscreen.pseudo ? " is-pseudo-fullscreen" : ""}`} ref={stageRef}>
      <div className={`cap-zone${fsBar ? " is-fullscreen" : ""}`} onMouseEnter={fsBar ? keepPeek : undefined} onMouseLeave={fsBar ? dropPeek : undefined}>
        {showBar ? (
          <div className="cap" role="toolbar" aria-label={(terminal ? t.terminal.title : t.session.screenOf).replace("{name}", hostName)}>
            <button type="button" className="bar-button bar-danger" aria-label={t.session.disconnect} title={t.session.disconnect} onClick={() => session?.hangUp()}>
              <CloseIcon />
            </button>
            <span className="cap-title">{hostName}</span>
            <span className="cap-divider" aria-hidden="true" />
            {control && terminal && (
              <BarMenu label={t.terminal.keys} icon={<CommandIcon />} onOpen={onMenu}>
                {(close) => (
                  <>
                    {TERMINAL_KEYS.map(([label, bytes]) => (
                      <MenuItem
                        key={label}
                        onSelect={() => {
                          close();
                          termApi.current?.type(bytes);
                          termApi.current?.focus();
                        }}
                      >
                        {label}
                      </MenuItem>
                    ))}
                  </>
                )}
              </BarMenu>
            )}
            {control && !terminal && (
              <BarMenu label={t.keys.menu} icon={<CommandIcon />} onOpen={onMenu}>
                {(close) => (
                  <>
                    {specialKeys(hostIsMac).map((k) => (
                      <MenuItem
                        key={k}
                        onSelect={() => {
                          close();
                          special(k);
                        }}
                      >
                        {t.keys.items[k as keyof typeof t.keys.items]}
                      </MenuItem>
                    ))}
                    <MenuSeparator />
                    <MenuItem
                      onSelect={() => {
                        close();
                        special("type");
                      }}
                    >
                      {t.keys.type}
                    </MenuItem>
                    {touch && (
                      <MenuItem
                        onSelect={() => {
                          close();
                          captureRef.current?.showKeyboard();
                        }}
                      >
                        {t.keys.keyboard}
                      </MenuItem>
                    )}
                    <MenuSeparator />
                    {hostIsMac && (
                      <MenuItem kind="check" checked={prefs.ctrlIsCmd} onSelect={() => update({ ctrlIsCmd: !prefs.ctrlIsCmd })}>
                        {t.keys.ctrlIsCmd}
                      </MenuItem>
                    )}
                    <MenuItem
                      kind="check"
                      checked={locked}
                      onSelect={() => {
                        close();
                        toggleLock();
                      }}
                    >
                      {t.keys.gameMode}
                    </MenuItem>
                  </>
                )}
              </BarMenu>
            )}
            {!terminal && (
              <button
                type="button"
                className={`bar-button chat-button${chatOpen ? " is-on" : ""}`}
                aria-pressed={chatOpen}
                aria-label={unread ? `${t.chat.title} (${unread})` : t.chat.title}
                title={t.chat.title}
                onClick={() => {
                  setChatOpen(!chatOpen);
                  setReadChat(view.chat.length);
                }}
              >
                <ChatIcon size={18} />
                {unread > 0 && <span className="badge">{unread}</span>}
              </button>
            )}
            {mayFiles && (
              <button type="button" className={`bar-button${filesOpen ? " is-on" : ""}`} aria-pressed={filesOpen} aria-label={t.files.title} title={t.files.title} onClick={() => setFilesOpen(!filesOpen)}>
                <FolderIcon size={18} />
              </button>
            )}
            {mayRecord && (
              <BarMenu label={t.capture.menu} icon={<RecordIcon />} onOpen={onMenu}>
                {(close) => (
                  <>
                    <MenuItem
                      onSelect={() => {
                        close();
                        void takeScreenshot();
                      }}
                    >
                      {t.capture.screenshot}
                    </MenuItem>
                    <MenuItem
                      onSelect={() => {
                        close();
                        if (recRef.current) recRef.current.stop("stopped");
                        else void startRecording();
                      }}
                    >
                      {recSince !== null ? t.capture.stop : t.capture.record}
                    </MenuItem>
                    {terminal && recSince === null && (
                      <MenuItem
                        onSelect={() => {
                          close();
                          void startRecording(true);
                        }}
                      >
                        {t.terminal.recordCast}
                      </MenuItem>
                    )}
                    <MenuSeparator />
                    <p className="bar-menu-note">{t.capture.shown}</p>
                  </>
                )}
              </BarMenu>
            )}
            {recSince !== null && (
              <button
                type="button"
                className="bar-button rec-button"
                aria-label={t.capture.recording.replace("{time}", recTime)}
                title={t.capture.recording.replace("{time}", recTime)}
                onClick={() => recRef.current?.stop("stopped")}
              >
                <span className="rec-dot" aria-hidden="true" />
                {recTime}
              </button>
            )}
            {!terminal && typeof navigator.mediaDevices?.getUserMedia === "function" && (
              <button type="button" className={`bar-button${callOpen ? " is-on" : ""}`} aria-pressed={callOpen} aria-label={t.call.start} title={t.call.start} disabled={callOpen} onClick={() => setCallOpen(true)}>
                <CameraIcon size={18} />
              </button>
            )}
            {hears && !terminal && (
              <button
                type="button"
                className={`bar-button${prefs.muted ? "" : " is-on"}`}
                aria-pressed={!prefs.muted}
                aria-label={prefs.muted ? t.session.soundOff : t.session.soundOn}
                title={prefs.muted ? t.session.soundOff : t.session.soundOn}
                onClick={() => update({ muted: !prefs.muted })}
              >
                <SoundIcon size={18} off={prefs.muted} />
              </button>
            )}
            {terminal && (
              <BarMenu label={t.terminal.menu} icon={<TerminalIcon />} onOpen={onMenu}>
                {(close) => (
                  <>
                    {view.host?.terminalTabs && (
                      <>
                        <MenuSection title={t.terminal.menu} />
                        {tabs.map((x) => (
                          <MenuItem
                            key={x.id}
                            kind="radio"
                            checked={x.id === shownTab}
                            onSelect={() => {
                              close();
                              showTab(x.id);
                            }}
                          >
                            {x.name}
                          </MenuItem>
                        ))}
                        {control && (
                          <>
                            <MenuSeparator />
                            <MenuItem
                              onSelect={() => {
                                close();
                                if (tabs.length < 16) openTab();
                              }}
                            >
                              {t.terminal.newTab} <span className="bar-menu-detail">Alt+Shift+T</span>
                            </MenuItem>
                            <MenuItem
                              onSelect={() => {
                                close();
                                setRenaming(shownTab);
                              }}
                            >
                              {t.terminal.rename.replace("{name}", tabs.find((x) => x.id === shownTab)?.name ?? "")}
                            </MenuItem>
                            <MenuItem
                              onSelect={() => {
                                close();
                                session?.closeTerminal(shownTab);
                              }}
                            >
                              {t.terminal.close.replace("{name}", tabs.find((x) => x.id === shownTab)?.name ?? "")}{" "}
                              <span className="bar-menu-detail">{t.terminal.closeHint}</span>
                            </MenuItem>
                          </>
                        )}
                      </>
                    )}
                    <MenuSection title={t.terminal.textSection} />
                    <MenuItem
                      onSelect={() => {
                        close();
                        void copyText(false);
                      }}
                    >
                      {t.terminal.copyScreen}
                    </MenuItem>
                    {control && (
                      <MenuItem
                        onSelect={() => {
                          close();
                          void pasteText();
                        }}
                      >
                        {t.terminal.paste}
                      </MenuItem>
                    )}
                  </>
                )}
              </BarMenu>
            )}
            {terminal && (
              // The terminal's own screen: its text size, kept in this browser.
              // The menu stays open, so a few clicks find the size.
              <BarMenu label={t.session.screenMenu} icon={<MonitorIcon />} onOpen={onMenu}>
                {() => (
                  <>
                    <MenuSection title={t.terminal.fontSize.replace("{n}", String(termFont))} />
                    <MenuItem onSelect={() => setFont(1)}>
                      <span className="bar-menu-key">A+</span> {t.terminal.bigger}
                    </MenuItem>
                    <MenuItem onSelect={() => setFont(-1)}>
                      <span className="bar-menu-key">A−</span> {t.terminal.smaller}
                    </MenuItem>
                    <MenuSeparator />
                    <MenuItem onSelect={() => setFont(0)}>{t.terminal.fontReset}</MenuItem>
                  </>
                )}
              </BarMenu>
            )}
            {!terminal && (
              <BarMenu label={t.session.screenMenu} icon={<MonitorIcon />} onOpen={onMenu}>
                {() => (
                  <>
                    {view.displays.length > 1 && (
                      <>
                        <MenuSection title={t.session.display} />
                        {view.displays.map((d, i) => (
                          <MenuItem key={d.id} kind="radio" checked={d.id === view.display} onSelect={() => session?.selectDisplay(d.id)}>
                            {d.name || t.session.displayN.replace("{n}", String(i + 1))} <span className="bar-menu-detail">{`${d.width}×${d.height}`}</span>
                          </MenuItem>
                        ))}
                      </>
                    )}
                    <MenuSection title={t.session.quality} />
                    {(["quality", "balanced", "speed"] as const).map((p) => (
                      <MenuItem key={p} kind="radio" checked={prefs.preset === p} onSelect={() => update({ preset: p })}>
                        {t.session.presets[p]}
                      </MenuItem>
                    ))}
                    <MenuSection title={t.session.view} />
                    {(["fit", "original", "stretch"] as const).map((v) => (
                      <MenuItem key={v} kind="radio" checked={prefs.view === v} onSelect={() => update({ view: v })}>
                        {t.session.views[v]}
                      </MenuItem>
                    ))}
                    <MenuSeparator />
                    <MenuItem kind="check" checked={prefs.retina} onSelect={() => update({ retina: !prefs.retina })}>
                      {t.session.retina}
                    </MenuItem>
                    <MenuItem kind="check" checked={prefs.cursorInVideo} onSelect={() => update({ cursorInVideo: !prefs.cursorInVideo })}>
                      {t.session.cursorInVideo}
                    </MenuItem>
                  </>
                )}
              </BarMenu>
            )}
            <button
              type="button"
              className="bar-button"
              aria-label={fullscreen.active ? t.session.exitFullscreen : t.session.fullscreen}
              title={fullscreen.active ? t.session.exitFullscreen : t.session.fullscreen}
              onClick={fullscreen.toggle}
            >
              {fullscreen.active ? <ShrinkIcon size={18} /> : <ExpandIcon size={18} />}
            </button>
            {!fsBar && (
              <button type="button" className="bar-button" aria-label={t.session.hideBar} title={t.session.hideBar} onClick={() => setBarHidden(true)}>
                <ChevronIcon up />
              </button>
            )}
          </div>
        ) : (
          <button
            type="button"
            className="cap-handle"
            aria-label={t.session.showBar}
            title={t.session.showBar}
            onClick={() => (fsBar ? keepPeek() : setBarHidden(false))}
          >
            <ChevronIcon />
          </button>
        )}
      </div>
      {hostNotice && (
        <p className="stage-banner" role="status">
          {hostNotice}
        </p>
      )}
      {resumed && (
        <p className="stage-banner stage-banner-ok" role="status">
          {resumed}
        </p>
      )}
      {terminal && !control && (
        <p className="stage-banner" role="status">
          {t.terminal.readOnly}
        </p>
      )}
      {captureNote && (
        <p className="stage-banner" role="status">
          {captureNote}
        </p>
      )}
      {soundBlocked && hears && !prefs.muted && (
        <p className="stage-banner" role="status">
          {t.session.soundBlocked}{" "}
          <button type="button" className="button button-primary button-small" onClick={playSound}>
            {t.session.soundEnable}
          </button>
        </p>
      )}
      {clipboardWaiting && sharesClipboard && view.hostClipboard && (
        <p className="stage-banner" role="status">
          {t.session.clipboardWaiting}{" "}
          <button type="button" className="button button-primary button-small" onClick={() => hostClipboard.current?.gesture()}>
            {t.session.clipboardCopy}
          </button>
        </p>
      )}
      {terminal && session ? (
        <div
          className="stage-screen stage-terminal"
          // Files dropped on the terminal go to the computer, as on a screen.
          onDragOver={(e) => {
            if (!mayFiles || !e.dataTransfer.types.includes("Files")) return;
            e.preventDefault();
            setDragging(true);
          }}
          onDragLeave={(e) => {
            if (e.currentTarget === e.target) setDragging(false);
          }}
          onDrop={(e) => {
            setDragging(false);
            if (!mayFiles || !e.dataTransfer.files.length) return;
            e.preventDefault();
            setDropped([...e.dataTransfer.files]);
            setFilesOpen(true);
          }}
        >
          <Suspense fallback={null}>
            <TerminalStage
              session={session}
              ids={tabs.map((x) => x.id)}
              active={shownTab}
              fontSize={termFont}
              control={control}
              onActivity={(id) => setActivity((a) => (a.includes(id) ? a : [...a, id]))}
              onKey={(key) => {
                if (!view.host?.terminalTabs) return;
                if (key === "new") {
                  if (control && tabs.length < 16) openTab();
                  return;
                }
                const i = tabs.findIndex((x) => x.id === shownTab);
                const next = tabs[(i + (key === "next" ? 1 : tabs.length - 1)) % tabs.length];
                if (next) showTab(next.id);
              }}
              onApi={(api) => {
                termApi.current = api;
              }}
            />
          </Suspense>
          {view.host?.terminalTabs && (
            <TerminalTabs
              tabs={tabs}
              active={shownTab}
              activity={activity}
              control={control}
              renaming={renaming}
              onShow={showTab}
              onOpen={openTab}
              onRename={(id, name) => {
                setRenaming(null);
                if (name !== null) session.renameTerminal(id, name);
                termApi.current?.focus();
              }}
              onStartRename={setRenaming}
            />
          )}
          {dragging && <div className="drop-overlay">{t.files.dropHere}</div>}
          {filesOpen && session.files && <FilesPanel files={session.files} dropped={dropped} onClose={() => { setFilesOpen(false); termApi.current?.focus(); }} />}
          {notice && (
            <p className="stage-notice" role="status">
              {notice}
            </p>
          )}
        </div>
      ) : (
      <div
        className={`stage-screen view-${prefs.view}`}
        ref={screenRef}
        // A click or key press is when Safari and Firefox let the page
        // write the clipboard: what the host copied lands then.
        onPointerDownCapture={() => hostClipboard.current?.gesture()}
        onKeyDownCapture={() => hostClipboard.current?.gesture()}
        // Files dropped on the screen go to the host.
        onDragOver={(e) => {
          if (!mayFiles || !e.dataTransfer.types.includes("Files")) return;
          e.preventDefault();
          setDragging(true);
        }}
        onDragLeave={(e) => {
          if (e.currentTarget === e.target) setDragging(false);
        }}
        onDrop={(e) => {
          setDragging(false);
          if (!mayFiles || !e.dataTransfer.files.length) return;
          e.preventDefault();
          setDropped([...e.dataTransfer.files]);
          setFilesOpen(true);
        }}
      >
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
        {/* The figures float over the picture, as in go-link's player: they stay in full screen. */}
        <div className="stream-info-float" hidden={fsBar && !showBar}>
          <StreamInfo
            stats={stats}
            sent={view.sent}
            latency={latency}
            onMeasure={() => void measure()}
            quality={t.session.presets[prefs.preset]}
            hud={prefs.hud}
            onHud={(on) => update({ hud: on })}
          />
        </div>
        {prefs.hud && <StreamHud stats={stats} sent={view.sent} />}
        <audio ref={audioRef} autoPlay />
        {/* Where the keyboard goes while this viewer controls: it keeps nothing. */}
        <textarea ref={keysRef} className="key-sink" aria-label={t.keys.sink} autoComplete="off" autoCorrect="off" autoCapitalize="off" spellCheck={false} />
        {typing && <TypeText onSend={(text) => { captureRef.current?.typeText(text); setTyping(false); keysRef.current?.focus({ preventScroll: true }); }} onClose={() => setTyping(false)} />}
        {dragging && <div className="drop-overlay">{t.files.dropHere}</div>}
        {filesOpen && session?.files && <FilesPanel files={session.files} dropped={dropped} onClose={() => setFilesOpen(false)} />}
        {callOpen && session && <CallPanel view={view} session={session} onClose={closeCall} />}
        {chatOpen && (
          <ChatPanel
            lines={view.chat}
            hostName={hostName}
            hostTyping={view.hostTyping}
            onSend={(text) => session?.sendChat(text)}
            onTyping={(on) => session?.sendTyping(on)}
            onClose={() => {
              setChatOpen(false);
              setReadChat(view.chat.length);
              keysRef.current?.focus({ preventScroll: true });
            }}
          />
        )}
        {notice && (
          <p className="stage-notice" role="status">
            {notice}
          </p>
        )}
      </div>
      )}
    </div>
  );
}

/** The terminals' strip, like tmux's: a click shows one, a double click renames it. */
function TerminalTabs({
  tabs,
  active,
  activity,
  control,
  renaming,
  onShow,
  onOpen,
  onRename,
  onStartRename,
}: {
  tabs: TerminalTab[];
  active: number;
  activity: number[];
  control: boolean;
  renaming: number | null;
  onShow: (id: number) => void;
  onOpen: () => void;
  onRename: (id: number, name: string | null) => void;
  onStartRename: (id: number) => void;
}) {
  return (
    <div className="term-tabs" role="tablist" aria-label={t.terminal.menu}>
      {tabs.map((x, i) =>
        renaming === x.id ? (
          <input
            key={x.id}
            className="term-tab-edit"
            aria-label={t.terminal.renameField}
            defaultValue={x.name}
            maxLength={40}
            autoFocus
            onFocus={(e) => e.currentTarget.select()}
            onBlur={(e) => onRename(x.id, e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") onRename(x.id, e.currentTarget.value);
              else if (e.key === "Escape") onRename(x.id, null);
            }}
          />
        ) : (
          <button
            key={x.id}
            type="button"
            role="tab"
            aria-selected={x.id === active}
            className={`term-tab${x.id === active ? " is-on" : ""}`}
            title={activity.includes(x.id) ? `${x.name} · ${t.terminal.newOutput}` : x.name}
            onClick={() => onShow(x.id)}
            onDoubleClick={() => control && onStartRename(x.id)}
          >
            <span className="term-tab-n">{i + 1}</span>
            <span className="term-tab-name">{x.name}</span>
            {activity.includes(x.id) && <span className="term-tab-dot" aria-label={t.terminal.newOutput} />}
          </button>
        ),
      )}
      {control && tabs.length < 16 && (
        <button type="button" className="term-tab term-tab-new" aria-label={t.terminal.newTab} title={`${t.terminal.newTab} (Alt+Shift+T)`} onClick={onOpen}>
          +
        </button>
      )}
    </div>
  );
}

/** A recording's length: "4:05", "1:02:09". */
export function clock(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000));
  const p = (n: number) => String(n).padStart(2, "0");
  const h = Math.floor(s / 3600);
  return h ? `${h}:${p(Math.floor(s / 60) % 60)}:${p(s % 60)}` : `${Math.floor(s / 60)}:${p(s % 60)}`;
}

/** The special keys menu, as shortcuts by KeyboardEvent.code. */
export const SPECIAL_KEYS: Record<string, string[]> = {
  cmdTab: ["MetaLeft", "Tab"],
  cmdSpace: ["MetaLeft", "Space"],
  forceQuit: ["MetaLeft", "AltLeft", "Escape"],
  screenshot: ["MetaLeft", "ShiftLeft", "Digit3"],
  lockScreen: ["ControlLeft", "MetaLeft", "KeyQ"],
  ctrlAltDel: ["ControlLeft", "AltLeft", "Delete"],
  altTab: ["AltLeft", "Tab"],
  win: ["MetaLeft"],
  printScreen: ["PrintScreen"],
};

/** The special keys that make sense for the host's system. */
export function specialKeys(hostIsMac: boolean): string[] {
  return hostIsMac ? ["cmdTab", "cmdSpace", "forceQuit", "screenshot", "lockScreen"] : ["ctrlAltDel", "altTab", "win", "printScreen"];
}

/** "Type text": a paste typed out on the host, character by character. */
function TypeText({ onSend, onClose }: { onSend: (text: string) => void; onClose: () => void }) {
  const [text, setText] = useState("");
  return (
    <div className="type-text" role="dialog" aria-label={t.keys.type}>
      <label className="field-label" htmlFor="type-text-field">
        {t.keys.typeHint}
      </label>
      <textarea id="type-text-field" className="type-text-field" autoFocus value={text} maxLength={20000} onChange={(e) => setText(e.target.value)} />
      <div className="join-row">
        <button type="button" className="button button-primary" disabled={!text} onClick={() => onSend(text)}>
          {t.keys.typeSend}
        </button>
        <button type="button" className="button button-secondary" onClick={onClose}>
          {t.session.cancel}
        </button>
      </div>
    </div>
  );
}
