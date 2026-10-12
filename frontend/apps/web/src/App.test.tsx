// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { resetAnalyticsForTests } from "./analytics";
import { App } from "./App";
import { setLang } from "./i18n";
import { en } from "./i18n/en";
import { es } from "./i18n/es";
import { pt } from "./i18n/pt";

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <App />
    </MemoryRouter>,
  );
}

afterEach(() => {
  cleanup();
  setLang("en");
  window.localStorage.removeItem("go-link-desk.consent");
  resetAnalyticsForTests();
});

describe("App", () => {
  it("asks about cookies on the public pages only, and remembers the answer", async () => {
    const user = userEvent.setup();
    renderAt("/web");
    expect(screen.queryByRole("region", { name: en.consent.title })).toBeNull();
    cleanup();
    renderAt("/");
    expect(screen.getByRole("region", { name: en.consent.title })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: en.consent.reject }));
    expect(screen.queryByRole("region", { name: en.consent.title })).toBeNull();
    expect(window.localStorage.getItem("go-link-desk.consent")).toBe("denied");
    expect(document.querySelector('script[src*="googletagmanager"]')).toBeNull();
    await user.click(screen.getByRole("button", { name: en.consent.settings }));
    expect(screen.getByRole("region", { name: en.consent.title })).toBeInTheDocument();
  });

  it("shows the landing with the four ways in", () => {
    renderAt("/");
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(en.site.hero.titleEm);
    expect(screen.getByRole("heading", { name: en.site.ways.web.title })).toBeInTheDocument();
    expect(screen.getAllByRole("heading", { name: en.site.ways.android.title }).length).toBeGreaterThan(0);
    expect(screen.getByRole("link", { name: en.site.nav.openWeb })).toHaveAttribute("href", "/web");
  });

  it("opens the session of a code typed on the landing", async () => {
    const user = userEvent.setup();
    renderAt("/");
    await user.type(screen.getByLabelText(en.site.hero.codeLabel), "915355636");
    expect(screen.getByLabelText(en.site.hero.codeLabel)).toHaveValue("915 355 636");
    await user.click(screen.getByRole("button", { name: en.web.connect }));
    expect(screen.getByRole("heading", { name: en.session.nameTitle.replace("{code}", "915 355 636") })).toBeInTheDocument();
  });

  it("opens the session of a code typed on Home (/web)", async () => {
    const user = userEvent.setup();
    renderAt("/web");
    const connect = screen.getByRole("button", { name: en.web.connect });
    expect(connect).toBeDisabled();
    await user.type(screen.getByLabelText(en.web.codeLabel), "915355636");
    expect(screen.getByLabelText(en.web.codeLabel)).toHaveValue("915 355 636");
    await user.click(connect);
    expect(screen.getByRole("heading", { name: en.session.nameTitle.replace("{code}", "915 355 636") })).toBeInTheDocument();
    expect(screen.getByLabelText(en.session.nameLabel)).toBeInTheDocument();
  });

  it("reads a pasted link on Home", async () => {
    const user = userEvent.setup();
    renderAt("/web");
    await user.click(screen.getByLabelText(en.web.codeLabel));
    await user.paste("https://desk.go-link.org/c/AbCdEfGhIjKlMnOpQrStUv");
    await user.click(screen.getByRole("button", { name: en.web.connect }));
    expect(screen.getByText(/AbCdEfGhIjKlMnOpQrStUv/)).toBeInTheDocument();
  });

  it("leads the old ways in and a bad session address to Home", () => {
    renderAt("/c");
    expect(screen.getByLabelText(en.web.codeLabel)).toBeInTheDocument();
    cleanup();
    renderAt("/devices");
    expect(screen.getByRole("heading", { name: en.web.myComputers })).toBeInTheDocument();
    cleanup();
    renderAt("/c/nope");
    expect(screen.getByLabelText(en.web.codeLabel)).toBeInTheDocument();
  });

  it("switches language", async () => {
    const user = userEvent.setup();
    renderAt("/");
    await user.click(screen.getByRole("button", { name: "ES" }));
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(es.site.hero.titleEm);
  });
});

describe("translations", () => {
  // Every language has the same keys (the type checks it) and the same
  // number of items in each list.
  it("have every list item", () => {
    for (const m of [es, pt]) {
      expect(m.site.steps.items).toHaveLength(en.site.steps.items.length);
      expect(m.site.faq.items).toHaveLength(en.site.faq.items.length);
      expect(m.site.hero.notes).toHaveLength(en.site.hero.notes.length);
      expect(m.site.security.cards).toHaveLength(en.site.security.cards.length);
      expect(m.site.features.picture.items).toHaveLength(en.site.features.picture.items.length);
      expect(m.site.features.support.items).toHaveLength(en.site.features.support.items.length);
    }
  });
});
