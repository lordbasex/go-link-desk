// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { en } from "../i18n/en";
import { nativeFragment, type Report, type runTest } from "../network/probe";
import { NetworkPage } from "./NetworkPage";

afterEach(() => {
  cleanup();
  window.location.hash = "";
});

const show = (test: typeof runTest) =>
  render(
    <MemoryRouter>
      <NetworkPage test={test} />
    </MemoryRouter>,
  );

/** A test that tells every step and ends with this report. */
const fake = (report: Report): typeof runTest =>
  vi.fn(async (_url, native, onStep) => {
    const r = { ...report, ...(native ? { native } : {}) };
    for (const id of ["signal", "lan", "routers", "udp", "public", "nat", "relay"] as const) {
      onStep({ id, state: "run", report: r });
      onStep({ id, state: "ok", report: r });
    }
    return r;
  });

describe("NetworkPage", () => {
  it("runs the test and says the connection can go direct", async () => {
    const test = fake({ signalOk: true, signalMs: 140, udp: true, ipv6: false, mapping: "independent", relayOk: true, relayMs: 143, relayProtocol: "udp", publicIP: "181.23.153.151" });
    show(test);
    await userEvent.click(screen.getByRole("button", { name: en.network.start }));
    expect(await screen.findByText(en.network.result.direct.title)).toBeTruthy();
    expect(screen.getAllByText("181.23.153.151").length).toBeGreaterThan(0);
    expect(screen.getByText("UDP · 143 ms")).toBeTruthy();
    expect(screen.getByText(en.network.values.hidden)).toBeTruthy();
  });

  it("starts by itself with an app's data and shows the double router guide", async () => {
    window.location.hash = nativeFragment({ src: "desktop", lan: "192.168.0.12", wan: "192.168.1.2" });
    const test = fake({ signalOk: true, signalMs: 140, udp: true, ipv6: false, mapping: "independent", relayOk: true, relayMs: 143, publicIP: "181.23.153.151" });
    show(test);
    expect(await screen.findByText(en.network.result.maybe.title)).toBeTruthy();
    expect(test).toHaveBeenCalledOnce();
    expect(screen.getAllByText("192.168.0.12").length).toBeGreaterThan(0);
    expect(screen.getByText(en.network.notes.routers)).toBeTruthy();
    expect(screen.getByRole("tab", { name: en.network.fix.tabs.double }).getAttribute("aria-selected")).toBe("true");
    expect(document.querySelector(".nt-rule")?.textContent).toContain("192.168.1.2"); // the DMZ host in the rule
  });

  it("finds the provider's modem in the hops and links its page", async () => {
    window.location.hash = nativeFragment({ src: "desktop", lan: "172.17.0.5", gw: "172.17.0.1", hops: ["172.17.0.1", "192.168.1.1", "200.51.241.1"] });
    show(fake({ signalOk: true, signalMs: 140, udp: true, ipv6: false, mapping: "independent", relayOk: true, relayMs: 143, publicIP: "181.23.153.151" }));
    expect(await screen.findByText(en.network.result.maybe.title)).toBeTruthy();
    expect(document.querySelector(".nt-rule")?.textContent).toContain("http://192.168.1.1");
    expect(screen.getAllByText(en.network.nodes.outer).length).toBeGreaterThan(0);
  });

  it("says when go-link Desk's server cannot be reached", async () => {
    show(fake({ signalOk: false, udp: false, ipv6: false, mapping: "unknown", relayOk: false }));
    await userEvent.click(screen.getByRole("button", { name: en.network.start }));
    expect(await screen.findByText(en.network.error)).toBeTruthy();
  });
});
