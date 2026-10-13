// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useEffect, useLayoutEffect, useRef, useState, type FormEvent, type ReactNode } from "react";
import { Link, useNavigate } from "react-router-dom";
import { editCodeFromChange, parseInvite, type CodeEdit } from "@go-link-desk/shared";
import { askConsent } from "../analytics";
import { REPO_URL } from "../config";
import { LANGS, setLang, t, useLang } from "../i18n";
import { signalUrl } from "../session/useSession";
import "../styles/landing.css";

const RELEASES = `${REPO_URL}/releases/latest`;

/**
 * desk.go-link.org: what go-link Desk is, the four ways to connect, how it
 * works, what it does, why it is safe, and where to get it. The app itself
 * is at /web; a code typed here opens its session page at once.
 */
export function LandingPage() {
  const lang = useLang();
  const s = t.site;
  const img = (name: string) => `/landing/${name}-${lang}.jpg`;
  return (
    <div className="lp">
      <div className="lp-wrap">
        <SiteNav />

        <div className="lp-hero">
          <div>
            <p className="lp-eyebrow">{s.hero.eyebrow}</p>
            <h1>
              {s.hero.title.split("{em}")[0]}
              <em>{s.hero.titleEm}</em>
              {s.hero.title.split("{em}")[1]}
            </h1>
            <p className="lp-lead">{s.hero.lead}</p>
            <CodeBox />
            <ul className="lp-notes">
              {s.hero.notes.map((n) => (
                <li key={n}>{n}</li>
              ))}
            </ul>
          </div>
          <div className="lp-stage" role="img" aria-label={s.hero.stageAlt}>
            <div className="lp-laptop">
              <div className="lp-screen">
                <img src="/landing/mac-desktop.jpg" alt={s.hero.desktopAlt} />
              </div>
              <div className="lp-base" />
            </div>
            <div className="lp-phone lp-phone-hero">
              <img src={img("android-home")} alt={s.hero.phoneAlt} />
            </div>
            <span className="lp-tag lp-tag-a">{s.hero.tagFps}</span>
            <span className="lp-tag lp-tag-b">{s.hero.tagLatency}</span>
          </div>
        </div>
      </div>

      <section id="ways" className="lp-section">
        <div className="lp-wrap">
          <Head eyebrow={s.ways.eyebrow} title={s.ways.title} text={s.ways.text} />
          <div className="lp-ways">
            <Way chip={s.ways.web.chip} kind="ok" title={s.ways.web.title} text={s.ways.web.text} shot={<img src={`/landing/web-home-${lang}.jpg`} alt={s.ways.web.alt} />}>
              <Link className="lp-go" to="/web">
                {s.ways.web.go}
              </Link>
            </Way>
            <Way chip={s.ways.ios.chip} kind="info" title={s.ways.ios.title} text={s.ways.ios.text} tall shot={<img src={img("iphone-home")} alt={s.ways.ios.alt} />} />
            <Way chip={s.ways.android.chip} kind="soon" title={s.ways.android.title} text={s.ways.android.text} tall shot={<img src={img("android-home")} alt={s.ways.android.alt} />} />
            <Way chip={s.ways.mac.chip} kind="ok" title={s.ways.mac.title} text={s.ways.mac.text} shot={<img src="/landing/mac-desktop.jpg" alt={s.ways.mac.alt} />}>
              <a className="lp-go" href={RELEASES}>
                {s.ways.mac.go}
              </a>
            </Way>
          </div>
          <div className="lp-hosts">
            <strong>{s.ways.hostsLabel}</strong>
            <span className="lp-os lp-os-on">
              <i>{s.ways.today}</i> {s.ways.macHost}
            </span>
            <span className="lp-os lp-os-on">
              <i>{s.ways.today}</i> {s.ways.linuxHost}
            </span>
            <span className="lp-os lp-os-on">
              <i>{s.ways.today}</i> {s.ways.winHost}
            </span>
          </div>
        </div>
      </section>

      <section className="lp-section">
        <div className="lp-wrap">
          <Head eyebrow={s.steps.eyebrow} title={s.steps.title} />
          <ol className="lp-steps">
            {s.steps.items.map((st, i) => (
              <li key={st.title} className="lp-step">
                <h3>{st.title}</h3>
                <p>{st.text}</p>
                {i === 0 && <span className="lp-code">915 355 636</span>}
              </li>
            ))}
          </ol>
        </div>
      </section>

      <section id="features" className="lp-section">
        <div className="lp-wrap">
          <Head eyebrow={s.features.eyebrow} title={s.features.title} />
          <div className="lp-feature">
            <div className="lp-media">
              <img src="/landing/tablet-session.jpg" alt={s.features.picture.alt} />
              <div className="lp-phone lp-phone-land">
                <img src="/landing/android-session.jpg" alt={s.features.picture.phoneAlt} />
              </div>
            </div>
            <div>
              <h3>{s.features.picture.title}</h3>
              <p className="lp-muted">{s.features.picture.lead}</p>
              <Bullets items={s.features.picture.items} />
            </div>
          </div>
          <div className="lp-feature lp-flip">
            <div className="lp-media">
              <img src={img("tablet-guide")} alt={s.features.support.alt} />
              <div className="lp-phone lp-phone-guide">
                <img src={img("iphone-guide")} alt={s.features.support.phoneAlt} />
              </div>
            </div>
            <div>
              <h3>{s.features.support.title}</h3>
              <Bullets items={s.features.support.items} />
            </div>
          </div>
        </div>
      </section>

      <section id="security" className="lp-section lp-sunken">
        <div className="lp-wrap">
          <Head eyebrow={s.security.eyebrow} title={s.security.title} text={s.security.text} />
          <div className="lp-cards">
            {s.security.cards.map((c) => (
              <div key={c.title} className="lp-card">
                <span className="lp-k">{c.k}</span>
                <h3>{c.title}</h3>
                <p>{c.text}</p>
              </div>
            ))}
          </div>
          <div className="lp-path">
            <svg viewBox="0 0 900 140" role="img" aria-label={s.security.diagram}>
              <rect x="10" y="40" width="200" height="60" rx="12" className="lp-svg-box" />
              <text x="110" y="76" textAnchor="middle" className="lp-svg-title">
                {s.security.you}
              </text>
              <rect x="690" y="40" width="200" height="60" rx="12" className="lp-svg-box" />
              <text x="790" y="76" textAnchor="middle" className="lp-svg-title">
                {s.security.computer}
              </text>
              <line x1="210" y1="70" x2="690" y2="70" className="lp-svg-line" />
              <text x="450" y="58" textAnchor="middle" className="lp-svg-data">
                {s.security.data}
              </text>
              <rect x="350" y="104" width="200" height="30" rx="8" className="lp-svg-hub" />
              <text x="450" y="124" textAnchor="middle" className="lp-svg-small">
                {s.security.hub}
              </text>
              <line x1="300" y1="72" x2="370" y2="104" className="lp-svg-dash" />
              <line x1="600" y1="72" x2="530" y2="104" className="lp-svg-dash" />
            </svg>
          </div>
        </div>
      </section>

      <section id="download" className="lp-section">
        <div className="lp-wrap">
          <Head eyebrow={s.download.eyebrow} title={s.download.title} />
          <div className="lp-cards">
            <div className="lp-card lp-dl">
              <h3>{s.download.mac.title}</h3>
              <p>{s.download.mac.text}</p>
              <span className="lp-meta">{s.download.mac.meta}</span>
              <a className="lp-btn lp-btn-primary" href={RELEASES}>
                {s.download.mac.button}
              </a>
            </div>
            <div className="lp-card lp-dl">
              <h3>{s.download.windows.title}</h3>
              <p>{s.download.windows.text}</p>
              <span className="lp-meta">{s.download.windows.meta}</span>
              <a className="lp-btn lp-btn-primary" href={RELEASES}>
                {s.download.windows.button}
              </a>
            </div>
            <div className="lp-card lp-dl">
              <h3>{s.download.linux.title}</h3>
              <p>{s.download.linux.text}</p>
              <span className="lp-meta">{s.download.linux.meta}</span>
              <a className="lp-btn lp-btn-primary" href={RELEASES}>
                {s.download.linux.button}
              </a>
            </div>
          </div>
          <div className="lp-cards lp-cards-2">
            <div className="lp-card lp-dl">
              <h3>{s.download.ios.title}</h3>
              <p>{s.download.ios.text}</p>
              <span className="lp-meta">{s.download.ios.meta}</span>
              <span className="lp-btn lp-btn-ghost" aria-disabled="true">
                {s.download.ios.button}
              </span>
            </div>
            <div className="lp-card lp-dl">
              <h3>{s.download.android.title}</h3>
              <p>{s.download.android.text}</p>
              <span className="lp-meta">{s.download.android.meta}</span>
              <span className="lp-btn lp-btn-ghost" aria-disabled="true">
                {s.download.android.button}
              </span>
            </div>
          </div>
          <div className="lp-hosts">
            <strong>{s.download.noInstall}</strong>
            <span>
              {s.download.openWeb.split("{web}")[0]}
              <Link to="/web">desk.go-link.org/web</Link>
              {s.download.openWeb.split("{web}")[1]}
            </span>
          </div>
        </div>
      </section>

      <section className="lp-section">
        <div className="lp-wrap">
          <Head eyebrow={s.faq.eyebrow} title={s.faq.title} />
          <div className="lp-faq">
            {s.faq.items.map((f, i) => (
              <details key={f.q} open={i === 0}>
                <summary>{f.q}</summary>
                <p>{f.a}</p>
              </details>
            ))}
          </div>
        </div>
      </section>

      <SiteFooter />
    </div>
  );
}

