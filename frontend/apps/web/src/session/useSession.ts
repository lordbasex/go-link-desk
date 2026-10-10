// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useEffect, useRef, useState } from "react";
import { SignalClient, browserLabel, resolveSignalUrl, type InviteTarget } from "@go-link-desk/shared";
import { DEFAULT_SIGNAL_URL } from "../config";
import { Session, type SessionView } from "./session";

export interface UseSessionOptions {
  target: InviteTarget;
  name: string;
  /** Bumped to start a visit (Connect, Try again); 0 starts nothing. */
  attempt: number;
  /** Injectable for tests. */
  createClient?: () => SignalClient;
}

function safeStorage(): Storage | undefined {
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
export function useSession({ target, name, attempt, createClient }: UseSessionOptions): { view: SessionView | null; session: Session | null } {
  const [view, setView] = useState<SessionView | null>(null);
  const [session, setSession] = useState<Session | null>(null);
  const nameRef = useRef(name);
  nameRef.current = name;
  const key = "invite" in target ? `i:${target.invite}` : `c:${target.code}`;

  useEffect(() => {
    if (attempt === 0) return;
    const client = createClient ? createClient() : new SignalClient({ url: resolveSignalUrl(DEFAULT_SIGNAL_URL, safeStorage()).url });
    const s = new Session({
      client,
      target,
      name: nameRef.current,
      browser: browserLabel(navigator.userAgent),
      onChange: setView,
    });
    setSession(s);
    setView(s.current);
    void s.start();
    return () => s.stop();
    // target is read through key, which names it.
  }, [attempt, key, createClient]);

  return { view, session };
}
