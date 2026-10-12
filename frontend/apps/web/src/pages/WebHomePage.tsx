// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useEffect, useLayoutEffect, useRef, useState, type FormEvent, type ReactNode } from "react";
import { Link, useNavigate } from "react-router-dom";
import { clearRecents, editCodeFromChange, formatCode, loadDevices, loadRecents, parseInvite, removeDevice, updateDevice, type CodeEdit, type RecentSession, type SavedDevice } from "@go-link-desk/shared";
import { APP_VERSION } from "../config";
import { LANGS, getLang, setLang, t, useLang } from "../i18n";
import { chatSoundOn, setChatSound } from "../session/chatSound";
import { safeStorage, signalUrl } from "../session/useSession";
import { setTheme, useTheme } from "../theme";
import { deviceName } from "./DevicePage";
import "../styles/webapp.css";

/** When something happened, in the page's language. */
function when(ms: number): string {
  return ms ? new Date(ms).toLocaleString(getLang(), { dateStyle: "medium", timeStyle: "short" }) : "";
}

/**
 * desk.go-link.org/web: Home, as the iPhone and iPad app has it. The
 * other computer's code; My computers (linked ones, opened without anyone
 * there); the recent sessions; and Settings in a sheet. Everything here
 * lives in this browser only.
 */
export function WebHomePage() {
  useLang();
  const store = safeStorage();
  const url = signalUrl();
  const [devices, setDevices] = useState(() => loadDevices(store, url));
  const [recents, setRecents] = useState(() => loadRecents(store));
  const [settings, setSettings] = useState(false);
  const reload = () => {
    setDevices(loadDevices(store, url));
    setRecents(loadRecents(store));
  };
  return (
    <div className="ios">
      <div className="ios-col">
        <header className="ios-head">
          <img src="/landing/icon.png" alt="" width={44} height={44} />
          <div>
            <h1 className="ios-title">go-link Desk</h1>
            <p className="ios-subtitle">{t.web.remoteDesktop}</p>
          </div>
          <button type="button" className="ios-round" aria-label={t.web.settings} onClick={() => setSettings(true)}>
            <Glyph d="M12 15.5a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7Z M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1Z" />
          </button>
        </header>

        <ConnectCard />

        <section className="ios-sec" aria-labelledby="ios-computers">
          <div className="ios-sec-head">
            <h2 id="ios-computers">{t.web.myComputers}</h2>
            <Link className="ios-action" to="/link">
              <Glyph d="M12 5v14M5 12h14" size={16} /> {t.web.link}
            </Link>
          </div>
          {devices.length === 0 ? (
            <p className="ios-card ios-empty">{t.web.computersEmpty}</p>
          ) : (
            <ul className="ios-list">
              {devices.map((d) => (
                <ComputerRow key={d.deviceId} device={d} onForget={reload} />
              ))}
            </ul>
          )}
        </section>

        {recents.length > 0 && (
          <section className="ios-sec" aria-labelledby="ios-recent">
            <div className="ios-sec-head">
              <h2 id="ios-recent">{t.web.recent}</h2>
              <button
                type="button"
                className="ios-action"
                onClick={() => {
                  clearRecents(store);
                  reload();
                }}
              >
                {t.web.clear}
              </button>
            </div>
            <ul className="ios-list">
              {recents.map((r) => (
                <RecentRow key={r.deviceId || r.target} recent={r} linked={devices.some((d) => d.deviceId === r.deviceId)} />
              ))}
            </ul>
          </section>
        )}

        <p className="ios-note">{t.web.note}</p>
      </div>
      {settings && <SettingsSheet onClose={() => setSettings(false)} />}
    </div>
  );
}

/** A line icon, drawn like the app's symbols. */
function Glyph({ d, size = 22 }: { d: string; size?: number }) {
  return (
    <svg className="ios-glyph" width={size} height={size} viewBox="0 0 24 24" aria-hidden="true">
      <path d={d} />
    </svg>
  );
}

const MONITOR = "M4 4h16a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1Z M9 20h6 M12 16v4";
const LOCK = "M6 11h12v9H6z M8.5 11V8a3.5 3.5 0 0 1 7 0v3";
const CHEVRON = "M9 6l6 6-6 6";

