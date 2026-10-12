// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useSyncExternalStore } from "react";

/**
 * Google Analytics 4 on the website's public pages (the landing, the docs
 * and the privacy page), only with the visitor's consent. The app's pages
 * (/web, sessions, linking, devices) never run it: nothing from Google
 * shares a page with a remote session. Leaving the public pages after
 * Google's script was loaded is a full page load (guardHistory), so the
 * app starts clean, under its strict Content Security Policy.
 */
export const GA_ID = "G-JF0SPS21MX";
// Desk's own cookies (desk__ga, desk__ga_<id>) on this host only, apart from
// go-link.org's: a choice here never touches the other site's.
const COOKIE_PREFIX = "desk";
const KEY = "go-link-desk.consent";

export type Consent = "granted" | "denied" | null;

/** The pages that may count visits: no codes, keys or names in their URLs. */
export function isPublicPath(path: string): boolean {
  return path === "/" || path === "/docs" || path.startsWith("/docs/") || path === "/privacy";
}

function readConsent(): Consent {
  try {
    const v = window.localStorage.getItem(KEY);
    return v === "granted" || v === "denied" ? v : null;
  } catch {
    return null;
  }
}

let consent: Consent = typeof window === "undefined" ? null : readConsent();
let asking = false;
let loaded = false;
const listeners = new Set<() => void>();
const notify = () => listeners.forEach((l) => l());

function subscribe(l: () => void): () => void {
  listeners.add(l);
  return () => listeners.delete(l);
}

/** The visitor's choice and whether the notice is open, for the banner. */
export function useConsent(): { consent: Consent; asking: boolean } {
  const c = useSyncExternalStore(subscribe, () => consent);
  const a = useSyncExternalStore(subscribe, () => asking);
  return { consent: c, asking: a };
}

/**
 * Saves the choice. Rejecting stops counting at once; accepting lets App
 * start it, on a public page only.
 */
export function setConsent(value: "granted" | "denied"): void {
  consent = value;
  asking = false;
  try {
    window.localStorage.setItem(KEY, value);
  } catch {
    // storage blocked: the choice lasts until the page closes
  }
  if (value === "denied") stopAnalytics();
  notify();
}

/** Opens the notice again ("Cookie settings"). */
export function askConsent(): void {
  asking = true;
  notify();
}

type GtagWindow = Window & { dataLayer?: unknown[]; gtag?: (...args: unknown[]) => void } & Record<string, unknown>;

/** Loads gtag.js once; only on a public page and with consent. */
export function startAnalytics(win: GtagWindow = window as unknown as GtagWindow): void {
  if (consent !== "granted" || !isPublicPath(win.location.pathname)) return;
  win[`ga-disable-${GA_ID}`] = false;
  if (loaded) return;
  loaded = true;
  win.dataLayer = win.dataLayer || [];
  win.gtag = function gtag() {
    // gtag.js reads the arguments object itself, as Google's snippet does.
    // eslint-disable-next-line prefer-rest-params
    win.dataLayer!.push(arguments);
  };
  win.gtag("js", new Date());
  win.gtag("config", GA_ID, { cookie_prefix: COOKIE_PREFIX, cookie_domain: win.location.hostname });
  const s = win.document.createElement("script");
  s.async = true;
  s.src = `https://www.googletagmanager.com/gtag/js?id=${GA_ID}`;
  win.document.head.appendChild(s);
}

/** Stops sending and deletes Desk's Google cookies (desk__ga, desk__ga_<id>). */
export function stopAnalytics(win: GtagWindow = window as unknown as GtagWindow): void {
  win[`ga-disable-${GA_ID}`] = true;
  for (const part of win.document.cookie.split(";")) {
    const name = (part.split("=")[0] ?? "").trim();
    if (!name.startsWith(`${COOKIE_PREFIX}_`)) continue;
    for (const domain of ["", `; domain=${win.location.hostname}`]) {
      win.document.cookie = `${name}=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/${domain}`;
    }
  }
}

/** Whether Google's script was loaded in this page. */
export function analyticsLoaded(): boolean {
  return loaded;
}

/**
 * Once Google's script is in the page, a move to an app page is a full
 * page load instead of an in-page one. Installed before gtag.js, so it
 * also runs before gtag's own history listeners and nothing is sent
 * about the app's URLs.
 */
export function guardHistory(win: GtagWindow = window as unknown as GtagWindow): void {
  const leaving = (url: string | URL | null | undefined) =>
    loaded && url != null && !isPublicPath(new URL(String(url), win.location.href).pathname);
  for (const method of ["pushState", "replaceState"] as const) {
    const original = win.history[method].bind(win.history);
    win.history[method] = (data: unknown, unused: string, url?: string | URL | null) => {
      if (leaving(url)) {
        win[`ga-disable-${GA_ID}`] = true;
        win.location.assign(String(url));
        return;
      }
      original(data, unused, url);
    };
  }
  // Back and forward change the URL before anyone can stop it: turn
  // Google off at once and load the app page fresh.
  win.addEventListener("popstate", () => {
    if (loaded && !isPublicPath(win.location.pathname)) {
      win[`ga-disable-${GA_ID}`] = true;
      win.location.reload();
    }
  });
}

/** For tests: forget the state of this module. */
export function resetAnalyticsForTests(): void {
  consent = readConsent();
  asking = false;
  loaded = false;
}
