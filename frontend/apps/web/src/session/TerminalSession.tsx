// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
// go-link-desk-terminal's session: the computer's shell in xterm.js, in
// place of the screen. Loaded only when a computer offers a terminal.

import { useEffect, useRef, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { t } from "../i18n";
import { CloseIcon, ExpandIcon, ShrinkIcon } from "../components/Icons";
import { useFullscreen } from "../components/useFullscreen";
import type { Session, SessionView } from "./session";

const FONT_KEY = "go-link-desk.terminal-font";
const FONT_SIZES = [11, 12, 13, 14, 16, 18, 20, 24];

function loadFont(): number {
  try {
    const n = Number(window.localStorage.getItem(FONT_KEY));
    return FONT_SIZES.includes(n) ? n : 14;
  } catch {
    return 14;
  }
}

function saveFont(n: number): void {
  try {
    window.localStorage.setItem(FONT_KEY, String(n));
  } catch {
    // storage blocked: the size lasts for this visit
  }
}

/**
 * xterm.js writes its colors and sizes into <style> elements, which the
 * site's Content Security Policy (style-src 'self', no inline styles)
 * refuses. Their rules go into a stylesheet built in script instead
 * (adoptedStyleSheets), which the policy allows; the refused elements
 * stay, unused.
 */
function adoptStyles(el: HTMLElement): () => void {
  if (typeof CSSStyleSheet === "undefined" || !("adoptedStyleSheets" in document)) return () => undefined;
  const sheet = new CSSStyleSheet();
  document.adoptedStyleSheets = [...document.adoptedStyleSheets, sheet];
  const sync = () => {
    try {
      sheet.replaceSync([...el.querySelectorAll("style")].map((s) => s.textContent ?? "").join("\n"));
    } catch {
      // a rule the browser does not take: the others stay
    }
  };
  const mo = new MutationObserver(sync);
  mo.observe(el, { subtree: true, childList: true, characterData: true });
  sync();
  return () => {
    mo.disconnect();
    document.adoptedStyleSheets = document.adoptedStyleSheets.filter((s) => s !== sheet);
  };
}

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

export default function TerminalSession({ view, session }: { view: SessionView; session: Session | null }) {
  const stageRef = useRef<HTMLDivElement>(null);
  const boxRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<Terminal | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const fullscreen = useFullscreen(stageRef);
  const [font, setFont] = useState(loadFont);
  const control = view.permissions?.control === true;
  const hostName = view.host?.name || t.session.title;
  const live = view.status.kind === "live" || view.status.kind === "reconnecting";

  // One terminal for the whole session: the shell's output in, what is typed out.
  useEffect(() => {
    const el = boxRef.current;
    if (!session || !el) return;
    const term = new Terminal({
      cursorBlink: true,
      fontFamily: '"JetBrains Mono", ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
      fontSize: loadFont(),
      scrollback: 5000,
      theme: THEME,
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    const unadopt = adoptStyles(el);
    term.open(el);
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
    const detach = session.attachTerminal((bytes) => term.write(bytes));
    const enc = new TextEncoder();
    const typed = term.onData((data) => session.sendTerminal(enc.encode(data)));
    // Mouse reports in X10 mode come as bytes, not text.
    const binary = term.onBinary((data) => session.sendTerminal(Uint8Array.from(data, (c) => c.charCodeAt(0) & 0xff)));
    const sized = term.onResize(({ cols, rows }) => session.sendTerminalSize(cols, rows));
    session.sendTerminalSize(term.cols, term.rows);
    const ro = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(refit);
    ro?.observe(el);
    term.focus();
    return () => {
      ro?.disconnect();
      typed.dispose();
      binary.dispose();
      sized.dispose();
      detach();
      term.dispose();
      unadopt();
      termRef.current = null;
      fitRef.current = null;
    };
  }, [session]);

  // A viewer that may only watch types nothing.
  useEffect(() => {
    if (termRef.current) termRef.current.options.disableStdin = !control;
  }, [control]);

  useEffect(() => {
    const term = termRef.current;
    if (!term) return;
    term.options.fontSize = font;
    try {
      fitRef.current?.fit();
    } catch {
      // not laid out yet
    }
  }, [font]);

  const zoom = (step: number) => {
    const i = FONT_SIZES.indexOf(font);
    const next = FONT_SIZES[Math.min(FONT_SIZES.length - 1, Math.max(0, (i < 0 ? 3 : i) + step))] ?? font;
    setFont(next);
    saveFont(next);
    termRef.current?.focus();
  };

  return (
    <div className={`stage terminal-stage${fullscreen.pseudo ? " is-pseudo-fullscreen" : ""}`} ref={stageRef}>
      <div className="terminal-bar" role="toolbar" aria-label={t.terminal.title.replace("{name}", hostName)}>
        <button type="button" className="bar-button bar-danger" aria-label={t.session.disconnect} title={t.session.disconnect} onClick={() => session?.hangUp()}>
          <CloseIcon />
        </button>
        <span className="cap-title">{t.terminal.title.replace("{name}", hostName)}</span>
        <span className="terminal-bar-spacer" />
        <button type="button" className="bar-button terminal-zoom" aria-label={t.terminal.smaller} title={t.terminal.smaller} onClick={() => zoom(-1)}>
          A−
        </button>
        <button type="button" className="bar-button terminal-zoom" aria-label={t.terminal.bigger} title={t.terminal.bigger} onClick={() => zoom(1)}>
          A+
        </button>
        <button
          type="button"
          className="bar-button"
          aria-label={fullscreen.active ? t.session.exitFullscreen : t.session.fullscreen}
          title={fullscreen.active ? t.session.exitFullscreen : t.session.fullscreen}
          onClick={fullscreen.toggle}
        >
          {fullscreen.active ? <ShrinkIcon size={18} /> : <ExpandIcon size={18} />}
        </button>
      </div>
      {!control && live && (
        <p className="stage-banner" role="status">
          {t.terminal.readOnly}
        </p>
      )}
      {view.status.kind === "reconnecting" && (
        <p className="stage-banner" role="status">
          {t.session.reconnecting}
        </p>
      )}
      <div className="terminal-box" ref={boxRef} aria-label={t.terminal.title.replace("{name}", hostName)} />
    </div>
  );
}
