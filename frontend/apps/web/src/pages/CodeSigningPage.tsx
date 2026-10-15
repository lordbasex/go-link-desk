// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { Link } from "react-router-dom";
import { REPO_URL } from "../config";
import { t } from "../i18n";

/** desk.go-link.org/code-signing: how the Windows files are signed (SignPath Foundation). */
export function CodeSigningPage() {
  const c = t.codeSigning;
  return (
    <article className="prose">
      <h1>{c.title}</h1>
      <p className="muted">{c.updated}</p>
      <p>{c.lead}</p>
      {/* SignPath Foundation's terms ask for this sentence as it is, in every language. */}
      <p className="signing-credit">
        Free code signing provided by <a href="https://about.signpath.io">SignPath.io</a>, certificate by{" "}
        <a href="https://signpath.org">SignPath Foundation</a>
      </p>
      {c.sections.map((s) => (
        <section key={s.title}>
          <h2>{s.title}</h2>
          {s.text.map((p) => (
            <p key={p}>{p}</p>
          ))}
        </section>
      ))}
      <section>
        <h2>{c.roles.title}</h2>
        <p>
          {c.roles.authors}:{" "}
          <a href="https://github.com/lordbasex" rel="noreferrer">
            Federico Pereira (@lordbasex)
          </a>
        </p>
        <p>
          {c.roles.approvers}:{" "}
          <a href="https://github.com/lordbasex" rel="noreferrer">
            Federico Pereira (@lordbasex)
          </a>
        </p>
        <p>{c.roles.note}</p>
      </section>
      <section>
        <h2>{c.privacy.title}</h2>
        <p>
          {c.privacy.text} <Link to="/privacy">{c.privacy.link}</Link>.
        </p>
      </section>
      <section>
        <h2>{c.verify.title}</h2>
        <p>{c.verify.text}</p>
        <pre className="signing-cmd">
          <code>Get-AuthenticodeSignature .\go-link-desk-setup.exe | Format-List</code>
        </pre>
      </section>
      <section>
        <h2>{c.report.title}</h2>
        <p>
          {c.report.text}{" "}
          <a href={`${REPO_URL}/issues`} rel="noreferrer">
            {REPO_URL.replace("https://", "")}/issues
          </a>
        </p>
      </section>
    </article>
  );
}
