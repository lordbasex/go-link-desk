// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { Suspense, lazy, useCallback, useEffect, useId, useRef, useState, type FormEvent, type ReactNode } from "react";
import { Link } from "react-router-dom";
import { MAX_NAME, type SignalClient } from "@go-link-desk/shared";
import { t } from "../i18n";
import { LiveSession } from "../session/LiveSession";
import { isOver, type CursorState, type Door, type HostInfo, type NewLink, type Session, type SessionView } from "../session/session";
import { useSession } from "../session/useSession";

const NAME_KEY = "go-link-desk.name";

// xterm.js comes only with a computer that offers a terminal.
const TerminalSession = lazy(() => import("../session/TerminalSession"));

export function savedName(): string {
  try {
    return window.localStorage.getItem(NAME_KEY) ?? "";
  } catch {
    return "";
  }
}

function saveName(name: string): void {
  try {
    window.localStorage.setItem(NAME_KEY, name);
  } catch {
    // storage blocked: asked again next time
  }
}

export interface SessionRunnerProps {
  door: Door;
  /** The heading while connecting ("Connect to 915 355 636"). */
  title: string;
  /** What the viewer waits for while the person at the computer decides. */
  waitingText?: string;
  /** Where "back" goes, and what it says. */
  back: { to: string; label: string };
  /** Start at once with the saved name (a linked computer). */
  autoStart?: boolean;
  onLinked?: (link: NewLink) => void;
  onUnlinked?: () => void;
  /** Told who the computer is, once it says. */
  onHost?: (host: HostInfo) => void;
  /** Told when the session goes live, and a thumbnail of its screen now and then. */
  onLive?: () => void;
  onSnapshot?: (url: string) => void;
  /** No computer has that code: a page may try it another way (a linking code) instead of saying so. */
  onNotFound?: () => void;
  createClient?: () => SignalClient;
}

/**
 * One visit to a computer, whatever the door: the viewer says its name,
 * knocks, waits for the person there (or, linked, types the password if
 * the computer asks), and then sees the screen.
 */
export function SessionRunner({ door, title, waitingText, back, autoStart, onLinked, onUnlinked, onHost, onLive, onSnapshot, onNotFound, createClient }: SessionRunnerProps) {
  const [name, setName] = useState(savedName);
  const [attempt, setAttempt] = useState(() => (autoStart && savedName().trim() ? 1 : 0));
  const cursorFn = useRef<((c: CursorState) => void) | null>(null);
  const registerCursor = useCallback((fn: ((c: CursorState) => void) | null) => {
    cursorFn.current = fn;
  }, []);
  const { view, session } = useSession({ door, name, attempt, createClient, onCursor: (c) => cursorFn.current?.(c), onLinked, onUnlinked });
  const start = () => {
    saveName(name.trim());
    setAttempt((n) => n + 1);
  };
  const hostRef = useRef(onHost);
  hostRef.current = onHost;
  const liveRef = useRef(onLive);
  liveRef.current = onLive;
  const host = view?.host;
  useEffect(() => {
    if (host) hostRef.current?.(host);
  }, [host]);
  const live = view?.status.kind === "live";
  useEffect(() => {
    if (live) liveRef.current?.();
  }, [live]);
  const notFoundRef = useRef(onNotFound);
  notFoundRef.current = onNotFound;
  const notFound = view?.status.kind === "notFound";
  useEffect(() => {
    if (notFound) notFoundRef.current?.();
  }, [notFound]);

  if (!view || attempt === 0) return <NameForm title={title} name={name} setName={setName} onSubmit={start} back={back} />;
  const s = view.status;
  if (s.kind === "joining" || s.kind === "waiting") {
    return (
      <Waiting title={title} session={session}>
        <p className="notice">{s.kind === "joining" ? t.session.joining : (waitingText ?? t.session.waiting)}</p>
        {s.kind === "waiting" && door.kind !== "reach" && <p className="small muted">{t.session.waitingHint}</p>}
      </Waiting>
    );
  }
  if (s.kind === "password") return <PasswordForm title={title} view={view} session={session} />;
  if (s.kind === "notFound" && onNotFound) return null; // the page tries it another way
  if (isOver(s)) return <Over view={view} onRetry={start} back={back} />;
  if (view.host?.terminal) {
    return (
      <Suspense fallback={<p className="notice">{t.session.connecting}</p>}>
        <TerminalSession view={view} session={session} />
      </Suspense>
    );
  }
  return <LiveSession view={view} session={session} registerCursor={registerCursor} onSnapshot={onSnapshot} />;
}

