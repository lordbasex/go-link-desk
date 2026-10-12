// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { describe, expect, it } from "vitest";
import { browserLang } from "./index";

describe("browserLang", () => {
  it("takes Spanish and Portuguese from the browser, English otherwise", () => {
    expect(browserLang(["es-AR", "en-US"])).toBe("es");
    expect(browserLang(["pt-BR"])).toBe("pt");
    expect(browserLang(["fr-FR", "es"])).toBe("es");
    expect(browserLang(["de-DE", "fr"])).toBe("en");
    expect(browserLang(["en-GB", "es"])).toBe("en");
    expect(browserLang([])).toBe("en");
  });
});
