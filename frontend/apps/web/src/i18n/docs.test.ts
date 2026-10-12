// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { describe, expect, it } from "vitest";
import { docsEn } from "./docs-en";
import { docsEs } from "./docs-es";
import { docsPt } from "./docs-pt";
import type { DocBlock, Docs } from "./docs-types";

const all: Record<string, Docs> = { en: docsEn, es: docsEs, pt: docsPt };

/** Every inline text of some blocks, tabs included. */
function texts(blocks: readonly DocBlock[]): string[] {
  return blocks.flatMap((b) => {
    switch (b.t) {
      case "p":
      case "note":
        return [b.text];
      case "list":
      case "steps":
        return [...b.items];
      case "table":
        return b.rows.flat();
      case "tabs":
        return b.tabs.flatMap((t) => texts(t.blocks));
      default:
        return [];
    }
  });
}

/** The h2 ids of some blocks, tabs included. */
function ids(blocks: readonly DocBlock[]): string[] {
  return blocks.flatMap((b) => (b.t === "h2" ? [b.id] : b.t === "tabs" ? b.tabs.flatMap((t) => ids(t.blocks)) : []));
}

describe("the docs", () => {
  it("have the same pages and sections in every language", () => {
    const slugs = docsEn.pages.map((p) => p.slug);
    for (const [lang, docs] of Object.entries(all)) {
      expect(docs.pages.map((p) => p.slug), lang).toEqual(slugs);
      docs.pages.forEach((p, i) => expect(ids(p.blocks), `${lang} ${p.slug}`).toEqual(ids(docsEn.pages[i]!.blocks)));
    }
  });

  it("link only to pages that exist", () => {
    const slugs = new Set(docsEn.pages.map((p) => p.slug));
    const routes = new Set(["/web", "/privacy", "/"]);
    for (const [lang, docs] of Object.entries(all)) {
      for (const p of docs.pages) {
        for (const text of [p.lead, ...texts(p.blocks)]) {
          for (const m of text.matchAll(/\]\((\/[^)\s]*)\)/g)) {
            const to = m[1]!;
            const ok = to.startsWith("/docs/") ? slugs.has(to.slice(6).split("#")[0]!) : routes.has(to);
            expect(ok, `${lang} ${p.slug}: ${to}`).toBe(true);
          }
        }
      }
    }
  });
});
