// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { t } from "../i18n";
import { JoinForm } from "../components/JoinForm";

/** /c: the code of the computer to control. */
export function JoinPage() {
  return (
    <section className="join">
      <h1 className="join-title">{t.join.title}</h1>
      <JoinForm autoFocus />
    </section>
  );
}
