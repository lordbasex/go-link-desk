// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useMemo } from "react";
import { Link, useParams } from "react-router-dom";
import { findDevice, removeDevice, updateDevice, type SavedDevice, type SignalClient } from "@go-link-desk/shared";
import { t } from "../i18n";
import type { Door } from "../session/session";
import { safeStorage, signalUrl } from "../session/useSession";
import { useRecent } from "./SessionPage";
import { SessionRunner } from "./SessionRunner";

/** /d/<device_id>: back to a linked computer, without a code. */
export function DevicePage({ createClient }: { createClient?: () => SignalClient }) {
  const { deviceId = "" } = useParams();
  const url = signalUrl();
  const device = useMemo(() => findDevice(safeStorage(), deviceId, url), [deviceId, url]);
  if (!device) {
    return (
      <section className="join">
        <h1 className="join-title">{t.devices.title}</h1>
        <p className="notice">{t.devices.notLinked}</p>
        <div className="join-row">
          <Link to="/link" className="button button-primary">
            {t.devices.link}
          </Link>
          <Link to="/web" className="button button-secondary">
            {t.devices.title}
          </Link>
        </div>
      </section>
    );
  }
  return <ReachSession device={device} createClient={createClient} />;
}

export function deviceName(d: SavedDevice): string {
  return d.label || d.name || t.devices.unnamed;
}

function ReachSession({ device, createClient }: { device: SavedDevice; createClient?: () => SignalClient }) {
  const door = useMemo<Door>(() => ({ kind: "reach", device }), [device]);
  const recent = useRecent("", device.deviceId);
  const store = safeStorage();
  return (
    <SessionRunner
      door={door}
      title={t.session.nameTitle.replace("{code}", deviceName(device))}
      back={{ to: "/web", label: t.devices.title }}
      autoStart
      onUnlinked={() => removeDevice(store, device.deviceId, device.signalUrl)}
      onBound={() => updateDevice(store, device.deviceId, device.signalUrl, { bound: true })}
      onHost={(h) => {
        updateDevice(store, device.deviceId, device.signalUrl, { name: h.name, os: h.os, terminal: h.terminal });
        recent.host(h);
      }}
      onLive={() => {
        updateDevice(store, device.deviceId, device.signalUrl, { lastUsed: Date.now() });
        recent.live();
      }}
      onSnapshot={(thumb) => {
        updateDevice(store, device.deviceId, device.signalUrl, { thumb });
        recent.snapshot(thumb);
      }}
      createClient={createClient}
    />
  );
}
