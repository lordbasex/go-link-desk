// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { describe, expect, it } from "vitest";
import vectors from "../../../../protocol/vectors/input.json";
import table from "../../../../protocol/keycodes.json";
import { KEY_CODES } from "../src/keycodes.gen";
import {
  MAX_TEXT_BYTES,
  encodeAbs,
  encodeButton,
  encodeKey,
  encodeRel,
  encodeReleaseAll,
  encodeText,
  encodeWheel,
  keycodeOf,
  modsOf,
} from "../src/input";

const hex = (b: ArrayBuffer) => [...new Uint8Array(b)].map((x) => x.toString(16).padStart(2, "0")).join("");

type P = Record<string, number | boolean | string>;

function encode(p: P): ArrayBuffer {
  switch (p.type) {
    case "abs":
      return encodeAbs(p.seq as number, p.display as number, p.x as number, p.y as number);
    case "rel":
      return encodeRel(p.seq as number, p.dx as number, p.dy as number);
    case "button":
      return encodeButton(p.button as number, p.down as boolean, p.display as number, p.x as number, p.y as number);
    case "wheel":
      return encodeWheel(p.dx as number, p.dy as number, p.mode as 0 | 1);
    case "key":
      return encodeKey(p.keycode as number, p.down as boolean, p.mods as number, p.repeat as boolean);
    case "text":
      return encodeText(p.text as string)[0]!;
    default:
      return encodeReleaseAll();
  }
}

describe("input packets", () => {
  it.each(vectors.packets.map((v) => [v.name, v] as const))("%s matches the shared vector", (_, v) => {
    expect(hex(encode(v.packet as unknown as P))).toBe(v.hex.replace(/ /g, ""));
  });

  it("splits long text without cutting a character", () => {
    const parts = encodeText("ñ".repeat(MAX_TEXT_BYTES));
    expect(parts.length).toBe(2);
    for (const p of parts) {
      expect(p.byteLength - 1).toBeLessThanOrEqual(MAX_TEXT_BYTES);
      expect(new TextDecoder("utf-8", { fatal: true }).decode(new Uint8Array(p).slice(1))).toMatch(/^ñ+$/);
    }
  });

  it("reads modifiers", () => {
    expect(modsOf({ shiftKey: true, ctrlKey: false, altKey: true, metaKey: true, getModifierState: () => true })).toBe(1 | 4 | 8 | 16);
  });
});

describe("keycodes", () => {
  it("are protocol/keycodes.json, in its order (npm run gen:keycodes)", () => {
    expect(KEY_CODES).toEqual(table.keys.map((k) => k.code));
  });
  it("index KeyboardEvent.code", () => {
    expect(keycodeOf("KeyA")).toBe(1);
    expect(KEY_CODES[keycodeOf("MetaLeft")]).toBe("MetaLeft");
    expect(keycodeOf("Nope")).toBe(0);
  });
});