/** The code card: grouped as typed, and the button lights up once it is a code. */
function ConnectCard() {
  const navigate = useNavigate();
  const [field, setField] = useState<CodeEdit>({ text: "", caret: 0 });
  const [tried, setTried] = useState(false);
  const ref = useRef<HTMLInputElement>(null);
  useLayoutEffect(() => {
    const el = ref.current;
    if (el && document.activeElement === el) el.setSelectionRange(field.caret, field.caret);
  }, [field]);
  const target = parseInvite(field.text);
  const submit = (e: FormEvent) => {
    e.preventDefault();
    setTried(true);
    if (target) navigate(`/c/${"invite" in target ? target.invite : target.code}`);
  };
  return (
    <form className="ios-card ios-connect" onSubmit={submit} noValidate>
      <label htmlFor="ios-code" className="ios-card-title">
        {t.web.connectTitle}
      </label>
      <input
        id="ios-code"
        ref={ref}
        className="ios-code"
        inputMode="numeric"
        autoComplete="off"
        spellCheck={false}
        placeholder="000 000 000"
        aria-label={t.web.codeLabel}
        aria-invalid={tried && !target}
        value={field.text}
        onChange={(e) => setField(editCodeFromChange(field.text, e.target.value, e.target.selectionStart ?? e.target.value.length))}
      />
      {tried && !target && field.text !== "" && (
        <p className="ios-error" role="alert">
          {t.web.invalid}
        </p>
      )}
      <button type="submit" className="ios-primary" disabled={!target}>
        {t.web.connect}
      </button>
    </form>
  );
}

function Row({ to, label, title, detail, trailing, extra }: { to: string; label: string; title: string; detail: string; trailing: ReactNode; extra?: ReactNode }) {
  return (
    <li className="ios-row">
      <Link to={to} className="ios-row-link" aria-label={label}>
        <span className="ios-row-icon">
          <Glyph d={MONITOR} />
        </span>
        <span className="ios-row-text">
          <span className="ios-row-title">{title}</span>
          <span className="ios-row-detail">{detail}</span>
        </span>
        <span className="ios-row-trailing">{trailing}</span>
      </Link>
      {extra}
    </li>
  );
}

function ComputerRow({ device, onForget }: { device: SavedDevice; onForget: () => void }) {
  const [menu, setMenu] = useState(false);
  const [renaming, setRenaming] = useState(false);
  const [forgetting, setForgetting] = useState(false);
  const [label, setLabel] = useState(device.label || device.name);
  const name = deviceName(device) || t.web.linkedComputer;
  const patch = (p: Parameters<typeof updateDevice>[3]) => {
    updateDevice(safeStorage(), device.deviceId, device.signalUrl, p);
    onForget();
  };
  return (
    <>
      <Row
        to={`/d/${device.deviceId}`}
        label={t.web.connectTo.replace("{name}", name)}
        title={device.favorite ? `★ ${name}` : name}
        detail={when(device.lastUsed)}
        trailing={<Glyph d={LOCK} size={18} />}
        extra={
          <button type="button" className="ios-more" aria-expanded={menu} aria-label={t.web.more.replace("{name}", name)} onClick={() => setMenu((m) => !m)}>
            <Glyph d="M5 12h.01M12 12h.01M19 12h.01" size={20} />
          </button>
        }
      />
      {menu && (
        <li className="ios-row-menu">
          {renaming ? (
            <form
              className="ios-rename"
              onSubmit={(e) => {
                e.preventDefault();
                patch({ label: label.trim() === device.name ? "" : label.trim() });
                setRenaming(false);
              }}
            >
              <input className="ios-field" aria-label={t.devices.rename} maxLength={64} autoFocus value={label} onChange={(e) => setLabel(e.target.value)} />
              <button type="submit" className="ios-chip-button">
                {t.devices.save}
              </button>
            </form>
          ) : (
            <>
              <button type="button" className="ios-chip-button" onClick={() => setRenaming(true)}>
                {t.devices.rename}
              </button>
              <button type="button" className="ios-chip-button" aria-pressed={device.favorite} onClick={() => patch({ favorite: !device.favorite })}>
                {device.favorite ? `★ ${t.devices.unfavorite}` : `☆ ${t.devices.favorite}`}
              </button>
              {forgetting ? (
                <button
                  type="button"
                  className="ios-chip-button ios-chip-danger"
                  onClick={() => {
                    removeDevice(safeStorage(), device.deviceId, device.signalUrl);
                    onForget();
                  }}
                >
                  {t.web.forgetConfirm}
                </button>
              ) : (
                <button type="button" className="ios-chip-button" onClick={() => setForgetting(true)}>
                  {t.web.forget}
                </button>
              )}
            </>
          )}
        </li>
      )}
    </>
  );
}

