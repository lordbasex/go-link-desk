// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { t } from "../i18n";

/** desk.go-link.org/privacy: what go-link Desk keeps, sends and never collects. */
export function PrivacyPage() {
  return (
    <article className="prose">
      <h1>{t.privacy.title}</h1>
      <p className="muted">{t.privacy.updated}</p>
      {t.privacy.sections.map((s) => (
        <section key={s.title}>
          <h2>{s.title}</h2>
          {s.text.map((p) => (
            <p key={p}>{p}</p>
          ))}
        </section>
      ))}
    </article>
  );
}
