// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// A browser linked to a device with the pairing code remembers the link,
// so it comes back without a code after a reload or a device restart. The
// token was handed out by the device over the encrypted WebRTC channel;
// the device keeps only its hash.

const KEY = "go-link-desk.device-link";

export interface SavedLink {
  deviceId: string;
  linkId: string;
  token: string;
  /** The link only works through the signaling server it was made on. */
  signalUrl: string;
  savedAt: number;
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** The link saved for this signaling server, if any. */
export function loadSavedLink(storage: Storage | undefined, signalUrl: string): SavedLink | null {
  if (!storage) return null;
  let raw: string | null = null;
  try {
    raw = storage.getItem(KEY);
  } catch {
    return null;
  }
  if (!raw) return null;
  try {
    const v = JSON.parse(raw) as Partial<SavedLink>;
    if (
      typeof v.deviceId !== "string" ||
      !UUID.test(v.deviceId) ||
      typeof v.linkId !== "string" ||
      !/^[0-9a-f]{16}$/.test(v.linkId) ||
      typeof v.token !== "string" ||
      !/^[A-Za-z0-9_-]{20,100}$/.test(v.token) ||
      v.signalUrl !== signalUrl
    ) {
      return null;
    }
    return { deviceId: v.deviceId, linkId: v.linkId, token: v.token, signalUrl: v.signalUrl, savedAt: typeof v.savedAt === "number" ? v.savedAt : 0 };
  } catch {
    return null;
  }
}

export function saveLink(storage: Storage | undefined, link: SavedLink): void {
  try {
    storage?.setItem(KEY, JSON.stringify(link));
  } catch {
    // storage full or blocked: the link lasts only for this page
  }
}

export function clearSavedLink(storage: Storage | undefined): void {
  try {
    storage?.removeItem(KEY);
  } catch {
    // nothing to clear
  }
}

/** auth_ok from the device: the token is only present on the first link. */
export interface AuthOk {
  type: "auth_ok";
  device_id: string;
  link_id: string;
  token?: string;
}

export function parseAuthOk(msg: unknown): AuthOk | null {
  if (typeof msg !== "object" || msg === null) return null;
  const m = msg as Record<string, unknown>;
  if (m.type !== "auth_ok" || typeof m.device_id !== "string" || typeof m.link_id !== "string") return null;
  return { type: "auth_ok", device_id: m.device_id, link_id: m.link_id, token: typeof m.token === "string" ? m.token : undefined };
}