function Waiting({ title, session, children }: { title: string; session: Session | null; children: ReactNode }) {
  return (
    <section className="join" aria-live="polite">
      <h1 className="join-title">{title}</h1>
      {children}
      <div>
        <button type="button" className="button button-secondary" onClick={() => session?.hangUp()}>
          {t.session.cancel}
        </button>
      </div>
    </section>
  );
}

function NameForm({ title, name, setName, onSubmit, back }: { title: string; name: string; setName: (v: string) => void; onSubmit: () => void; back: SessionRunnerProps["back"] }) {
  const id = useId();
  const submit = (e: FormEvent) => {
    e.preventDefault();
    onSubmit();
  };
  return (
    <form className="join" onSubmit={submit}>
      <h1 className="join-title">{title}</h1>
      <label htmlFor={`${id}-name`} className="field-label">
        {t.session.nameLabel}
      </label>
      <div className="join-row">
        <input
          id={`${id}-name`}
          className="input input-name"
          autoComplete="name"
          maxLength={MAX_NAME}
          autoFocus
          placeholder={t.session.namePlaceholder}
          aria-describedby={`${id}-hint`}
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <button type="submit" className="button button-primary">
          {t.session.connect}
        </button>
      </div>
      <span id={`${id}-hint`} className="small muted">
        {t.session.nameHint}
      </span>
      <div>
        <Link to={back.to} className="button button-ghost">
          {back.label}
        </Link>
      </div>
    </form>
  );
}

function PasswordForm({ title, view, session }: { title: string; view: SessionView; session: Session | null }) {
  const id = useId();
  const [password, setPassword] = useState("");
  const s = view.status;
  if (s.kind !== "password") return null;
  const submit = (e: FormEvent) => {
    e.preventDefault();
    session?.sendPassword(password);
    setPassword("");
  };
  return (
    <form className="join" onSubmit={submit}>
      <h1 className="join-title">{title}</h1>
      <label htmlFor={`${id}-pw`} className="field-label">
        {t.session.passwordLabel}
      </label>
      <div className="join-row">
        <input
          id={`${id}-pw`}
          className="input input-name"
          type="password"
          autoComplete="current-password"
          autoFocus
          maxLength={128}
          disabled={s.checking}
          aria-describedby={`${id}-hint`}
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        <button type="submit" className="button button-primary" disabled={s.checking || !password}>
          {t.session.connect}
        </button>
      </div>
      <span id={`${id}-hint`} className="small muted" role={s.error ? "alert" : undefined}>
        {s.error === "wrong" ? t.session.passwordWrong.replace("{n}", String(s.left)) : t.session.passwordHint}
      </span>
      <div>
        <button type="button" className="button button-ghost" onClick={() => session?.hangUp()}>
          {t.session.cancel}
        </button>
      </div>
    </form>
  );
}

function Over({ view, onRetry, back }: { view: SessionView; onRetry: () => void; back: SessionRunnerProps["back"] }) {
  const s = view.status;
  let text: string;
  switch (s.kind) {
    case "rejected":
      text = t.session.rejected[s.reason];
      break;
    case "ended":
      text = t.session.ended[s.reason];
      if (view.terminalExit !== null) text = view.terminalExit === 0 ? t.terminal.exitedOk : t.terminal.exited.replace("{code}", String(view.terminalExit));
      break;
    case "notFound":
      text = t.session.notFound;
      break;
    case "rateLimited":
      text = t.session.rateLimited;
      break;
    case "unlinked":
      text = t.session.unlinked;
      break;
    case "offline":
      text = t.session.offline;
      break;
    case "untrusted":
      text = t.session.untrusted;
      break;
    case "passwordRefused":
      text = s.locked ? t.session.passwordLocked.replace("{n}", String(Math.max(1, Math.ceil(s.retryAfter / 60)))) : t.session.passwordBlocked;
      break;
    default:
      text = t.session.unreachable;
  }
  const retry = s.kind !== "notFound" && s.kind !== "unlinked" && s.kind !== "untrusted";
  return (
    <section className="join">
      <h1 className="join-title">{t.session.title}</h1>
      <p className="notice" role="status">
        {text}
      </p>
      <div className="join-row">
        {retry && (
          <button type="button" className="button button-primary" onClick={onRetry}>
            {t.session.retry}
          </button>
        )}
        <Link to={back.to} className="button button-secondary">
          {back.label}
        </Link>
      </div>
    </section>
  );
}
