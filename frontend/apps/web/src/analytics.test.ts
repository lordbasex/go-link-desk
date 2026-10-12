// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  GA_ID,
  analyticsLoaded,
  guardHistory,
  isPublicPath,
  resetAnalyticsForTests,
  setConsent,
  startAnalytics,
  stopAnalytics,
} from "./analytics";

/** A window with its own location and history, so navigation can be watched. */
function fakeWindow(path: string) {
  const doc = document.implementation.createHTMLDocument("t");
  const pushed: string[] = [];
  const win = {
    document: doc,
    location: {
      pathname: path,
      href: `https://desk.go-link.org${path}`,
      assign: vi.fn(),
      reload: vi.fn(),
    },
    history: {
      pushState: (_d: unknown, _u: string, url?: string | URL | null) => pushed.push(String(url)),
      replaceState: (_d: unknown, _u: string, url?: string | URL | null) => pushed.push(String(url)),
    },
    listeners: {} as Record<string, () => void>,
    addEventListener(type: string, fn: () => void) {
      this.listeners[type] = fn;
    },
  };
  return { win, pushed };
}
type W = Parameters<typeof startAnalytics>[0];

afterEach(() => {
  window.localStorage.clear();
  resetAnalyticsForTests();
});

describe("analytics", () => {
  it("counts only the public pages", () => {
    for (const p of ["/", "/docs", "/docs/install", "/privacy"]) expect(isPublicPath(p)).toBe(true);
    for (const p of ["/web", "/c/915355636", "/link/abc", "/d/xyz", "/docs-x", "/privacy/x"]) expect(isPublicPath(p)).toBe(false);
  });

  it("loads nothing without consent, nor on an app page", () => {
    const home = fakeWindow("/");
    startAnalytics(home.win as unknown as W);
    expect(home.win.document.querySelector("script")).toBeNull();

    setConsent("granted");
    const session = fakeWindow("/c/915355636");
    startAnalytics(session.win as unknown as W);
    expect(session.win.document.querySelector("script")).toBeNull();
    expect(analyticsLoaded()).toBe(false);
  });

  it("loads gtag.js once after Accept on a public page", () => {
    setConsent("granted");
    expect(window.localStorage.getItem("go-link-desk.consent")).toBe("granted");
    const home = fakeWindow("/docs");
    startAnalytics(home.win as unknown as W);
    startAnalytics(home.win as unknown as W);
    const scripts = home.win.document.querySelectorAll("script");
    expect(scripts).toHaveLength(1);
    expect(scripts[0]?.src).toBe(`https://www.googletagmanager.com/gtag/js?id=${GA_ID}`);
    expect(analyticsLoaded()).toBe(true);
  });

  it("rejecting turns it off and deletes Google's cookies", () => {
    const home = fakeWindow("/");
    home.win.document.cookie = "_ga=GA1.1.1; path=/";
    home.win.document.cookie = "lang=es; path=/";
    stopAnalytics(home.win as unknown as W);
    expect((home.win as unknown as Record<string, unknown>)[`ga-disable-${GA_ID}`]).toBe(true);
    expect(home.win.document.cookie).not.toContain("_ga");
  });

  it("leaves for an app page with a full page load once Google's script is in", () => {
    const { win, pushed } = fakeWindow("/");
    guardHistory(win as unknown as W);
    win.history.pushState(null, "", "/docs");
    expect(pushed).toEqual(["/docs"]);

    setConsent("granted");
    startAnalytics(win as unknown as W);
    win.history.pushState(null, "", "/docs/install");
    win.history.pushState(null, "", "/c/915355636");
    expect(pushed).toEqual(["/docs", "/docs/install"]);
    expect(win.location.assign).toHaveBeenCalledWith("/c/915355636");
    expect((win as unknown as Record<string, unknown>)[`ga-disable-${GA_ID}`]).toBe(true);
  });

  it("going back to an app page reloads it without Google", () => {
    const { win } = fakeWindow("/");
    guardHistory(win as unknown as W);
    setConsent("granted");
    startAnalytics(win as unknown as W);
    win.location.pathname = "/web";
    win.listeners.popstate?.();
    expect(win.location.reload).toHaveBeenCalled();
  });
});
