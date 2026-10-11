// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useId, useMemo, useRef, useState, type FormEvent } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { editCodeFromChange, formatCode, normalizeCode, saveDevice, updateDevice, type CodeEdit, type SignalClient } from "@go-link-desk/shared";
import { t } from "../i18n";
import type { Door, HostInfo, NewLink } from "../session/session";
import { safeStorage, signalUrl } from "../session/useSession";
import { useRecent } from "./SessionPage";
import { SessionRunner } from "./SessionRunner";

/**
 * /link and /link/<code>: links this browser to a computer for unattended
 * access, with the linking code its Settings show. The person at that
 * computer accepts, with the permissions the link keeps; from then on this
 * browser comes back from My computers without a code.
 */
export function LinkPage({ createClient }: { createClient?: () => SignalClient }) {
  const { code: raw = "" } = useParams();
  const code = normalizeCode(raw);
  if (!code) return <LinkForm />;
  return <LinkSession code={code} createClient={createClient} />;
}

function LinkForm() {
  const navigate = useNavigate();
  const id = useId();
  const [field, setField] = useState<CodeEdit>({ text: "", caret: 0 });
  const [tried, setTried] = useState(false);
  const code = normalizeCode(field.text);
  const submit = (e: FormEvent) => {
    e.preventDefault();
    setTried(true);
    if (code) navigate(`/link/${code}`);
  };
  return (
    <form className="join" onSubmit={submit} noValidate>
      <h1 className="join-title">{t.link.title}</h1>
      <p className="muted">{t.link.text}</p>
      <label htmlFor={`${id}-code`} className="field-label">
        {t.link.codeLabel}
      </label>
      <div className="join-row">
        <input
          id={`${id}-code`}
          className="input"
          autoComplete="off"
          inputMode="numeric"
          autoFocus
          placeholder="123 456 789"
          aria-invalid={tried && !code}
          aria-describedby={`${id}-hint`}
          value={field.text}
          onChange={(e) => {
            const el = e.target;
            setField(editCodeFromChange(field.text, el.value, el.selectionStart ?? el.value.length, false));
          }}
        />
        <button type="submit" className="button button-primary">
          {t.link.next}
        </button>
      </div>
      <span id={`${id}-hint`} className="small muted" role={tried && !code ? "alert" : undefined}>
        {tried && !code ? t.link.invalid : t.link.codeHint}
      </span>
      <div>
        <Link to="/devices" className="button button-ghost">
          {t.devices.title}
        </Link>
      </div>
    </form>
  );
}

function LinkSession({ code, createClient }: { code: string; createClient?: () => SignalClient }) {
  const door = useMemo<Door>(() => ({ kind: "claim", code }), [code]);
  const linked = useRef<NewLink | null>(null);
  const host = useRef<HostInfo | null>(null);
  const [deviceId, setDeviceId] = useState("");
  const recent = useRecent("", deviceId);
  const url = signalUrl();
  const onLinked = (l: NewLink) => {
    linked.current = l;
    const now = Date.now();
    saveDevice(safeStorage(), {
      deviceId: l.deviceId, linkId: l.linkId, token: l.token, signalUrl: url,
      name: host.current?.name ?? "", os: host.current?.os ?? "", label: "", favorite: false, savedAt: now, lastUsed: now, thumb: "",
    });
    setDeviceId(l.deviceId);
  };
  const onHost = (h: HostInfo) => {
    host.current = h;
    if (linked.current) updateDevice(safeStorage(), linked.current.deviceId, url, { name: h.name, os: h.os });
    recent.host(h);
  };
  return (
    <SessionRunner
      door={door}
      title={t.link.sessionTitle.replace("{code}", formatCode(code))}
      waitingText={t.link.waiting}
      back={{ to: "/link", label: t.link.back }}
      onLinked={onLinked}
      onHost={onHost}
      onLive={recent.live}
      onSnapshot={(thumb) => {
        if (linked.current) updateDevice(safeStorage(), linked.current.deviceId, url, { thumb, lastUsed: Date.now() });
        recent.snapshot(thumb);
      }}
      createClient={createClient}
    />
  );
}
