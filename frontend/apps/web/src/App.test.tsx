// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
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
});

describe("App", () => {
  it("shows the landing with the way in", () => {
    renderAt("/");
    expect(screen.getByRole("heading", { level: 1, name: en.landing.title })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: en.landing.connect })).toHaveAttribute("href", "/c");
  });

  it("opens the session of a typed code", async () => {
    const user = userEvent.setup();
    renderAt("/c");
    await user.type(screen.getByLabelText(en.join.codeLabel), "915355636");
    expect(screen.getByLabelText(en.join.codeLabel)).toHaveValue("915 355 636");
    await user.click(screen.getByRole("button", { name: en.join.connect }));
    expect(screen.getByRole("heading", { name: en.session.title })).toBeInTheDocument();
    expect(screen.getByText(/915 355 636/)).toBeInTheDocument();
  });

  it("refuses what is not a code", async () => {
    const user = userEvent.setup();
    renderAt("/c");
    await user.type(screen.getByLabelText(en.join.codeLabel), "12345");
    await user.click(screen.getByRole("button", { name: en.join.connect }));
    expect(screen.getByRole("alert")).toHaveTextContent(en.join.invalid);
  });

  it("reads a pasted link", async () => {
    const user = userEvent.setup();
    renderAt("/c");
    await user.click(screen.getByLabelText(en.join.codeLabel));
    await user.paste("https://desk.go-link.org/c/AbCdEfGhIjKlMnOpQrStUv");
    await user.click(screen.getByRole("button", { name: en.join.connect }));
    expect(screen.getByText(/AbCdEfGhIjKlMnOpQrStUv/)).toBeInTheDocument();
  });

  it("sends a bad session address back to the form", () => {
    renderAt("/c/nope");
    expect(screen.getByRole("heading", { name: en.join.title })).toBeInTheDocument();
  });

  it("switches language", () => {
    setLang("es");
    renderAt("/");
    expect(screen.getByRole("heading", { level: 1, name: es.landing.title })).toBeInTheDocument();
  });
});

describe("translations", () => {
  // Every language has the same keys (the type checks it) and the same
  // number of landing steps.
  it("have every landing step", () => {
    expect(es.landing.steps).toHaveLength(en.landing.steps.length);
    expect(pt.landing.steps).toHaveLength(en.landing.steps.length);
  });
});
