// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
// go-link-desk-terminal's terminals, drawn where a session shows the
// screen: one xterm.js on a canvas (WebGL, so it records and takes
// screenshots like a screen), showing the chosen terminal; each one's
// output is kept here, so switching is at once. Loaded only for a terminal.

import { useEffect, useRef } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebglAddon } from "@xterm/addon-webgl";
import "@xterm/xterm/css/xterm.css";
import type { Session } from "./session";

/** What each terminal keeps here, as the computer does. */
const KEEP = 2 << 20;

/** A dark terminal, from the site's palette. */
const THEME = {
  background: "#0d1117",
  foreground: "#e6edf3",
  cursor: "#58a6ff",
  selectionBackground: "#264f78",
  black: "#484f58", red: "#ff7b72", green: "#3fb950", yellow: "#d29922",
  blue: "#58a6ff", magenta: "#bc8cff", cyan: "#39c5cf", white: "#b1bac4",
  brightBlack: "#6e7681", brightRed: "#ffa198", brightGreen: "#56d364", brightYellow: "#e3b341",
  brightBlue: "#79c0ff", brightMagenta: "#d2a8ff", brightCyan: "#56d4dd", brightWhite: "#f0f6fc",
};

/** What the session's toolbar does with the terminal. */
export interface TerminalApi {
  /** The canvas it is drawn on (null without WebGL): for screenshots and video. */
  canvas(): HTMLCanvasElement | null;
  /** Types bytes into the shown terminal (special keys). */
  type(text: string): void;
  /** The selection, or everything shown, as text. */
  text(selectionOnly: boolean): string;
  focus(): void;
  /** Starts an asciicast (.cast) of what is shown; stop() returns the file's text. */
  cast(): { stop(): string };
}

/** The terminals' hosts whose <style> elements are kept off the page. */
const hosts = new Set<HTMLElement>();
let patched = false;

/**
 * xterm.js writes its colors and sizes into <style> elements, which the
 * site's Content Security Policy (style-src 'self', no inline styles)
 * refuses, with an error in the console for each. Inside a terminal's
 * host they are never put on the page: their rules go into a stylesheet
 * built in script instead (adoptedStyleSheets), which the policy allows,
 * and follow them as xterm.js changes them.
 */
function adoptStyles(el: HTMLElement): () => void {
  if (typeof CSSStyleSheet === "undefined" || !("adoptedStyleSheets" in document)) return () => undefined;
  const sheet = new CSSStyleSheet();
  document.adoptedStyleSheets = [...document.adoptedStyleSheets, sheet];
  const kept = new Set<HTMLStyleElement>();
  const sync = () => {
    try {
      sheet.replaceSync([...kept].map((s) => s.textContent ?? "").join("\n"));
    } catch {
      // a rule the browser does not take: the others stay
    }
  };
  const mo = new MutationObserver(sync);
  const keep = (style: HTMLStyleElement) => {
    kept.add(style);
    mo.observe(style, { subtree: true, childList: true, characterData: true });
    sync();
  };
  (el as HTMLElement & { __keepStyle?: (s: HTMLStyleElement) => void }).__keepStyle = keep;
  hosts.add(el);
  if (!patched) {
    patched = true;
    const append = Node.prototype.appendChild;
    Node.prototype.appendChild = function <T extends Node>(this: Node, child: T): T {
      if (child instanceof HTMLStyleElement) {
        // xterm.js builds its elements before it puts them in the host: a
        // style in a tree not yet on the page is the newest terminal's.
        const detached = this.getRootNode() !== document;
        for (const h of [...hosts].reverse()) {
          if (h === this || h.contains(this) || detached) {
            (h as HTMLElement & { __keepStyle?: (s: HTMLStyleElement) => void }).__keepStyle?.(child);
            return child; // off the page: its rules are in the adopted sheet
          }
        }
      }
      return append.call(this, child) as T;
    };
  }
  return () => {
    hosts.delete(el);
    mo.disconnect();
    document.adoptedStyleSheets = document.adoptedStyleSheets.filter((s) => s !== sheet);
  };
}

/** Each terminal's output, bounded. */
class Logs {
  private parts = new Map<number, Uint8Array[]>();
  private sizes = new Map<number, number>();
  add(id: number, b: Uint8Array): void {
    const list = this.parts.get(id) ?? [];
    list.push(b.slice());
    let size = (this.sizes.get(id) ?? 0) + b.length;
    while (size > KEEP && list.length > 1) size -= list.shift()!.length;
    this.parts.set(id, list);
    this.sizes.set(id, size);
  }
  all(id: number): Uint8Array {
    const list = this.parts.get(id) ?? [];
    const out = new Uint8Array(this.sizes.get(id) ?? 0);
    let at = 0;
    for (const p of list) {
      out.set(p, at);
      at += p.length;
    }
    return out;
  }
  forget(keep: Set<number>): void {
    for (const id of [...this.parts.keys()]) {
      if (!keep.has(id)) {
        this.parts.delete(id);
        this.sizes.delete(id);
      }
    }
  }
}

