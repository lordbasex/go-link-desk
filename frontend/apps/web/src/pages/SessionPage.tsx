// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useMemo } from "react";
import { Navigate, useNavigate, useParams } from "react-router-dom";
import { addRecent, formatCode, parseInvite, type InviteTarget, type SignalClient } from "@go-link-desk/shared";
import { t } from "../i18n";
import type { Door, HostInfo } from "../session/session";
import { safeStorage } from "../session/useSession";
import { SessionRunner } from "./SessionRunner";

/**
 * /c/<invite or code>: a session with one computer. The viewer says its
 * name, knocks, waits for the person at that computer to accept, and then
 * sees the screen.
 */
export function SessionPage({ createClient }: { createClient?: () => SignalClient }) {
  const { target: raw = "" } = useParams();
  const target = parseInvite(raw);
  if (!target) return <Navigate to="/web" replace />;
  return <JoinSession target={target} createClient={createClient} />;
}

function JoinSession({ target, createClient }: { target: InviteTarget; createClient?: () => SignalClient }) {
  const door = useMemo<Door>(() => ({ kind: "join", target }), [target]);
  const shown = "code" in target ? formatCode(target.code) : target.invite;
  const key = "code" in target ? target.code : target.invite;
  const recent = useRecent(key, "");
  const navigate = useNavigate();
  return (
    <SessionRunner
      door={door}
      title={t.session.nameTitle.replace("{code}", shown)}
      back={{ to: "/web", label: t.session.back }}
      // A 9-digit code that is no session's may be a computer's linking
      // code (go-link-desk-terminal link, Settings › Unattended access):
      // one box for both, the link page asks the computer.
      onNotFound={"code" in target ? () => navigate(`/link/${target.code}`, { replace: true, state: { auto: true } }) : undefined}
      onHost={recent.host}
      onLive={recent.live}
      onSnapshot={recent.snapshot}
      createClient={createClient}
    />
  );
}

/** Keeps the session in the recent list: when it goes live, and its thumbnail. */
export function useRecent(target: string, deviceId: string) {
  return useMemo(() => {
    let host: HostInfo | null = null;
    let thumb = "";
    const note = () => addRecent(safeStorage(), { target, deviceId, name: host?.name ?? "", os: host?.os ?? "", at: Date.now(), thumb });
    let live = false;
    return {
      host: (h: HostInfo) => {
        host = h;
        if (live) note();
      },
      live: () => {
        live = true;
        note();
      },
      snapshot: (url: string) => {
        thumb = url;
        note();
      },
    };
  }, [target, deviceId]);
}
