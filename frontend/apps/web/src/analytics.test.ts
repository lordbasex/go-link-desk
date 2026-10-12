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

  it("rejecting turns it off and deletes Desk's Google cookies only", () => {
    // The test page's real document, which keeps cookies.
    const win = { document, location: window.location } as unknown as W;
    document.cookie = "desk__ga=GA1.1.1; path=/";
    document.cookie = `desk__ga_${GA_ID.slice(2)}=GS1.1; path=/`;
    document.cookie = "_ga=GA1.1.2; path=/";
    expect(document.cookie).toContain("desk__ga=");
    stopAnalytics(win);
    expect((win as unknown as Record<string, unknown>)[`ga-disable-${GA_ID}`]).toBe(true);
    expect(document.cookie).not.toContain("desk__ga");
    expect(document.cookie).toContain("_ga=GA1.1.2");
    document.cookie = "_ga=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/";
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
