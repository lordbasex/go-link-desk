// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { afterEach, describe, expect, it } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { addRecent, loadDevices, saveDevice, type SavedDevice } from "@go-link-desk/shared";
import { en } from "../i18n/en";
import { FakeClient } from "../session/test-utils";
import { signalUrl } from "../session/useSession";
import { DevicePage } from "./DevicePage";
import { LinkPage } from "./LinkPage";
import { WebHomePage } from "./WebHomePage";

const device = (): SavedDevice => ({
  deviceId: "c2c5f76f-1a25-45d8-abe7-91f51d11f75d",
  linkId: "3f9c0a1b2c3d4e5f",
  hostKey: "BI9-mKWqJqRvAlOAtDzOTQYVD-MI1LAlIWwc0F-ZZ9fLSe83_vmxFW8JdzizDGAXfM1CoJ0Vyv4QtpeGLYpNxKQ",
  signalUrl: signalUrl(),
  name: "mac-studio",
  label: "",
  os: "darwin",
  favorite: false,
  savedAt: 1,
  lastUsed: 1,
  thumb: "",
});

function at(path: string, client?: FakeClient) {
  const create = client ? () => client.asClient() : undefined;
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/web" element={<WebHomePage />} />
        <Route path="/d/:deviceId" element={<DevicePage createClient={create} />} />
        <Route path="/link" element={<LinkPage createClient={create} />} />
        <Route path="/link/:code" element={<LinkPage createClient={create} />} />
      </Routes>
    </MemoryRouter>,
  );
}

afterEach(() => {
  cleanup();
  window.localStorage.clear();
});

describe("My computers", () => {
  it("lists the linked computers and the recent sessions on Home", async () => {
    const user = userEvent.setup();
    saveDevice(window.localStorage, device());
    addRecent(window.localStorage, { target: "915355636", deviceId: "", name: "", os: "", at: 5, thumb: "" });
    at("/web");
    expect(screen.getByRole("link", { name: en.web.connectTo.replace("{name}", "mac-studio") })).toHaveAttribute("href", `/d/${device().deviceId}`);
    expect(screen.getByRole("link", { name: en.web.connectTo.replace("{name}", "915 355 636") })).toHaveAttribute("href", "/c/915355636");

    await user.click(screen.getByRole("button", { name: en.web.more.replace("{name}", "mac-studio") }));
    await user.click(screen.getByRole("button", { name: `☆ ${en.devices.favorite}` }));
    expect(loadDevices(window.localStorage)[0]?.favorite).toBe(true);

    await user.click(screen.getByRole("button", { name: en.devices.rename }));
    const input = screen.getByRole("textbox", { name: en.devices.rename });
    await user.clear(input);
    await user.type(input, "Office{Enter}");
    expect(screen.getByRole("link", { name: en.web.connectTo.replace("{name}", "Office") })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: en.web.forget }));
    await user.click(screen.getByRole("button", { name: en.web.forgetConfirm }));
    expect(loadDevices(window.localStorage)).toEqual([]);
    expect(screen.getByText(en.web.computersEmpty)).toBeInTheDocument();
  });

  it("goes back to a linked computer without a code, and says when unattended access is off", async () => {
    window.localStorage.setItem("go-link-desk.name", "Ana");
    saveDevice(window.localStorage, device());
    const client = new FakeClient();
    client.joinReply = { type: "paired", remote: "H", session_id: "S" };
    at(`/d/${device().deviceId}`, client);
    await screen.findByText(en.session.waiting);
    expect(client.sent[0]).toMatchObject({ type: "reach", device_id: device().deviceId });
    // The control channel is the stream's; the page reacts to what it says.
    act(() => client.deliver({ type: "signal", from: "H", payload: { kind: "rejected", reason: "off" } }));
    expect(await screen.findByText(en.session.rejected.off)).toBeInTheDocument();
  });

  it("says when this browser is not linked to that computer", () => {
    at("/d/11111111-1111-4111-8111-111111111111");
    expect(screen.getByText(en.devices.notLinked)).toBeInTheDocument();
  });

  it("asks for a 9-digit linking code, then claims it", async () => {
    const user = userEvent.setup();
    const client = new FakeClient();
    client.joinReply = { type: "paired", remote: "H", session_id: "S" };
    at("/link", client);
    await user.type(screen.getByLabelText(en.link.codeLabel), "12345");
    await user.click(screen.getByRole("button", { name: en.link.next }));
    expect(screen.getByText(en.link.invalid)).toBeInTheDocument();
    await user.type(screen.getByLabelText(en.link.codeLabel), "6789");
    await user.click(screen.getByRole("button", { name: en.link.next }));
    await user.type(await screen.findByLabelText(en.session.nameLabel), "Ana");
    await user.click(screen.getByRole("button", { name: en.session.connect }));
    expect(await screen.findByText(en.link.waiting)).toBeInTheDocument();
    expect(client.sent[0]).toEqual({ type: "claim", app: "go-link-desk", code: "123456789" });
  });
});
