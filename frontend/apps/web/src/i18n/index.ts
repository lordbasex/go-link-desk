// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useSyncExternalStore } from "react";
import { en, type Messages } from "./en";
import { es } from "./es";
import { pt } from "./pt";

export type Lang = "en" | "es" | "pt";

/** Languages of the switcher, in order. English unless the browser prefers Spanish or Portuguese. */
export const LANGS: readonly { id: Lang; label: string; name: string }[] = [
  { id: "en", label: "EN", name: "English" },
  { id: "es", label: "ES", name: "Español" },
  { id: "pt", label: "PT", name: "Português" },
];

const DICTIONARIES: Record<Lang, Messages> = { en, es, pt };
const KEY = "go-link-desk.lang";

/**
 * The browser's language among ours: Spanish or Portuguese when the
 * browser prefers it (es-AR, pt-BR…), else English.
 */
export function browserLang(prefs: readonly string[]): Lang {
  for (const p of prefs) {
    const base = p.toLowerCase().split("-")[0];
    if (base === "es" || base === "pt" || base === "en") return base;
  }
  return "en";
}

/** The language picked in the switcher, else the browser's. */
function savedLang(): Lang {
  try {
    const v = window.localStorage.getItem(KEY);
    if (v === "en" || v === "es" || v === "pt") return v;
  } catch {
    // storage blocked
  }
  const prefs = window.navigator.languages?.length ? window.navigator.languages : [window.navigator.language ?? "en"];
  return browserLang(prefs);
}

let current: Lang = typeof window === "undefined" ? "en" : savedLang();
const listeners = new Set<() => void>();

/**
 * The texts of the active language. Components read it while rendering, so
 * a language change followed by a re-render shows the new texts.
 */
export const t: Messages = new Proxy({} as Messages, {
  get: (_, key) => DICTIONARIES[current][key as keyof Messages],
});

export function getLang(): Lang {
  return current;
}

export function setLang(lang: Lang): void {
  if (lang === current) return;
  current = lang;
  try {
    window.localStorage.setItem(KEY, lang);
  } catch {
    // storage blocked: the choice lasts for this page
  }
  if (typeof document !== "undefined") document.documentElement.lang = lang;
  listeners.forEach((fn) => fn());
}

function subscribe(fn: () => void): () => void {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

/** The active language; re-renders when it changes. */
export function useLang(): Lang {
  return useSyncExternalStore(subscribe, getLang, () => "en");
}

if (typeof document !== "undefined") document.documentElement.lang = current;
