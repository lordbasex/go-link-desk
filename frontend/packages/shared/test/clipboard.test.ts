// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { describe, expect, it } from "vitest";
import { CLIPBOARD_PIECE, ClipboardAssembler, MAX_CLIPBOARD, clipboardPieces } from "../src/clipboard";

const bytes = (s: string) => new TextEncoder().encode(s).length;

describe("clipboard pieces", () => {
  it("cuts on characters, within the piece size, and marks the paste on the last one", () => {
    const text = "a" + "日".repeat(CLIPBOARD_PIECE); // 3 bytes each, misaligned
    const ps = clipboardPieces(4, text, true)!;
    expect(ps.length).toBe(4);
    for (const p of ps) expect(bytes(p.text)).toBeLessThanOrEqual(CLIPBOARD_PIECE);
    expect(ps.map((p) => p.text).join("")).toBe(text);
    expect(ps.filter((p) => p.paste).map((p) => p.part)).toEqual([3]);
  });

  it("refuses too much text", () => {
    expect(clipboardPieces(1, "x".repeat(MAX_CLIPBOARD + 1), false)).toBeNull();
  });

  it("puts the pieces back together, and drops a text with a piece out of order", () => {
    const a = new ClipboardAssembler();
    const ps = clipboardPieces(1, "ñ".repeat(CLIPBOARD_PIECE + 1), false)!;
    expect(ps.length).toBe(3);
    expect(a.add(ps[0]!)).toBeNull();
    expect(a.add(ps[1]!)).toBeNull();
    expect(a.add(ps[2]!)).toBe("ñ".repeat(CLIPBOARD_PIECE + 1));
    expect(a.add({ id: 2, part: 1, parts: 2, text: "late" })).toBeNull();
    expect(a.add({ id: 3, part: 0, parts: 1, text: "hola" })).toBe("hola");
    expect(a.add({ id: 3, part: 0, parts: 1, text: 5 })).toBeNull();
  });
});
