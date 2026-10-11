// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useCallback, useId, useRef, useState, type FormEvent } from "react";
import { Link, Navigate, useParams } from "react-router-dom";
import { MAX_NAME, formatCode, parseInvite, type InviteTarget, type SignalClient } from "@go-link-desk/shared";
import { t } from "../i18n";
import { LiveSession } from "../session/LiveSession";
import { isOver, type CursorState, type SessionView } from "../session/session";
import { useSession } from "../session/useSession";

const NAME_KEY = "go-link-desk.name";

function savedName(): string {
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

/**
 * /c/<invite or code>: a session with one computer. The viewer says its
 * name, knocks, waits for the person at that computer to accept, and then
 * sees the screen.
 */
export function SessionPage({ createClient }: { createClient?: () => SignalClient }) {
  const { target: raw = "" } = useParams();
  const target = parseInvite(raw);
  if (!target) return <Navigate to="/c" replace />;
  return <SessionWith target={target} createClient={createClient} />;
}

function SessionWith({ target, createClient }: { target: InviteTarget; createClient?: () => SignalClient }) {
  const [name, setName] = useState(savedName);
  const [attempt, setAttempt] = useState(0);
  const cursorFn = useRef<((c: CursorState) => void) | null>(null);
  const registerCursor = useCallback((fn: ((c: CursorState) => void) | null) => {
    cursorFn.current = fn;
  }, []);
  const { view, session } = useSession({ target, name, attempt, createClient, onCursor: (c) => cursorFn.current?.(c) });
  const shown = "code" in target ? formatCode(target.code) : target.invite;
  const start = () => {
    saveName(name.trim());
    setAttempt((n) => n + 1);
  };
  if (!view || attempt === 0) return <NameForm code={shown} name={name} setName={setName} onSubmit={start} />;
  const s = view.status;
  if (s.kind === "joining" || s.kind === "waiting") {
    return (
      <section className="join" aria-live="polite">
        <h1 className="join-title">{t.session.nameTitle.replace("{code}", shown)}</h1>
        <p className="notice">{s.kind === "joining" ? t.session.joining : t.session.waiting}</p>
        {s.kind === "waiting" && <p className="small muted">{t.session.waitingHint}</p>}
        <div>
          <button type="button" className="button button-secondary" onClick={() => session?.hangUp()}>
            {t.session.cancel}
          </button>
        </div>
      </section>
    );
  }
  if (isOver(s)) return <Over view={view} onRetry={start} />;
  return <LiveSession view={view} session={session} registerCursor={registerCursor} />;
}

function NameForm({ code, name, setName, onSubmit }: { code: string; name: string; setName: (v: string) => void; onSubmit: () => void }) {
  const id = useId();
  const submit = (e: FormEvent) => {
    e.preventDefault();
    onSubmit();
  };
  return (
    <form className="join" onSubmit={submit}>
      <h1 className="join-title">{t.session.nameTitle.replace("{code}", code)}</h1>
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
        <Link to="/c" className="button button-ghost">
          {t.session.back}
        </Link>
      </div>
    </form>
  );
}

function Over({ view, onRetry }: { view: SessionView; onRetry: () => void }) {
  const s = view.status;
  const text =
    s.kind === "rejected"
      ? t.session.rejected[s.reason]
      : s.kind === "ended"
        ? t.session.ended[s.reason]
        : s.kind === "notFound"
          ? t.session.notFound
          : s.kind === "rateLimited"
            ? t.session.rateLimited
            : t.session.unreachable;
  return (
    <section className="join">
      <h1 className="join-title">{t.session.title}</h1>
      <p className="notice" role="status">
        {text}
      </p>
      <div className="join-row">
        {s.kind !== "notFound" && (
          <button type="button" className="button button-primary" onClick={onRetry}>
            {t.session.retry}
          </button>
        )}
        <Link to="/c" className="button button-secondary">
          {t.session.back}
        </Link>
      </div>
    </section>
  );
}
