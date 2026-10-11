// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useEffect, useRef, useState } from "react";
import { t } from "../i18n";
import { MAX_CHAT, type ChatLine } from "./session";

/** The session's chat with the person at the host. */
export function ChatPanel({ lines, hostName, onSend, onClose }: { lines: ChatLine[]; hostName: string; onSend: (text: string) => void; onClose: () => void }) {
  const [text, setText] = useState("");
  const listRef = useRef<HTMLOListElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => inputRef.current?.focus({ preventScroll: true }), []);
  useEffect(() => {
    const el = listRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [lines.length]);

  const send = () => {
    if (!text.trim()) return;
    onSend(text);
    setText("");
  };

  return (
    <aside className="chat-panel" role="dialog" aria-label={t.chat.title}>
      <header className="chat-head">
        <strong>{t.chat.title}</strong>
        <button type="button" className="button button-secondary button-small" onClick={onClose}>
          {t.session.close}
        </button>
      </header>
      <ol className="chat-lines" ref={listRef} aria-live="polite">
        {lines.length === 0 && <li className="chat-empty">{t.chat.empty}</li>}
        {lines.map((l, i) => (
          <li key={i} className={l.fromHost ? "chat-line from-host" : "chat-line from-me"}>
            <span className="chat-who">
              {l.fromHost ? hostName : t.chat.you} · {new Date(l.at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
            </span>
            <span className="chat-text">{l.text}</span>
          </li>
        ))}
      </ol>
      <div className="chat-write">
        <textarea
          ref={inputRef}
          value={text}
          maxLength={MAX_CHAT}
          rows={2}
          aria-label={t.chat.write}
          placeholder={t.chat.write}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
              e.preventDefault();
              send();
            }
          }}
        />
        <button type="button" className="button button-primary button-small" disabled={!text.trim()} onClick={send}>
          {t.chat.send}
        </button>
      </div>
    </aside>
  );
}
