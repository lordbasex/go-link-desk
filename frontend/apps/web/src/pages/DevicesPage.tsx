// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useState } from "react";
import { Link } from "react-router-dom";
import { clearRecents, formatCode, loadDevices, loadRecents, removeDevice, updateDevice, type RecentSession, type SavedDevice } from "@go-link-desk/shared";
import { getLang, t } from "../i18n";
import { safeStorage, signalUrl } from "../session/useSession";
import { deviceName } from "./DevicePage";

/** When something happened, in the page's language. */
function when(ms: number): string {
  if (!ms) return t.devices.never;
  return new Date(ms).toLocaleString(getLang(), { dateStyle: "medium", timeStyle: "short" });
}

/**
 * /devices: the computers this browser is linked to (favorites first)
 * and the latest sessions. Everything here lives in this browser only.
 */
export function DevicesPage() {
  const store = safeStorage();
  const url = signalUrl();
  const [devices, setDevices] = useState(() => loadDevices(store, url));
  const [recents, setRecents] = useState(() => loadRecents(store));
  const reload = () => {
    setDevices(loadDevices(store, url));
    setRecents(loadRecents(store));
  };
  return (
    <section className="devices">
      <div className="devices-head">
        <h1 className="join-title">{t.devices.title}</h1>
        <Link to="/link" className="button button-primary">
          {t.devices.link}
        </Link>
      </div>
      <p className="small muted">{t.devices.local}</p>
      {devices.length === 0 ? (
        <p className="notice">{t.devices.empty}</p>
      ) : (
        <ul className="device-list">
          {devices.map((d) => (
            <DeviceCard key={d.deviceId} device={d} onChange={reload} />
          ))}
        </ul>
      )}
      <div className="devices-head">
        <h2 className="devices-subtitle">{t.devices.recent}</h2>
        {recents.length > 0 && (
          <button
            type="button"
            className="button button-ghost"
            onClick={() => {
              clearRecents(store);
              reload();
            }}
          >
            {t.devices.clearRecent}
          </button>
        )}
      </div>
      {recents.length === 0 ? (
        <p className="small muted">{t.devices.recentEmpty}</p>
      ) : (
        <ul className="device-list">
          {recents.map((r) => (
            <RecentCard key={r.deviceId || r.target} recent={r} linked={devices.some((d) => d.deviceId === r.deviceId)} />
          ))}
        </ul>
      )}
    </section>
  );
}

function Thumb({ src, label }: { src: string; label: string }) {
  return src ? <img className="device-thumb" src={src} alt={t.devices.thumbOf.replace("{name}", label)} /> : <div className="device-thumb device-thumb-empty" aria-hidden="true" />;
}

function DeviceCard({ device, onChange }: { device: SavedDevice; onChange: () => void }) {
  const store = safeStorage();
  const [renaming, setRenaming] = useState(false);
  const [label, setLabel] = useState(device.label || device.name);
  const [forgetting, setForgetting] = useState(false);
  const name = deviceName(device);
  const patch = (p: Parameters<typeof updateDevice>[3]) => {
    updateDevice(store, device.deviceId, device.signalUrl, p);
    onChange();
  };
  return (
    <li className="device-card">
      <Thumb src={device.thumb} label={name} />
      <div className="device-body">
        {renaming ? (
          <form
            className="join-row"
            onSubmit={(e) => {
              e.preventDefault();
              patch({ label: label.trim() === device.name ? "" : label.trim() });
              setRenaming(false);
            }}
          >
            <input className="input" aria-label={t.devices.rename} maxLength={64} autoFocus value={label} onChange={(e) => setLabel(e.target.value)} />
            <button type="submit" className="button button-secondary">
              {t.devices.save}
            </button>
          </form>
        ) : (
          <h3 className="device-name">{name}</h3>
        )}
        <p className="small muted">{t.devices.lastUsed.replace("{when}", when(device.lastUsed))}</p>
        <div className="device-actions">
          <Link to={`/d/${device.deviceId}`} className="button button-primary">
            {t.devices.connect}
          </Link>
          <button
            type="button"
            className="button button-ghost"
            aria-pressed={device.favorite}
            onClick={() => patch({ favorite: !device.favorite })}
          >
            {device.favorite ? `★ ${t.devices.unfavorite}` : `☆ ${t.devices.favorite}`}
          </button>
          <button type="button" className="button button-ghost" onClick={() => setRenaming((r) => !r)}>
            {t.devices.rename}
          </button>
          {forgetting ? (
            <button
              type="button"
              className="button button-danger"
              onClick={() => {
                removeDevice(store, device.deviceId, device.signalUrl);
                onChange();
              }}
            >
              {t.devices.forgetConfirm}
            </button>
          ) : (
            <button type="button" className="button button-ghost" onClick={() => setForgetting(true)}>
              {t.devices.forget}
            </button>
          )}
        </div>
        {forgetting && <p className="small muted">{t.devices.forgetHint}</p>}
      </div>
    </li>
  );
}

function RecentCard({ recent, linked }: { recent: RecentSession; linked: boolean }) {
  const name = recent.name || (recent.target.length === 9 ? formatCode(recent.target) : t.devices.unnamed);
  const to = recent.deviceId ? (linked ? `/d/${recent.deviceId}` : "") : `/c/${recent.target}`;
  return (
    <li className="device-card">
      <Thumb src={recent.thumb} label={name} />
      <div className="device-body">
        <h3 className="device-name">{name}</h3>
        <p className="small muted">{when(recent.at)}</p>
        {to && (
          <div className="device-actions">
            <Link to={to} className="button button-secondary">
              {recent.deviceId ? t.devices.connect : t.devices.again}
            </Link>
          </div>
        )}
      </div>
    </li>
  );
}