function RecentRow({ recent, linked }: { recent: RecentSession; linked: boolean }) {
  const code = recent.target.length === 9 ? formatCode(recent.target) : "";
  const name = recent.name || code || t.devices.unnamed;
  const to = recent.deviceId ? (linked ? `/d/${recent.deviceId}` : "/link") : `/c/${recent.target}`;
  return <Row to={to} label={t.web.connectTo.replace("{name}", name)} title={name} detail={[code, when(recent.at)].filter(Boolean).join(" · ")} trailing={<Glyph d={CHEVRON} size={16} />} />;
}

/** Settings, as a sheet over Home: language, appearance, the message sound, and what this is. */
function SettingsSheet({ onClose }: { onClose: () => void }) {
  const lang = useLang();
  const theme = useTheme();
  const [sound, setSound] = useState(chatSoundOn);
  const done = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    done.current?.focus();
    const key = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, [onClose]);
  return (
    <div className="ios-sheet-back" onClick={onClose}>
      <div className="ios-sheet" role="dialog" aria-modal="true" aria-labelledby="ios-settings-title" onClick={(e) => e.stopPropagation()}>
        <div className="ios-sheet-bar">
          <span />
          <h2 id="ios-settings-title">{t.web.settings}</h2>
          <button type="button" ref={done} className="ios-bar-button" onClick={onClose}>
            {t.web.done}
          </button>
        </div>
        <div className="ios-sheet-body">
          <Group header={t.web.language}>
            <Segmented value={lang} options={LANGS.map((l) => ({ value: l.id, label: l.name }))} onChange={setLang} label={t.web.language} />
          </Group>
          <Group header={t.web.appearance}>
            <Segmented
              value={theme}
              options={[
                { value: "dark", label: t.web.dark },
                { value: "light", label: t.web.light },
              ]}
              onChange={setTheme}
              label={t.web.appearance}
            />
          </Group>
          <Group footer={t.web.soundFooter}>
            <label className="ios-toggle-row">
              <span>{t.web.sound}</span>
              <input
                type="checkbox"
                role="switch"
                className="ios-switch"
                checked={sound}
                onChange={(e) => {
                  setChatSound(e.target.checked);
                  setSound(e.target.checked);
                }}
              />
            </label>
          </Group>
          <Group footer={t.web.about}>
            <div className="ios-value-row">
              <span>{t.web.version}</span>
              <span className="ios-dim">{APP_VERSION}</span>
            </div>
            <Link className="ios-link-row" to="/" onClick={onClose}>
              {t.web.website}
            </Link>
            <Link className="ios-link-row" to="/privacy" onClick={onClose}>
              {t.footer.privacy}
            </Link>
          </Group>
        </div>
      </div>
    </div>
  );
}

function Group({ header, footer, children }: { header?: string; footer?: string; children: ReactNode }) {
  return (
    <div className="ios-group">
      {header && <h3 className="ios-group-header">{header}</h3>}
      <div className="ios-card ios-group-card">{children}</div>
      {footer && <p className="ios-group-footer">{footer}</p>}
    </div>
  );
}

function Segmented<V extends string>({ value, options, onChange, label }: { value: V; options: { value: V; label: string }[]; onChange: (v: V) => void; label: string }) {
  return (
    <div className="ios-segmented" role="radiogroup" aria-label={label}>
      {options.map((o) => (
        <button key={o.value} type="button" role="radio" aria-checked={o.value === value} onClick={() => onChange(o.value)}>
          {o.label}
        </button>
      ))}
    </div>
  );
}
