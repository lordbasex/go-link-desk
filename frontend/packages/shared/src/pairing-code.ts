// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// The 9-digit code a user types to link a browser with a device.

export const CODE_DIGITS = 9;

/** Removes separators; returns the 9 digits or null. */
export function normalizeCode(input: string): string | null {
  const digits = input.replace(/[\s.-]/g, "");
  return /^[0-9]{9}$/.test(digits) ? digits : null;
}

/** "113134323" -> "113 134 323". */
export function formatCode(code: string): string {
  return code.length === CODE_DIGITS ? `${code.slice(0, 3)} ${code.slice(3, 6)} ${code.slice(6)}` : code;
}

/** "915355636" (up to 9 digits) -> "915 355 636", grouped 3-3-3 as typed. */
export function groupCode(digits: string): string {
  return (digits.match(/\d{1,3}/g) ?? []).join(" ");
}

/** A code field's text after an edit, and where its caret goes. */
export type CodeEdit = { text: string; caret: number };

const LINK_CHARS = /[^\d\s.-]/;
const MAX_FIELD = 120;

/** Offset in a grouped text right after its first `count` digits. */
function caretAfterDigits(text: string, count: number): number {
  if (count <= 0) return 0;
  let seen = 0;
  for (let i = 0; i < text.length; i++) {
    if (/\d/.test(text[i]!) && ++seen === count) return i + 1;
  }
  return text.length;
}

const digitsOf = (s: string) => s.replace(/\D/g, "");

/**
 * Applies an edit to a room code field that shows the code grouped as
 * "915 355 636": `text` is what the field showed, [start, end) the range
 * the user replaced and `insert` what they typed or pasted. The spaces are
 * only visual: the digits keep their order and the caret stays after the
 * same digit. Deleting just a space (backspace over it) deletes the digit
 * before it. When the field also takes links (`allowLinks`) and the result
 * holds anything but digits and separators, it is left as typed: a link is
 * never grouped.
 */
export function editCode(
  text: string,
  start: number,
  end: number,
  insert: string,
  allowLinks = true,
): CodeEdit {
  const raw = text.slice(0, start) + insert + text.slice(end);
  if (allowLinks && LINK_CHARS.test(raw)) {
    const capped = raw.slice(0, MAX_FIELD);
    return { text: capped, caret: Math.min(start + insert.length, capped.length) };
  }
  if (insert === "" && end > start && start > 0 && /^\s+$/.test(text.slice(start, end))) {
    // Backspace over a separator: take the digit before it too.
    const before = digitsOf(text.slice(0, start));
    return build(before.slice(0, -1), digitsOf(text.slice(end)), "");
  }
  return build(digitsOf(text.slice(0, start)), digitsOf(text.slice(end)), digitsOf(insert));
}

function build(before: string, after: string, typed: string): CodeEdit {
  const digits = (before + typed + after).slice(0, CODE_DIGITS);
  const grouped = groupCode(digits);
  return { text: grouped, caret: caretAfterDigits(grouped, Math.min(CODE_DIGITS, before.length + typed.length)) };
}

/**
 * The same edit worked out from a field's text before and after a change
 * and the caret after it, for inputs that only report their new value.
 */
export function editCodeFromChange(prev: string, next: string, caret: number, allowLinks = true): CodeEdit {
  let start = 0;
  const limit = Math.min(caret, prev.length);
  while (start < limit && prev[start] === next[start]) start++;
  const tail = next.length - caret;
  const end = Math.max(start, prev.length - tail);
  return editCode(prev, start, end, next.slice(start, caret), allowLinks);
}