function Head({ eyebrow, title, text }: { eyebrow: string; title: string; text?: string }) {
  return (
    <div className="lp-head">
      <p className="lp-eyebrow">{eyebrow}</p>
      <h2>{title}</h2>
      {text && <p>{text}</p>}
    </div>
  );
}

function Way({ chip, kind, title, text, shot, tall, children }: { chip: string; kind: "ok" | "soon" | "info"; title: string; text: string; shot: ReactNode; tall?: boolean; children?: ReactNode }) {
  return (
    <article className="lp-way">
      <div className={tall ? "lp-shot lp-shot-tall" : "lp-shot"}>{shot}</div>
      <div className="lp-way-body">
        <span className={`lp-chip lp-chip-${kind}`}>{chip}</span>
        <h3>{title}</h3>
        <p>{text}</p>
        {children}
      </div>
    </article>
  );
}

function Bullets({ items }: { items: readonly { b: string; t: string }[] }) {
  return (
    <ul className="lp-bullets">
      {items.map((i) => (
        <li key={i.b}>
          <span>
            <b>{i.b}</b> {i.t}
          </span>
        </li>
      ))}
    </ul>
  );
}

/** The hero's code: grouped as typed, and straight to its session page. */
function CodeBox() {
  const navigate = useNavigate();
  const [field, setField] = useState<CodeEdit>({ text: "", caret: 0 });
  const ref = useRef<HTMLInputElement>(null);
  useLayoutEffect(() => {
    const el = ref.current;
    if (el && document.activeElement === el) el.setSelectionRange(field.caret, field.caret);
  }, [field]);
  const target = parseInvite(field.text);
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (target) navigate(`/c/${"invite" in target ? target.invite : target.code}`);
    else navigate("/web");
  };
  return (
    <form className="lp-codebox" onSubmit={submit}>
      <label htmlFor="lp-code" className="visually-hidden">
        {t.site.hero.codeLabel}
      </label>
      <input
        id="lp-code"
        ref={ref}
        inputMode="numeric"
        autoComplete="off"
        spellCheck={false}
        placeholder="000 000 000"
        value={field.text}
        onChange={(e) => setField(editCodeFromChange(field.text, e.target.value, e.target.selectionStart ?? e.target.value.length))}
      />
      <button type="submit" className="lp-btn lp-btn-primary">
        {t.web.connect}
      </button>
    </form>
  );
}

