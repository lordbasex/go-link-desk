// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useId, useLayoutEffect, useRef, useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { editCodeFromChange, parseInvite, type CodeEdit } from "@go-link-desk/shared";
import { t } from "../i18n";

/**
 * The code (or link) of a session, as the /c page asks for it. A valid
 * one opens /c/<invite or code>.
 */
export function JoinForm({ autoFocus = false }: { autoFocus?: boolean }) {
  const navigate = useNavigate();
  const id = useId();
  // The code shows grouped as "915 355 636" while typed; a link stays as pasted.
  const [field, setField] = useState<CodeEdit>({ text: "", caret: 0 });
  const [tried, setTried] = useState(false);
  const code = field.text;
  const codeRef = useRef<HTMLInputElement>(null);
  useLayoutEffect(() => {
    const el = codeRef.current;
    if (el && document.activeElement === el) el.setSelectionRange(field.caret, field.caret);
  }, [field]);
  const target = parseInvite(code);
  const invalid = tried && target === null;

  const submit = (e: FormEvent) => {
    e.preventDefault();
    setTried(true);
    if (!target) return;
    navigate(`/c/${"invite" in target ? target.invite : target.code}`);
  };

  return (
    <form className="join" onSubmit={submit} noValidate>
      <label htmlFor={`${id}-code`} className="field-label">
        {t.join.codeLabel}
      </label>
      <div className="join-row">
        <input
          id={`${id}-code`}
          className="input"
          ref={codeRef}
          autoComplete="off"
          autoCapitalize="off"
          autoCorrect="off"
          spellCheck={false}
          autoFocus={autoFocus}
          placeholder="123 456 789"
          aria-invalid={invalid}
          aria-describedby={`${id}-hint`}
          value={code}
          onChange={(e) => {
            const el = e.target;
            setField(editCodeFromChange(code, el.value, el.selectionStart ?? el.value.length));
          }}
        />
        <button type="submit" className="button button-primary">
          {t.join.connect}
        </button>
      </div>
      <span id={`${id}-hint`} className="small muted">
        {t.join.codeHint}
      </span>
      {invalid && (
        <p className="error-text" role="alert">
          {t.join.invalid}
        </p>
      )}
    </form>
  );
}
