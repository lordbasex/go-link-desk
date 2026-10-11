// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useEffect, useRef, useState } from "react";
import { t } from "../i18n";
import { chatSoundOn, setChatSound } from "./chatSound";
import { MAX_CHAT, TYPING_EVERY_MS, TYPING_PAUSE_MS, type ChatLine } from "./session";

/**
 * Tells the host this side is writing: "typing" again every few seconds
 * while it writes, "stopped" after a pause, when the line is emptied or
 * when the message goes.
 */
function useTypingSender(onTyping: (on: boolean) => void) {
  const sentAt = useRef(0);
  const pause = useRef<ReturnType<typeof setTimeout> | null>(null);
  const quiet = () => {
    if (pause.current) clearTimeout(pause.current);
    pause.current = null;
    if (sentAt.current) {
      sentAt.current = 0;
      onTyping(false);
    }
  };
  const wrote = (any: boolean) => {
    if (!any) return quiet();
    if (Date.now() - sentAt.current >= TYPING_EVERY_MS) {
      sentAt.current = Date.now();
      onTyping(true);
    }
    if (pause.current) clearTimeout(pause.current);
    pause.current = setTimeout(quiet, TYPING_PAUSE_MS);
  };
  useEffect(() => quiet, []); // eslint-disable-line react-hooks/exhaustive-deps
  return { wrote, quiet };
}

/** The session's chat with the person at the host. */
export function ChatPanel({ lines, hostName, hostTyping, onSend, onTyping, onClose }: {
  lines: ChatLine[];
  hostName: string;
  hostTyping: boolean;
  onSend: (text: string) => void;
  onTyping: (on: boolean) => void;
  onClose: () => void;
}) {
  const [text, setText] = useState("");
  const [sound, setSound] = useState(chatSoundOn);
  const listRef = useRef<HTMLOListElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const typing = useTypingSender(onTyping);

  useEffect(() => inputRef.current?.focus({ preventScroll: true }), []);
  useEffect(() => {
    const el = listRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [lines.length, hostTyping]);

  const send = () => {
    if (!text.trim()) return;
    onSend(text);
    setText("");
    typing.quiet();
  };

  return (
    <aside className="chat-panel" role="dialog" aria-label={t.chat.title}>
      <header className="chat-head">
        <strong>{t.chat.title}</strong>
        <button
          type="button"
          className="icon-button chat-sound"
          aria-pressed={sound}
          aria-label={sound ? t.chat.soundOff : t.chat.soundOn}
          title={sound ? t.chat.soundOff : t.chat.soundOn}
          onClick={() => {
            setChatSound(!sound);
            setSound(!sound);
          }}
        >
          {sound ? "🔔" : "🔕"}
        </button>
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
        {hostTyping && (
          <li className="chat-line from-host chat-typing" aria-label={t.chat.typing.replace("{name}", hostName)}>
            <span className="chat-who">{t.chat.typing.replace("{name}", hostName)}</span>
            <span className="chat-dots" aria-hidden="true">
              <i />
              <i />
              <i />
            </span>
          </li>
        )}
      </ol>
      <div className="chat-write">
        <textarea
          ref={inputRef}
          value={text}
          maxLength={MAX_CHAT}
          rows={2}
          aria-label={t.chat.write}
          placeholder={t.chat.write}
          onChange={(e) => {
            setText(e.target.value);
            typing.wrote(e.target.value.trim() !== "");
          }}
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