type Status = "checking" | "ok" | "down";

/** Whether the signaling server answers (a hello), checked once. */
function useSignalStatus(): Status {
  const [status, setStatus] = useState<Status>("checking");
  useEffect(() => {
    let done = false;
    let ws: WebSocket | undefined;
    const finish = (s: Status) => {
      if (done) return;
      done = true;
      setStatus(s);
      ws?.close();
    };
    try {
      const url = signalUrl();
      ws = new WebSocket(url.includes("?") ? `${url}&v=1` : `${url}?v=1`);
      ws.onmessage = (e) => {
        try {
          if (JSON.parse(String(e.data)).type === "hello") finish("ok");
        } catch {
          // not JSON: keep waiting
        }
      };
      ws.onerror = () => finish("down");
    } catch {
      finish("down");
    }
    const timer = window.setTimeout(() => finish("down"), 8000);
    return () => {
      done = true;
      window.clearTimeout(timer);
      ws?.close();
    };
  }, []);
  return status;
}

/** The site's header: the brand, the sections, the docs and the web app. */
export function SiteNav() {
  const s = t.site;
  return (
    <header className="lp-nav">
      <Link className="lp-brand" to="/" aria-label={t.brand.home}>
        <img src="/landing/icon.png" alt="" width={32} height={32} />
        {t.brand.name} <b>{t.brand.suffix}</b>
      </Link>
      <nav className="lp-nav-links" aria-label={s.nav.ways}>
        <a href="/#ways">{s.nav.ways}</a>
        <a href="/#features">{s.nav.features}</a>
        <a href="/#security">{s.nav.security}</a>
        <Link to="/docs">{s.nav.docs}</Link>
        <a href={REPO_URL} rel="noreferrer">
          {s.nav.source}
        </a>
      </nav>
      <div className="lp-nav-cta">
        <Link className="lp-btn lp-btn-ghost" to="/web">
          {s.nav.openWeb}
        </Link>
        <a className="lp-btn lp-btn-primary" href="/#download">
          {s.nav.downloadButton}
        </a>
      </div>
    </header>
  );
}

