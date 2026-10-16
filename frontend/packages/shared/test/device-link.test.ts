// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import { describe, expect, it } from "vitest";
import { MAX_RECENTS, addRecent, findDevice, loadDevices, loadRecents, parseAuthOk, removeDevice, saveDevice, updateDevice, type SavedDevice } from "../src";

function memoryStorage(): Storage {
  const m = new Map<string, string>();
  return {
    get length() {
      return m.size;
    },
    clear: () => m.clear(),
    getItem: (k) => m.get(k) ?? null,
    key: (i) => [...m.keys()][i] ?? null,
    removeItem: (k) => void m.delete(k),
    setItem: (k, v) => void m.set(k, v),
  };
}

const device: SavedDevice = {
  deviceId: "c2c5f76f-1a25-45d8-abe7-91f51d11f75d",
  linkId: "3f9c0a1b2c3d4e5f",
  hostKey: "BI9-mKWqJqRvAlOAtDzOTQYVD-MI1LAlIWwc0F-ZZ9fLSe83_vmxFW8JdzizDGAXfM1CoJ0Vyv4QtpeGLYpNxKQ", // a 65-byte P-256 point, base64url
  signalUrl: "wss://signal.example.com/ws",
  name: "mac-studio",
  label: "",
  os: "darwin",
  favorite: false,
  savedAt: 1,
  lastUsed: 1,
  thumb: "",
};

describe("linked computers", () => {
  it("round trip for their own signaling server", () => {
    const s = memoryStorage();
    saveDevice(s, device);
    expect(findDevice(s, device.deviceId, device.signalUrl)).toEqual(device);
    expect(findDevice(s, device.deviceId, "wss://other.example.com/ws")).toBeNull();
    removeDevice(s, device.deviceId, device.signalUrl);
    expect(loadDevices(s)).toEqual([]);
  });

  it("remember a terminal computer (go-link-desk-terminal), and only a real true", () => {
    const s = memoryStorage();
    saveDevice(s, device);
    updateDevice(s, device.deviceId, device.signalUrl, { terminal: true });
    expect(findDevice(s, device.deviceId, device.signalUrl)?.terminal).toBe(true);
    updateDevice(s, device.deviceId, device.signalUrl, { terminal: "yes" as unknown as boolean });
    expect(findDevice(s, device.deviceId, device.signalUrl)?.terminal).toBeUndefined();
  });

  it("list favorites first, then the latest used, and a new link replaces the old", () => {
    const s = memoryStorage();
    const b = { ...device, deviceId: "11111111-1111-4111-8111-111111111111", lastUsed: 5 };
    const c = { ...device, deviceId: "22222222-2222-4222-8222-222222222222", lastUsed: 9 };
    saveDevice(s, device);
    saveDevice(s, b);
    saveDevice(s, c);
    updateDevice(s, device.deviceId, device.signalUrl, { favorite: true, label: "Office" });
    expect(loadDevices(s).map((d) => d.deviceId)).toEqual([device.deviceId, c.deviceId, b.deviceId]);
    saveDevice(s, { ...b, linkId: "aaaaaaaaaaaaaaaa" });
    expect(loadDevices(s)).toHaveLength(3);
    expect(findDevice(s, b.deviceId, b.signalUrl)?.linkId).toBe("aaaaaaaaaaaaaaaa");
    expect(findDevice(s, device.deviceId, device.signalUrl)?.label).toBe("Office");
  });

  it("drop tampered entries and anything but a small JPEG thumbnail", () => {
    const s = memoryStorage();
    s.setItem("go-link-desk.devices", JSON.stringify([{ ...device, deviceId: "../../x" }, { ...device, thumb: "javascript:alert(1)" }, "junk"]));
    expect(loadDevices(s)).toEqual([device]);
    s.setItem("go-link-desk.devices", "not json");
    expect(loadDevices(s)).toEqual([]);
    updateDevice(s, device.deviceId, device.signalUrl, { thumb: "data:image/jpeg;base64,AAAA" });
    saveDevice(s, { ...device, thumb: "data:image/png;base64,AAAA" });
    expect(loadDevices(s)[0]?.thumb).toBe("");
  });
});

describe("recent sessions", () => {
  it("keep the newest, one per computer", () => {
    const s = memoryStorage();
    for (let i = 0; i < MAX_RECENTS + 3; i++) addRecent(s, { target: String(100000000 + i), deviceId: "", name: `pc${i}`, os: "darwin", at: i, thumb: "" });
    addRecent(s, { target: "", deviceId: device.deviceId, name: "mac", os: "darwin", at: 100, thumb: "" });
    addRecent(s, { target: "", deviceId: device.deviceId, name: "mac", os: "darwin", at: 200, thumb: "" });
    const r = loadRecents(s);
    expect(r).toHaveLength(MAX_RECENTS);
    expect(r[0]).toMatchObject({ deviceId: device.deviceId, at: 200 });
    expect(r.filter((x) => x.deviceId)).toHaveLength(1);
    addRecent(s, { target: "../x", deviceId: "", name: "", os: "", at: 300, thumb: "" });
    expect(loadRecents(s)[0]?.at).toBe(200);
  });
});

describe("auth_ok", () => {
  it("parses", () => {
    expect(parseAuthOk({ type: "auth_ok", device_id: "d", link_id: "l", host_key: "k" })?.host_key).toBe("k");
    expect(parseAuthOk({ type: "auth_ok", device_id: "d", link_id: "l" })?.host_key).toBeUndefined();
    expect(parseAuthOk({ type: "device_status" })).toBeNull();
  });
});