export default function TerminalStage({
  session,
  ids,
  active,
  fontSize,
  control,
  onActivity,
  onKey,
  onApi,
}: {
  session: Session;
  /** The terminals that exist (their output is kept). */
  ids: number[];
  active: number;
  fontSize: number;
  control: boolean;
  /** A terminal not shown wrote something. */
  onActivity: (id: number) => void;
  /** The terminals' own keys (Alt+Shift+T, Alt+Shift+← →): "new", "prev", "next". */
  onKey: (key: "new" | "prev" | "next") => void;
  onApi: (api: TerminalApi | null) => void;
}) {
  const boxRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<Terminal | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const logs = useRef(new Logs());
  const activeRef = useRef(active);
  const castRef = useRef<((data: string) => void) | null>(null);
  const activityRef = useRef(onActivity);
  activityRef.current = onActivity;
  const keyRef = useRef(onKey);
  keyRef.current = onKey;
  const decoder = useRef(new TextDecoder());
  // What comes first is the computer's history of each terminal, not new output.
  const quietUntil = useRef(0);

  // One terminal for the whole session.
  useEffect(() => {
    const el = boxRef.current;
    if (!el) return;
    const term = new Terminal({
      cursorBlink: true,
      fontFamily: '"JetBrains Mono", ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
      fontSize,
      scrollback: 5000,
      theme: THEME,
      allowProposedApi: false,
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    const unadopt = adoptStyles(el);
    term.open(el);
    let webgl: WebglAddon | null = null;
    try {
      // On a canvas, kept after each frame: video and screenshots can read it.
      webgl = new WebglAddon(true);
      webgl.onContextLoss(() => webgl?.dispose());
      term.loadAddon(webgl);
    } catch {
      webgl = null; // the DOM renderer: it works, it just cannot be recorded as video
    }
    termRef.current = term;
    fitRef.current = fit;
    const refit = () => {
      try {
        fit.fit();
      } catch {
        // not laid out yet
      }
    };
    refit();
    const shown = (b: Uint8Array) => {
      term.write(b);
      castRef.current?.(decoder.current.decode(b, { stream: true }));
    };
    quietUntil.current = performance.now() + 2000;
    const detach = session.attachTerminal((id, bytes) => {
      logs.current.add(id, bytes);
      if (id === activeRef.current) shown(bytes);
      else if (performance.now() > quietUntil.current) activityRef.current(id);
    }, () => {
      logs.current = new Logs();
      term.reset();
      quietUntil.current = performance.now() + 2000;
    });
    // The browser keeps Ctrl+Shift+T and Ctrl+Tab for itself: these go with Alt+Shift.
    term.attachCustomKeyEventHandler((e) => {
      if (e.type !== "keydown" || !e.altKey || !e.shiftKey || e.ctrlKey || e.metaKey) return true;
      const key = e.code === "KeyT" ? "new" : e.code === "ArrowLeft" ? "prev" : e.code === "ArrowRight" ? "next" : null;
      if (!key) return true;
      e.preventDefault();
      keyRef.current(key);
      return false;
    });
    const enc = new TextEncoder();
    const typed = term.onData((data) => session.sendTerminal(activeRef.current, enc.encode(data)));
    const binary = term.onBinary((data) => session.sendTerminal(activeRef.current, Uint8Array.from(data, (c) => c.charCodeAt(0) & 0xff)));
    const sized = term.onResize(({ cols, rows }) => session.sendTerminalSize(cols, rows));
    session.sendTerminalSize(term.cols, term.rows);
    const ro = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(refit);
    ro?.observe(el);
    term.focus();
    onApi({
      // The text's canvas (WebGL); the link layer over it is transparent.
      canvas: () => (webgl ? (el.querySelector(".xterm-screen canvas:not(.xterm-link-layer)") as HTMLCanvasElement | null) : null),
      type: (text) => session.sendTerminal(activeRef.current, enc.encode(text)),
      text: (selectionOnly) => {
        if (term.hasSelection()) return term.getSelection();
        if (selectionOnly) return "";
        const b = term.buffer.active;
        const lines: string[] = [];
        for (let y = b.viewportY; y < b.viewportY + term.rows; y++) lines.push(b.getLine(y)?.translateToString(true) ?? "");
        return lines.join("\n").replace(/\n+$/, "\n");
      },
      focus: () => term.focus(),
      cast: () => {
        const start = performance.now();
        const lines = [JSON.stringify({ version: 2, width: term.cols, height: term.rows, timestamp: Math.floor(Date.now() / 1000), env: { TERM: "xterm-256color" } })];
        castRef.current = (data) => lines.push(JSON.stringify([Number(((performance.now() - start) / 1000).toFixed(6)), "o", data]));
        // It starts with what is on screen now.
        castRef.current(new TextDecoder().decode(logs.current.all(activeRef.current).slice(-65536)));
        return {
          stop: () => {
            castRef.current = null;
            return lines.join("\n") + "\n";
          },
        };
      },
    });
    return () => {
      onApi(null);
      ro?.disconnect();
      typed.dispose();
      binary.dispose();
      sized.dispose();
      detach();
      webgl?.dispose();
      term.dispose();
      unadopt();
      termRef.current = null;
      fitRef.current = null;
    };
    // The terminal is made once per session; the font and the tab change below.
  }, [session]);

  // Another terminal: what it showed, at once.
  useEffect(() => {
    activeRef.current = active;
    const term = termRef.current;
    if (!term) return;
    term.reset();
    const all = logs.current.all(active);
    term.write(all);
    castRef.current?.("\x1b[2J\x1b[3J\x1b[H" + new TextDecoder().decode(all.slice(-65536)));
    term.focus();
  }, [active]);

  // Terminals that closed are forgotten.
  const idsKey = ids.join(",");
  useEffect(() => {
    logs.current.forget(new Set(ids));
    // ids changes identity on every list; its contents are what count.
  }, [idsKey]);

  useEffect(() => {
    const term = termRef.current;
    if (!term) return;
    term.options.fontSize = fontSize;
    try {
      fitRef.current?.fit();
    } catch {
      // not laid out yet
    }
  }, [fontSize]);

  useEffect(() => {
    if (termRef.current) termRef.current.options.disableStdin = !control;
  }, [control]);

  return (
    <div className="terminal-box">
      <div className="terminal-host" ref={boxRef} />
    </div>
  );
}