/** The site map, the scam warning, and the fine print. */
export function SiteFooter() {
  const lang = useLang();
  const f = t.site.footer;
  const status = useSignalStatus();
  return (
    <footer className="lp-foot">
      <div className="lp-wrap">
        <div className="lp-foot-top">
          <div className="lp-foot-brand">
            <Link className="lp-brand" to="/">
              <img src="/landing/icon.png" alt="" width={32} height={32} />
              {t.brand.name} <b>{t.brand.suffix}</b>
            </Link>
            <p>{f.tagline}</p>
            <Link className="lp-foot-code" to="/web">
              000 000 000 <span>{f.connect}</span>
            </Link>
          </div>
          <nav className="lp-foot-col" aria-label={f.product}>
            <h4>{f.product}</h4>
            <ul>
              <li>
                <Link to="/web">{f.webApp}</Link>
              </li>
              <li>
                <a href={RELEASES}>{f.macApp}</a>
              </li>
              <li>
                <a href="/#download">{f.ios}</a>
                <span className="lp-soon">{f.review}</span>
              </li>
              <li>
                <a href="/#download">{f.android}</a>
                <span className="lp-soon">{f.soon}</span>
              </li>
              <li>
                <a href={RELEASES}>{f.linuxApp}</a>
              </li>
              <li>
                <a href={RELEASES}>{f.winLinux}</a>
              </li>
            </ul>
          </nav>
          <nav className="lp-foot-col" aria-label={f.help}>
            <h4>{f.help}</h4>
            <ul>
              <li>
                <Link to="/docs">{t.site.nav.docs}</Link>
              </li>
              <li>
                <a href="/#features">{t.site.nav.features}</a>
              </li>
              <li>
                <a href="/#security">{t.site.nav.security}</a>
              </li>
              <li>
                <Link to="/privacy">{f.privacy}</Link>
              </li>
              <li>
                <button type="button" className="lp-foot-link" onClick={askConsent}>
                  {t.consent.settings}
                </button>
              </li>
              <li>
                <a href="/#download">{t.site.nav.download}</a>
              </li>
              <li>
                <a href={`${REPO_URL}/issues`} rel="noreferrer">
                  {f.report}
                </a>
              </li>
            </ul>
          </nav>
          <nav className="lp-foot-col" aria-label={f.project}>
            <h4>{f.project}</h4>
            <ul>
              <li>
                <a href={REPO_URL} rel="noreferrer">
                  {f.source}
                </a>
              </li>
              <li>
                <a href={`${REPO_URL}/blob/main/CHANGELOG.md`} rel="noreferrer">
                  {f.changes}
                </a>
              </li>
              <li>
                <a href={`${REPO_URL}/blob/main/LICENSE`} rel="noreferrer">
                  {f.license}
                </a>
              </li>
              <li>
                <a href="https://github.com/lordbasex/signalhub" rel="noreferrer">
                  signalhub
                </a>
              </li>
              <li>
                <a href="https://github.com/lordbasex/go-link" rel="noreferrer">
                  go-link
                </a>
              </li>
            </ul>
          </nav>
        </div>
        <div className="lp-foot-warn" role="note">
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <path d="M12 3l9 16H3z" />
            <path d="M12 10v4M12 17h.01" />
          </svg>
          <span>
            <b>{f.warnTitle}</b> {f.warn}
          </span>
        </div>
        <div className="lp-foot-bottom">
          <span>{f.copy}</span>
          <span className={`lp-status lp-status-${status}`} role="status">
            {status === "ok" ? f.statusOk : status === "down" ? f.statusDown : f.statusChecking}
          </span>
          <div className="lp-langs" role="group" aria-label={t.header.language}>
            {LANGS.map((l) => (
              <button key={l.id} type="button" aria-pressed={l.id === lang} lang={l.id} onClick={() => setLang(l.id)}>
                {l.label}
              </button>
            ))}
          </div>
        </div>
      </div>
    </footer>
  );
}
