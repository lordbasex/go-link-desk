// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { en } from "../i18n/en";
import { DemoPage, demoPath, type DemoCode } from "./DemoPage";
import { WebHomePage } from "./WebHomePage";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const show = (load: () => Promise<DemoCode>) =>
  render(
    <MemoryRouter>
      <DemoPage load={load} />
    </MemoryRouter>,
  );

describe("DemoPage", () => {
  it("shows the demo computer's code and a link on this site", async () => {
    show(async () => ({ online: true, code: "287 200 020", link: "https://desk.go-link.org/c/s7tR9tkyBRp0lv81CpHNDw", name: "go-link Desk Demo" }));
    expect(await screen.findByText("287 200 020")).toBeTruthy();
    const open = screen.getByRole("link", { name: en.demo.open });
    expect(open.getAttribute("href")).toBe("/c/s7tR9tkyBRp0lv81CpHNDw");
  });

  it("shows the pairing code and the password to link it", async () => {
    show(async () => ({ online: true, code: "287 200 020", link: "https://desk.go-link.org/c/s7tR9tkyBRp0lv81CpHNDw", pairing: "113 134 323", password: "demo-desk-2026" }));
    expect(await screen.findByText("113 134 323")).toBeTruthy();
    expect(screen.getByText("demo-desk-2026")).toBeTruthy();
    expect(screen.getByText(en.demo.linkTitle)).toBeTruthy();
  });

  it("says the computer is reconnecting while it has no code", async () => {
    show(async () => ({ online: false }));
    expect(await screen.findByText(en.demo.offline)).toBeTruthy();
  });

  it("says the same when the file cannot be read", async () => {
    show(() => Promise.reject(new Error("404")));
    expect(await screen.findByText(en.demo.offline)).toBeTruthy();
  });
});

describe("demoPath", () => {
  it("keeps only a well-formed invitation", () => {
    expect(demoPath("https://desk.go-link.org/c/s7tR9tkyBRp0lv81CpHNDw")).toBe("/c/s7tR9tkyBRp0lv81CpHNDw");
    expect(demoPath("https://evil.example/c/../x")).toBeUndefined();
    expect(demoPath(undefined)).toBeUndefined();
  });
});

describe("Try the demo on Home", () => {
  const home = () =>
    render(
      <MemoryRouter initialEntries={["/web"]}>
        <Routes>
          <Route path="/web" element={<WebHomePage />} />
          <Route path="/c/:target" element={<p>session page</p>} />
        </Routes>
      </MemoryRouter>,
    );

  it("opens the demo computer's session", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ online: true, code: "287 200 020", link: "https://desk.go-link.org/c/s7tR9tkyBRp0lv81CpHNDw" }))));
    home();
    await userEvent.click(screen.getByRole("button", { name: en.web.tryDemo }));
    expect(await screen.findByText("session page")).toBeTruthy();
  });

  it("says when the demo computer is not there", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ online: false }))));
    home();
    await userEvent.click(screen.getByRole("button", { name: en.web.tryDemo }));
    expect(await screen.findByText(en.web.demoUnavailable)).toBeTruthy();
  });
});
