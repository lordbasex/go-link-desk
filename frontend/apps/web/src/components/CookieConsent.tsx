// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { setConsent, useConsent } from "../analytics";
import { t } from "../i18n";

/**
 * The cookie notice of the public pages: go-link Desk keeps nothing; with
 * the visitor's yes, Google Analytics counts visits to the website. Shown
 * until they choose, and again from "Cookie settings".
 */
export function CookieConsent() {
  const { consent, asking } = useConsent();
  if (consent !== null && !asking) return null;
  const c = t.consent;
  return (
    <section className="consent" aria-labelledby="consent-title">
      <div className="consent-text">
        <strong id="consent-title">{c.title}</strong>
        <p>{c.text}</p>
      </div>
      <div className="consent-actions">
        <button type="button" className="consent-btn" onClick={() => setConsent("denied")}>
          {c.reject}
        </button>
        <button type="button" className="consent-btn consent-accept" onClick={() => setConsent("granted")} autoFocus={asking}>
          {c.accept}
        </button>
      </div>
    </section>
  );
}
