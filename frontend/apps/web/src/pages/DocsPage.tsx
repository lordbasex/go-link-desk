// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { Fragment, useEffect, useId, useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { Link, Navigate, useParams } from "react-router-dom";
import { useLang, type Lang } from "../i18n";
import type { DocBlock, DocPage, DocTab, Docs } from "../i18n/docs-types";
import { docsEn } from "../i18n/docs-en";
import { docsEs } from "../i18n/docs-es";
import { docsPt } from "../i18n/docs-pt";
import { REPO_URL } from "../config";
import { SiteFooter, SiteNav } from "./LandingPage";
import "../styles/landing.css";
import "../styles/docs.css";

const DOCS: Record<Lang, Docs> = { en: docsEn, es: docsEs, pt: docsPt };

/**
 * Renders inline text: `code`, **bold** and [links](/path). Internal links
 * use the router, external ones open in a new tab. Never raw HTML.
 */
function inline(text: string): ReactNode[] {
  const out: ReactNode[] = [];
  const re = /`([^`]+)`|\*\*([^*]+)\*\*|\[([^\]]+)\]\(([^)\s]+)\)/g;
  let last = 0;
  let key = 0;
  for (let m = re.exec(text); m; m = re.exec(text)) {
    if (m.index > last) out.push(text.slice(last, m.index));
    if (m[1] !== undefined) out.push(<code key={key++}>{m[1]}</code>);
    else if (m[2] !== undefined) out.push(<strong key={key++}>{m[2]}</strong>);
    else if (m[4]!.startsWith("/"))
      out.push(
        <Link key={key++} to={m[4]!}>
          {m[3]}
        </Link>,
      );
    else if (m[4]!.startsWith("https://"))
      out.push(
        <a key={key++} href={m[4]} target="_blank" rel="noopener">
          {m[3]}
        </a>,
      );
    else out.push(m[3]);
    last = re.lastIndex;
  }
  if (last < text.length) out.push(text.slice(last));
  return out;
}

/** Copies a code block, and says so for a moment. */
function CopyButton({ text, label }: { text: string; label: string }) {
  const [done, setDone] = useState(false);
  return (
    <button
      type="button"
      className="docs-copy"
      onClick={() => {
        void navigator.clipboard?.writeText(text).then(() => {
          setDone(true);
          window.setTimeout(() => setDone(false), 1500);
        });
      }}
    >
      {done ? "✓" : label}
    </button>
  );
}

function Block({ block, copy }: { block: DocBlock; copy: string }) {
  switch (block.t) {
    case "p":
      return <p>{inline(block.text)}</p>;
    case "h2":
      return (
        <h2 id={block.id}>
          <a href={`#${block.id}`} className="docs-anchor">
            {block.text}
          </a>
        </h2>
      );
    case "list":
      return (
        <ul>
          {block.items.map((item) => (
            <li key={item}>{inline(item)}</li>
          ))}
        </ul>
      );
    case "steps":
      return (
        <ol className="docs-steps">
          {block.items.map((item) => (
            <li key={item}>{inline(item)}</li>
          ))}
        </ol>
      );
    case "code":
      return (
        <div className="docs-code">
          {/* Focusable: a long line scrolls sideways with the keyboard too. */}
          <pre tabIndex={0}>
            <code>{block.code}</code>
          </pre>
          <CopyButton text={block.code} label={copy} />
        </div>
      );
    case "table":
      return (
        <div className="docs-table" tabIndex={0}>
          <table>
            <thead>
              <tr>
                {block.head.map((h) => (
                  <th key={h} scope="col">
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {block.rows.map((row) => (
                <tr key={row.join("|")}>
                  {row.map((cell, i) => (i === 0 ? <th key={i} scope="row">{inline(cell)}</th> : <td key={i}>{inline(cell)}</td>))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      );
    case "note":
      return <p className={`docs-note docs-note-${block.tone}`}>{inline(block.text)}</p>;
    case "tabs":
      return <Tabs label={block.label} tabs={block.tabs} copy={copy} />;
  }
}

/** The visitor's system, to open the matching tab first. */
function visitorOS(): DocTab["os"] {
  const ua = typeof navigator === "undefined" ? "" : navigator.userAgent;
  if (/iPhone|iPad/i.test(ua)) return "ios";
  if (/Android/i.test(ua)) return "android";
  if (/Windows/i.test(ua)) return "windows";
  if (/Mac OS X|Macintosh/i.test(ua) && !/iPhone|iPad/i.test(ua)) return "macos";
  if (/Linux/i.test(ua) && !/Android/i.test(ua)) return "linux";
  return undefined;
}

/**
 * Tabs, like a choice of system: arrows, Home and End move between them
 * (WAI-ARIA tabs pattern). The address's #tab opens that one.
 */
function Tabs({ label, tabs, copy }: { label: string; tabs: readonly DocTab[]; copy: string }) {
  const base = useId();
  const [selected, setSelected] = useState(() => {
    const hash = typeof window === "undefined" ? "" : window.location.hash.slice(1);
    const byHash = tabs.findIndex((tab) => tab.id === hash);
    if (byHash >= 0) return byHash;
    const os = visitorOS();
    return Math.max(0, tabs.findIndex((tab) => os !== undefined && tab.os === os));
  });
  const buttons = useRef<(HTMLButtonElement | null)[]>([]);
  const pick = (i: number) => {
    setSelected(i);
    buttons.current[i]?.focus();
  };
  const onKey = (e: KeyboardEvent) => {
    const last = tabs.length - 1;
    const next = { ArrowRight: selected + 1, ArrowLeft: selected - 1, Home: 0, End: last }[e.key];
    if (next === undefined) return;
    e.preventDefault();
    pick(next < 0 ? last : next > last ? 0 : next);
  };
  return (
    <div className="docs-tabs">
      <div role="tablist" aria-label={label} className="docs-tablist" onKeyDown={onKey}>
        {tabs.map((tab, i) => (
          <button
            key={tab.id}
            ref={(el) => {
              buttons.current[i] = el;
            }}
            type="button"
            role="tab"
            id={`${base}-tab-${tab.id}`}
            aria-selected={i === selected}
            aria-controls={`${base}-panel-${tab.id}`}
            tabIndex={i === selected ? 0 : -1}
            className="docs-tab"
            onClick={() => setSelected(i)}
          >
            {tab.label}
          </button>
        ))}
      </div>
      {tabs.map((tab, i) => (
        <div
          key={tab.id}
          role="tabpanel"
          id={`${base}-panel-${tab.id}`}
          aria-labelledby={`${base}-tab-${tab.id}`}
          hidden={i !== selected}
          className="docs-tabpanel"
        >
          {tab.blocks.map((b, j) => (
            <Fragment key={j}>
              <Block block={b} copy={copy} />
            </Fragment>
          ))}
        </div>
      ))}
    </div>
  );
}

/** Every word of some blocks, tabs included, for the search. */
function searchText(blocks: readonly DocBlock[]): string {
  return blocks
    .map((b) => {
      switch (b.t) {
        case "p":
        case "h2":
        case "note":
          return b.text;
        case "list":
        case "steps":
          return b.items.join(" ");
        case "code":
          return b.code;
        case "table":
          return [...b.head, ...b.rows.flat()].join(" ");
        case "tabs":
          return b.tabs.map((tab) => `${tab.label} ${searchText(tab.blocks)}`).join(" ");
      }
    })
    .join(" ");
}

/** Sidebar: the pages by group, with a filter. */
function DocsNav({ docs, current, onPick }: { docs: Docs; current: string; onPick: () => void }) {
  const [query, setQuery] = useState("");
  const q = query.trim().toLowerCase();
  const pages = q
    ? docs.pages.filter((p) => `${p.title} ${p.lead} ${searchText(p.blocks)}`.toLowerCase().includes(q))
    : docs.pages;
  const groups = [...new Set(pages.map((p) => p.group))];
  return (
    <nav className="docs-nav" aria-label={docs.menu}>
      <input
        type="search"
        className="docs-search"
        placeholder={docs.search}
        aria-label={docs.search}
        value={query}
        onChange={(e) => setQuery(e.target.value)}
      />
      {groups.length === 0 && <p className="docs-faint">{docs.noResults}</p>}
      {groups.map((g) => (
        <div key={g} className="docs-group">
          <p className="docs-group-title">{g}</p>
          <ul>
            {pages
              .filter((p) => p.group === g)
              .map((p) => (
                <li key={p.slug}>
                  <Link to={`/docs/${p.slug}`} aria-current={p.slug === current ? "page" : undefined} onClick={onPick}>
                    {p.title}
                  </Link>
                </li>
              ))}
          </ul>
        </div>
      ))}
    </nav>
  );
}

/** The user documentation: /docs/<page>, in the site's language. */
export function DocsPage() {
  const lang = useLang();
  const docs = DOCS[lang];
  const { slug } = useParams();
  const [menuOpen, setMenuOpen] = useState(false);
  const index = docs.pages.findIndex((p) => p.slug === slug);
  const page: DocPage | undefined = docs.pages[index];
  const headings = useMemo(
    () => (page ? page.blocks.filter((b): b is Extract<DocBlock, { t: "h2" }> => b.t === "h2") : []),
    [page],
  );

  useEffect(() => {
    if (!window.location.hash) window.scrollTo(0, 0);
  }, [slug]);

  if (!slug) return <Navigate to={`/docs/${docs.pages[0]!.slug}`} replace />;
  if (!page) return <Navigate to="/docs" replace />;
  const prev = docs.pages[index - 1];
  const next = docs.pages[index + 1];

  return (
    <div className="lp docs">
      <div className="lp-wrap">
        <SiteNav />
        <div className="docs-page">
          <aside className={`docs-sidebar${menuOpen ? " is-open" : ""}`}>
            <button type="button" className="lp-btn lp-btn-ghost docs-menu-toggle" aria-expanded={menuOpen} onClick={() => setMenuOpen((o) => !o)}>
              {docs.menu}
            </button>
            <DocsNav docs={docs} current={page.slug} onPick={() => setMenuOpen(false)} />
          </aside>

          <article className="docs-article" aria-labelledby="docs-title">
            <p className="lp-eyebrow">
              {docs.eyebrow} · {page.group}
            </p>
            <h1 id="docs-title">{page.title}</h1>
            <p className="docs-lead">{inline(page.lead)}</p>
            <div className="docs-body">
              {page.blocks.map((b, i) => (
                <Fragment key={i}>
                  <Block block={b} copy={docs.copy} />
                </Fragment>
              ))}
            </div>
            <nav className="docs-pager" aria-label={`${docs.previous} / ${docs.next}`}>
              {prev ? (
                <Link to={`/docs/${prev.slug}`} className="docs-pager-link">
                  <span className="docs-faint">← {docs.previous}</span>
                  <span>{prev.title}</span>
                </Link>
              ) : (
                <span />
              )}
              {next && (
                <Link to={`/docs/${next.slug}`} className="docs-pager-link docs-pager-next">
                  <span className="docs-faint">{docs.next} →</span>
                  <span>{next.title}</span>
                </Link>
              )}
            </nav>
            <p className="docs-faint">
              <a href={`${REPO_URL}/blob/main/frontend/apps/web/src/i18n/docs-${lang}.ts`} target="_blank" rel="noopener">
                {docs.edit}
              </a>
            </p>
          </article>

          {headings.length > 0 && (
            <nav className="docs-toc" aria-label={docs.onThisPage}>
              <p className="docs-group-title">{docs.onThisPage}</p>
              <ul>
                {headings.map((h) => (
                  <li key={h.id}>
                    <a href={`#${h.id}`}>{h.text}</a>
                  </li>
                ))}
              </ul>
            </nav>
          )}
        </div>
      </div>
      <SiteFooter />
    </div>
  );
}
