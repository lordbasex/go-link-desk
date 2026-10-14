// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { en } from "../i18n/en";
import { FakeClient } from "../session/test-utils";
import { SessionPage } from "./SessionPage";

function renderSession(client: FakeClient) {
  const create = () => client.asClient();
  return render(
    <MemoryRouter initialEntries={["/c/915355636"]}>
      <Routes>
        <Route path="/c/:target" element={<SessionPage createClient={create} />} />
        <Route path="/c" element={<p>form</p>} />
        <Route path="/link/:code" element={<p>linking 915355636</p>} />
      </Routes>
    </MemoryRouter>,
  );
}

afterEach(() => {
  cleanup();
  window.localStorage.clear();
});

describe("SessionPage", () => {
  it("asks for a name, knocks with it and waits", async () => {
    const user = userEvent.setup();
    const client = new FakeClient();
    renderSession(client);
    expect(client.connected).toBe(false); // nothing happens before Connect
    await user.type(screen.getByLabelText(en.session.nameLabel), "Ana");
    await user.click(screen.getByRole("button", { name: en.session.connect }));
    expect(await screen.findByText(en.session.waiting)).toBeInTheDocument();
    expect(client.signals()[0]).toMatchObject({ kind: "knock", name: "Ana" });
    expect(window.localStorage.getItem("go-link-desk.name")).toBe("Ana");

    act(() => client.deliver({ type: "signal", from: "H", payload: { kind: "rejected", reason: "declined" } }));
    expect(await screen.findByText(en.session.rejected.declined)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: en.session.retry })).toBeInTheDocument();
  });

  it("shows the screen once accepted, and ends when the host says so", async () => {
    const user = userEvent.setup();
    const client = new FakeClient();
    renderSession(client);
    await user.click(screen.getByRole("button", { name: en.session.connect }));
    await screen.findByText(en.session.waiting);
    act(() => client.deliver({ type: "signal", from: "H", payload: { kind: "accepted", permissions: { view: true } } }));
    expect(await screen.findByRole("button", { name: en.session.disconnect })).toBeInTheDocument();
    expect(screen.getByText(en.session.connecting)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: en.session.disconnect }));
    expect(await screen.findByText(en.session.ended.viewer)).toBeInTheDocument();
    await vi.waitFor(() => expect(client.closed).toBe(true));
  });

  it("tries a code that is no session's as a linking code (one box for both)", async () => {
    const user = userEvent.setup();
    const client = new FakeClient();
    client.joinReply = "invalid or expired code";
    renderSession(client);
    await user.click(screen.getByRole("button", { name: en.session.connect }));
    expect(await screen.findByText("linking 915355636")).toBeInTheDocument();
    expect(screen.queryByText(en.session.notFound)).toBeNull();
  });
});
