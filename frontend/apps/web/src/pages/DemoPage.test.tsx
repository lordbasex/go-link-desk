// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { en } from "../i18n/en";
import { DemoPage, demoPath, type DemoCode } from "./DemoPage";

afterEach(cleanup);

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
