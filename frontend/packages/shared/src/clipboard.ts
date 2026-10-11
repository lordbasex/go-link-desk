// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

/** The most text of a clipboard piece, in UTF-8 bytes (the host's ClipboardPiece). */
export const CLIPBOARD_PIECE = 16 << 10;
/** The most text a clipboard carries, in UTF-8 bytes (the host's MaxClipboard). */
export const MAX_CLIPBOARD = 1 << 20;

/** One piece of a clipboard's text, the "clipboard" control message. */
export interface ClipboardMessage {
  type: "clipboard";
  id: number;
  part: number;
  parts: number;
  text: string;
  /** On the last piece, viewer to host: paste it where the host's cursor is. */
  paste?: boolean;
}

const encoder = new TextEncoder();

/** Cuts text into pieces of at most CLIPBOARD_PIECE bytes, never splitting a character. Null if it is too long. */
export function clipboardPieces(id: number, text: string, paste: boolean): ClipboardMessage[] | null {
  if (encoder.encode(text).length > MAX_CLIPBOARD) return null;
  const pieces: string[] = [];
  let cur = "";
  let bytes = 0;
  for (const ch of text) {
    const n = encoder.encode(ch).length;
    if (bytes + n > CLIPBOARD_PIECE) {
      pieces.push(cur);
      cur = "";
      bytes = 0;
    }
    cur += ch;
    bytes += n;
  }
  pieces.push(cur);
  return pieces.map((p, i) => ({ type: "clipboard", id, part: i, parts: pieces.length, text: p, ...(paste && i === pieces.length - 1 ? { paste: true } : {}) }));
}

/** Puts the host's clipboard pieces back together; a piece out of order drops the text. */
export class ClipboardAssembler {
  private id = -1;
  private parts: string[] = [];

  /** Takes a piece; returns the whole text when its last piece arrives. */
  add(m: Partial<Record<"id" | "part" | "parts" | "text", unknown>>): string | null {
    const id = typeof m.id === "number" ? m.id : -1;
    const part = typeof m.part === "number" ? m.part : -1;
    const parts = typeof m.parts === "number" ? m.parts : 0;
    const text = typeof m.text === "string" ? m.text : null;
    if (text === null || parts < 1 || parts > MAX_CLIPBOARD / CLIPBOARD_PIECE + 1 || part < 0 || part >= parts) return null;
    if (part === 0 || id !== this.id) {
      this.id = id;
      this.parts = [];
    }
    if (part !== this.parts.length) {
      this.parts = [];
      return null;
    }
    this.parts.push(text);
    if (this.parts.length < parts) return null;
    const whole = this.parts.join("");
    this.parts = [];
    return whole.length <= MAX_CLIPBOARD ? whole : null;
  }
}
