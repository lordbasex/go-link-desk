// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useCallback, useEffect, useState, type RefObject } from "react";

// Safari (iPad before 16.4) still uses the webkit-prefixed names.
type FsElement = HTMLElement & { webkitRequestFullscreen?: () => Promise<void> | void };
type FsDocument = Document & { webkitFullscreenElement?: Element | null; webkitExitFullscreen?: () => Promise<void> | void };

function fullscreenElement(): Element | null {
  const d = document as FsDocument;
  return d.fullscreenElement ?? d.webkitFullscreenElement ?? null;
}

/**
 * Full screen for an element. Where the browser cannot put an element in
 * full screen (iPhone Safari only allows it for bare videos), the element
 * fills the window instead: "pseudo" full screen, drawn with CSS.
 */
export function useFullscreen(ref: RefObject<HTMLElement | null>) {
  const [real, setReal] = useState(false);
  const [pseudo, setPseudo] = useState(false);

  useEffect(() => {
    const sync = () => {
      const el = fullscreenElement();
      setReal(el !== null && (el === ref.current || el === document.documentElement));
    };
    document.addEventListener("fullscreenchange", sync);
    document.addEventListener("webkitfullscreenchange", sync);
    return () => {
      document.removeEventListener("fullscreenchange", sync);
      document.removeEventListener("webkitfullscreenchange", sync);
    };
  }, [ref]);

  // Pseudo full screen: no page scroll behind it, and Esc leaves it.
  useEffect(() => {
    if (!pseudo) return;
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const onKey = (e: KeyboardEvent) => (e.key === "Escape" || e.code === "Escape") && setPseudo(false);
    window.addEventListener("keydown", onKey);
    return () => {
      document.body.style.overflow = prev;
      window.removeEventListener("keydown", onKey);
    };
  }, [pseudo]);

  const toggle = useCallback(() => {
    const el = ref.current as FsElement | null;
    if (!el) return;
    if (pseudo) {
      setPseudo(false);
      return;
    }
    const d = document as FsDocument;
    if (fullscreenElement()) {
      void Promise.resolve((d.exitFullscreen ?? d.webkitExitFullscreen)?.call(d)).catch(() => undefined);
      return;
    }
    const request = el.requestFullscreen ?? el.webkitRequestFullscreen;
    if (!request) {
      setPseudo(true);
      return;
    }
    void Promise.resolve(request.call(el)).catch(() => setPseudo(true));
  }, [ref, pseudo]);

  return { active: real || pseudo, pseudo, toggle };
}
