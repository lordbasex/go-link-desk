// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import { describe, expect, it } from "vitest";
import { clearSavedLink, loadSavedLink, parseAuthOk, saveLink } from "../src";

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

const link = {
  deviceId: "c2c5f76f-1a25-45d8-abe7-91f51d11f75d",
  linkId: "3f9c0a1b2c3d4e5f",
  token: "q8VY3n1s0bPq0n8oJ1m3Zr2yX7eW5vT4uS6rQ9pO0nM", // gitleaks:allow (test value)
  signalUrl: "wss://signal.example.com/ws",
  savedAt: 1,
};

describe("saved device link", () => {
  it("round trips for the same signaling server only", () => {
    const s = memoryStorage();
    saveLink(s, link);
    expect(loadSavedLink(s, link.signalUrl)).toEqual(link);
    expect(loadSavedLink(s, "wss://other.example.com/ws")).toBeNull();
    clearSavedLink(s);
    expect(loadSavedLink(s, link.signalUrl)).toBeNull();
  });

  it("rejects tampered values", () => {
    const s = memoryStorage();
    s.setItem("go-link-desk.device-link", JSON.stringify({ ...link, deviceId: "../../x" }));
    expect(loadSavedLink(s, link.signalUrl)).toBeNull();
    s.setItem("go-link-desk.device-link", "not json");
    expect(loadSavedLink(s, link.signalUrl)).toBeNull();
  });

  it("parses auth_ok", () => {
    expect(parseAuthOk({ type: "auth_ok", device_id: "d", link_id: "l", token: "t" })?.token).toBe("t");
    expect(parseAuthOk({ type: "auth_ok", device_id: "d", link_id: "l" })?.token).toBeUndefined();
    expect(parseAuthOk({ type: "device_status" })).toBeNull();
  });
});
