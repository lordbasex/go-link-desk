// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import { describe, expect, it } from "vitest";
import { checkSignalUrl, clearCustomSignalUrl, resolveSignalUrl, saveCustomSignalUrl, SIGNAL_URL_STORAGE_KEY } from "../src";

const OFFICIAL = "wss://signal.go-link.org/ws";

describe("checkSignalUrl", () => {
  it("accepts wss and loopback ws", () => {
    expect(checkSignalUrl("wss://signal.example.com/ws")).toEqual({ ok: true, url: "wss://signal.example.com/ws" });
    expect(checkSignalUrl(" ws://127.0.0.1:8090/ws ").ok).toBe(true);
    expect(checkSignalUrl("ws://localhost:8090/ws").ok).toBe(true);
  });
  it("explains what is wrong", () => {
    expect(checkSignalUrl("not a url")).toEqual({ ok: false, problem: "format" });
    expect(checkSignalUrl("https://signal.example.com/ws")).toEqual({ ok: false, problem: "scheme" });
    expect(checkSignalUrl("ws://signal.example.com/ws")).toEqual({ ok: false, problem: "insecure" });
  });
});

describe("resolveSignalUrl", () => {
  it("uses the official server by default", () => {
    localStorage.clear();
    expect(resolveSignalUrl(OFFICIAL, localStorage)).toEqual({ url: OFFICIAL, custom: false });
  });
  it("uses a saved custom server until it is cleared", () => {
    saveCustomSignalUrl("wss://mine.example.com/ws", localStorage);
    expect(resolveSignalUrl(OFFICIAL, localStorage)).toEqual({ url: "wss://mine.example.com/ws", custom: true });
    clearCustomSignalUrl(localStorage);
    expect(resolveSignalUrl(OFFICIAL, localStorage).custom).toBe(false);
  });
  it("ignores tampered storage", () => {
    localStorage.setItem(SIGNAL_URL_STORAGE_KEY, "ws://evil.example.com/ws");
    expect(resolveSignalUrl(OFFICIAL, localStorage)).toEqual({ url: OFFICIAL, custom: false });
    localStorage.clear();
  });
});
