// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { Link, Navigate, useParams } from "react-router-dom";
import { formatCode, parseInvite } from "@go-link-desk/shared";
import { t } from "../i18n";

/**
 * /c/<invite or code>: the session with one computer. For now it only
 * reads the address; joining, consent and the remote screen arrive with
 * the next phase.
 */
export function SessionPage() {
  const { target: raw = "" } = useParams();
  const target = parseInvite(raw);
  if (!target) return <Navigate to="/c" replace />;
  const shown = "code" in target ? formatCode(target.code) : target.invite;
  return (
    <section className="join">
      <h1 className="join-title">{t.session.title}</h1>
      <p className="notice">{t.session.pending.replace("{code}", shown)}</p>
      <div>
        <Link to="/c" className="button button-secondary">
          {t.session.back}
        </Link>
      </div>
    </section>
  );
}
