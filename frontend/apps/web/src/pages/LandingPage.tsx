// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { Link } from "react-router-dom";
import { t } from "../i18n";

/** desk.go-link.org: what the product is and the way in. */
export function LandingPage() {
  return (
    <>
      <section className="hero">
        <h1 className="hero-title">{t.landing.title}</h1>
        <p className="hero-text">{t.landing.text}</p>
        <div>
          <Link to="/c" className="button button-primary button-large">
            {t.landing.connect}
          </Link>
        </div>
      </section>
      <ol className="steps">
        {t.landing.steps.map((s) => (
          <li key={s.title} className="step">
            <h2 className="step-title">{s.title}</h2>
            <p className="muted">{s.text}</p>
          </li>
        ))}
      </ol>
    </>
  );
}
