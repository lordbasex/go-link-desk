// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// A browser linked to a computer with its pairing code remembers the link,
// so it comes back without a code (unattended access) after a reload or a
// restart of that computer. The token was handed out by the host over the
// encrypted WebRTC channel; the host keeps only its hash. The list lives
// in this browser's localStorage, with the sessions it had lately: there
// are no accounts and nothing of it leaves the browser.

const DEVICES = "go-link-desk.devices";
const RECENTS = "go-link-desk.recents";

/** The most linked computers and recent sessions kept. */
export const MAX_DEVICES = 50;
export const MAX_RECENTS = 12;
/** The largest thumbnail kept (a data: URL of a small JPEG). */
export const MAX_THUMB = 60_000;

/** A computer this browser is linked to. */
export interface SavedDevice {
  deviceId: string;
  linkId: string;
  token: string;
  /** The link only works through the signaling server it was made on. */
  signalUrl: string;
  /** The computer's name (its welcome), and what the person here calls it. */
  name: string;
  label: string;
  os: string;
  favorite: boolean;
  savedAt: number;
  lastUsed: number;
  /** The screen as it was when the last session ended. */
  thumb: string;
  /** go-link-desk-terminal: a shell, not a screen. */
  terminal?: boolean;
}

/** A session this browser had lately, linked or with a code. */
export interface RecentSession {
  /** How to go back: the invitation or code of /c/, or a linked device. */
  target: string;
  deviceId: string;
  name: string;
  os: string;
  at: number;
  thumb: string;
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const TARGET = /^([A-Za-z0-9_-]{22}|\d{9})$/;

const text = (v: unknown, max: number) => (typeof v === "string" ? v.slice(0, max) : "");
const num = (v: unknown) => (typeof v === "number" && Number.isFinite(v) ? v : 0);
const thumb = (v: unknown) => (typeof v === "string" && v.length <= MAX_THUMB && v.startsWith("data:image/jpeg;base64,") ? v : "");

function readList(storage: Storage | undefined, key: string): unknown[] {
  try {
    const raw = storage?.getItem(key);
    const v: unknown = raw ? JSON.parse(raw) : [];
    return Array.isArray(v) ? v : [];
  } catch {
    return [];
  }
}

function writeList(storage: Storage | undefined, key: string, list: unknown[]): void {
  try {
    storage?.setItem(key, JSON.stringify(list));
  } catch {
    // Storage full (thumbnails) or blocked: try once without thumbnails.
    try {
      storage?.setItem(key, JSON.stringify(list.map((x) => ({ ...(x as object), thumb: "" }))));
    } catch {
      // nothing more to do: the list lasts only for this page
    }
  }
}

function toDevice(v: unknown): SavedDevice | null {
  if (typeof v !== "object" || v === null) return null;
  const d = v as Record<string, unknown>;
  if (
    typeof d.deviceId !== "string" ||
    !UUID.test(d.deviceId) ||
    typeof d.linkId !== "string" ||
    !/^[0-9a-f]{16}$/.test(d.linkId) ||
    typeof d.token !== "string" ||
    !/^[A-Za-z0-9_-]{20,100}$/.test(d.token) ||
    typeof d.signalUrl !== "string"
  ) {
    return null;
  }
  return {
    deviceId: d.deviceId.toLowerCase(), linkId: d.linkId, token: d.token, signalUrl: d.signalUrl,
    name: text(d.name, 64), label: text(d.label, 64), os: text(d.os, 16), favorite: d.favorite === true,
    savedAt: num(d.savedAt), lastUsed: num(d.lastUsed), thumb: thumb(d.thumb),
    ...(d.terminal === true ? { terminal: true } : {}),
  };
}

/** The computers this browser is linked to through signalUrl (all of them without it), favorites first, then the latest used. */
export function loadDevices(storage: Storage | undefined, signalUrl?: string): SavedDevice[] {
  return readList(storage, DEVICES)
    .map(toDevice)
    .filter((d): d is SavedDevice => d !== null && (signalUrl === undefined || d.signalUrl === signalUrl))
    .sort((a, b) => Number(b.favorite) - Number(a.favorite) || b.lastUsed - a.lastUsed);
}

/** The link to one computer, if this browser has it. */
export function findDevice(storage: Storage | undefined, deviceId: string, signalUrl: string): SavedDevice | null {
  return loadDevices(storage, signalUrl).find((d) => d.deviceId === deviceId.toLowerCase()) ?? null;
}

/** Keeps a link (replacing this computer's previous one on the same server). */
export function saveDevice(storage: Storage | undefined, device: SavedDevice): void {
  const clean = toDevice(device);
  if (!clean) return;
  const others = loadDevices(storage).filter((d) => !(d.deviceId === clean.deviceId && d.signalUrl === clean.signalUrl));
  writeList(storage, DEVICES, [clean, ...others].slice(0, MAX_DEVICES));
}

/** Changes what is kept about a linked computer (name, label, favorite, last use, thumbnail). */
export function updateDevice(storage: Storage | undefined, deviceId: string, signalUrl: string, patch: Partial<Pick<SavedDevice, "name" | "label" | "os" | "favorite" | "lastUsed" | "thumb" | "terminal">>): void {
  const list = loadDevices(storage);
  const i = list.findIndex((d) => d.deviceId === deviceId.toLowerCase() && d.signalUrl === signalUrl);
  if (i < 0) return;
  const next = toDevice({ ...list[i], ...patch });
  if (!next) return;
  list[i] = next;
  writeList(storage, DEVICES, list);
}

/** Forgets the link to a computer here (the computer forgets it when it is revoked there, or when the browser unlinks itself). */
export function removeDevice(storage: Storage | undefined, deviceId: string, signalUrl: string): void {
  writeList(storage, DEVICES, loadDevices(storage).filter((d) => !(d.deviceId === deviceId.toLowerCase() && d.signalUrl === signalUrl)));
}

function toRecent(v: unknown): RecentSession | null {
  if (typeof v !== "object" || v === null) return null;
  const r = v as Record<string, unknown>;
  const target = typeof r.target === "string" && TARGET.test(r.target) ? r.target : "";
  const deviceId = typeof r.deviceId === "string" && UUID.test(r.deviceId) ? r.deviceId.toLowerCase() : "";
  if (!target && !deviceId) return null;
  return { target, deviceId, name: text(r.name, 64), os: text(r.os, 16), at: num(r.at), thumb: thumb(r.thumb) };
}

/** The latest sessions, newest first. */
export function loadRecents(storage: Storage | undefined): RecentSession[] {
  return readList(storage, RECENTS)
    .map(toRecent)
    .filter((r): r is RecentSession => r !== null)
    .sort((a, b) => b.at - a.at);
}

/** Notes a session (the same computer replaces its older entry). */
export function addRecent(storage: Storage | undefined, recent: RecentSession): void {
  const clean = toRecent(recent);
  if (!clean) return;
  const same = (r: RecentSession) => (clean.deviceId ? r.deviceId === clean.deviceId : r.target === clean.target);
  writeList(storage, RECENTS, [clean, ...loadRecents(storage).filter((r) => !same(r))].slice(0, MAX_RECENTS));
}

/** Forgets every recent session. */
export function clearRecents(storage: Storage | undefined): void {
  writeList(storage, RECENTS, []);
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
