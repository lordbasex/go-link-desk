// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Which signaling server the web uses. By default the official one (build
// config); a user who runs their own server can switch to it from the UI.
// The choice lives in localStorage and is only changed by hand, never from
// a URL parameter, so a link cannot redirect people to a hostile server.

export const SIGNAL_URL_STORAGE_KEY = "go-link-desk.signal-url";

export type SignalUrlProblem = "format" | "scheme" | "insecure";

export type SignalUrlCheck = { ok: true; url: string } | { ok: false; problem: SignalUrlProblem };

const LOOPBACK = new Set(["localhost", "127.0.0.1", "[::1]"]);

/**
 * Validates a signaling URL. wss:// is always accepted. ws:// is accepted
 * only for loopback hosts, for local development: a page served over
 * HTTPS cannot open unencrypted sockets to the internet anyway.
 */
export function checkSignalUrl(raw: string): SignalUrlCheck {
  let url: URL;
  try {
    url = new URL(raw.trim());
  } catch {
    return { ok: false, problem: "format" };
  }
  if (url.protocol !== "wss:" && url.protocol !== "ws:") return { ok: false, problem: "scheme" };
  if (!url.hostname) return { ok: false, problem: "format" };
  if (url.protocol === "ws:" && !LOOPBACK.has(url.hostname)) return { ok: false, problem: "insecure" };
  return { ok: true, url: url.toString() };
}

export interface SignalUrlChoice {
  url: string;
  /** true when the user picked a server other than the official one. */
  custom: boolean;
}

function readStorage(storage: Storage | undefined): string | null {
  try {
    return storage?.getItem(SIGNAL_URL_STORAGE_KEY) ?? null;
  } catch {
    return null; // storage can be disabled (privacy mode)
  }
}

/** Returns the stored custom server, or the default when none is valid. */
export function resolveSignalUrl(defaultUrl: string, storage?: Storage): SignalUrlChoice {
  const stored = readStorage(storage);
  if (stored) {
    const check = checkSignalUrl(stored);
    if (check.ok && check.url !== defaultUrl) return { url: check.url, custom: true };
  }
  return { url: defaultUrl, custom: false };
}

export function saveCustomSignalUrl(url: string, storage: Storage): void {
  storage.setItem(SIGNAL_URL_STORAGE_KEY, url);
}

export function clearCustomSignalUrl(storage: Storage): void {
  storage.removeItem(SIGNAL_URL_STORAGE_KEY);
}
