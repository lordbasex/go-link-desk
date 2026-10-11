// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useEffect, useRef, useState } from "react";
import { SignalClient, browserLabel, resolveSignalUrl } from "@go-link-desk/shared";
import { DEFAULT_SIGNAL_URL } from "../config";
import { Session, type CursorState, type Door, type NewLink, type SessionView } from "./session";

export interface UseSessionOptions {
  door: Door;
  name: string;
  /** Bumped to start a visit (Connect, Try again); 0 starts nothing. */
  attempt: number;
  /** The host's pointer moves (kept off React's renders). */
  onCursor?: (c: CursorState) => void;
  /** A claim got its link; the computer forgot this browser's link. */
  onLinked?: (link: NewLink) => void;
  onUnlinked?: () => void;
  /** Injectable for tests. */
  createClient?: () => SignalClient;
}

/** The signaling server sessions go through. */
export function signalUrl(): string {
  return resolveSignalUrl(DEFAULT_SIGNAL_URL, safeStorage()).url;
}

/** localStorage, or nothing where it is blocked. */
export function safeStorage(): Storage | undefined {
  try {
    return window.localStorage;
  } catch {
    return undefined;
  }
}

/**
 * Runs one visit to a host per attempt, on a signaling connection of its
 * own that closes with it. Returns the visit's view and the Session (to
 * hang up and read statistics).
 */
export function useSession({ door, name, attempt, onCursor, onLinked, onUnlinked, createClient }: UseSessionOptions): { view: SessionView | null; session: Session | null } {
  const [view, setView] = useState<SessionView | null>(null);
  const [session, setSession] = useState<Session | null>(null);
  const nameRef = useRef(name);
  nameRef.current = name;
  const cursorRef = useRef(onCursor);
  cursorRef.current = onCursor;
  const linkRef = useRef({ onLinked, onUnlinked });
  linkRef.current = { onLinked, onUnlinked };
  const doorRef = useRef(door);
  doorRef.current = door;
  const key = door.kind === "join" ? ("invite" in door.target ? `i:${door.target.invite}` : `c:${door.target.code}`) : door.kind === "claim" ? `l:${door.code}` : `d:${door.device.deviceId}`;

  useEffect(() => {
    if (attempt === 0) return;
    const client = createClient ? createClient() : new SignalClient({ url: signalUrl() });
    const s = new Session({
      client,
      door: doorRef.current,
      onLinked: (l) => linkRef.current.onLinked?.(l),
      onUnlinked: () => linkRef.current.onUnlinked?.(),
      name: nameRef.current,
      browser: browserLabel(navigator.userAgent),
      onChange: setView,
      onCursor: (c) => cursorRef.current?.(c),
    });
    setSession(s);
    setView(s.current);
    void s.start();
    return () => s.stop();
    // The door is read through key, which names it.
  }, [attempt, key, createClient]);

  return { view, session };
}
