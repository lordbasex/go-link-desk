// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
// The shape of the user documentation (/docs). Each language file
// (docs-en.ts, docs-es.ts, docs-pt.ts) is one Docs value with the same
// pages and slugs. Inline text accepts `code`, **bold** and [links](/path).

export type DocBlock =
  | { readonly t: "p"; readonly text: string }
  | { readonly t: "h2"; readonly id: string; readonly text: string }
  | { readonly t: "list"; readonly items: readonly string[] }
  | { readonly t: "steps"; readonly items: readonly string[] }
  | { readonly t: "code"; readonly code: string }
  | { readonly t: "table"; readonly head: readonly string[]; readonly rows: readonly (readonly string[])[] }
  | { readonly t: "note"; readonly tone: "info" | "warn"; readonly text: string }
  | { readonly t: "tabs"; readonly label: string; readonly tabs: readonly DocTab[] };

/**
 * One tab of a tabs block. `os` marks the tab for a system, so the page
 * opens on the visitor's own (macos, windows, linux).
 */
export interface DocTab {
  readonly id: string;
  readonly label: string;
  readonly os?: "macos" | "windows" | "linux" | "ios" | "android";
  readonly blocks: readonly DocBlock[];
}

export interface DocPage {
  readonly slug: string;
  readonly group: string;
  readonly title: string;
  readonly lead: string;
  readonly blocks: readonly DocBlock[];
}

export interface Docs {
  readonly title: string;
  readonly eyebrow: string;
  readonly search: string;
  readonly noResults: string;
  readonly onThisPage: string;
  readonly previous: string;
  readonly next: string;
  readonly menu: string;
  readonly copy: string;
  readonly edit: string;
  readonly pages: readonly DocPage[];
}
