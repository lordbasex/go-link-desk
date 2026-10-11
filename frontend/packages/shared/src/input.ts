// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// The viewer's input packets (docs/protocol.md, section 5): pointer moves
// on the unreliable "pointer" channel; buttons, wheel, keys, text and
// release-all on the reliable "input" channel. Big endian;
// protocol/vectors/input.json is the reference both sides test against.

import { KEY_CODES } from "./keycodes.gen";

export const Mods = { Shift: 1, Ctrl: 2, Alt: 4, Meta: 8, Caps: 16 } as const;

/** Mouse buttons as the protocol numbers them (the same as MouseEvent.button). */
export const MouseButton = { Left: 0, Middle: 1, Right: 2, Back: 3, Forward: 4 } as const;

/** The most a text packet carries. */
export const MAX_TEXT_BYTES = 1024;

const u16 = (v: number) => Math.max(0, Math.min(0xffff, Math.round(v)));
const i16 = (v: number) => Math.max(-0x8000, Math.min(0x7fff, Math.round(v)));

/** A position over the display, 0..1, as the wire's 0..65535. */
export function norm(v: number): number {
  return u16(Math.max(0, Math.min(1, v)) * 65535);
}

export function encodeAbs(seq: number, display: number, x: number, y: number): ArrayBuffer {
  const v = new DataView(new ArrayBuffer(8));
  v.setUint8(0, 0x01);
  v.setUint16(1, seq & 0xffff);
  v.setUint8(3, display & 0xff);
  v.setUint16(4, u16(x));
  v.setUint16(6, u16(y));
  return v.buffer;
}

export function encodeRel(seq: number, dx: number, dy: number): ArrayBuffer {
  const v = new DataView(new ArrayBuffer(7));
  v.setUint8(0, 0x02);
  v.setUint16(1, seq & 0xffff);
  v.setInt16(3, i16(dx));
  v.setInt16(5, i16(dy));
  return v.buffer;
}

export function encodeButton(button: number, down: boolean, display: number, x: number, y: number): ArrayBuffer {
  const v = new DataView(new ArrayBuffer(8));
  v.setUint8(0, 0x10);
  v.setUint8(1, button);
  v.setUint8(2, down ? 1 : 0);
  v.setUint8(3, display & 0xff);
  v.setUint16(4, u16(x));
  v.setUint16(6, u16(y));
  return v.buffer;
}

/** Wheel deltas, in pixels (mode 0) or lines (mode 1). */
export function encodeWheel(dx: number, dy: number, mode: 0 | 1): ArrayBuffer {
  const v = new DataView(new ArrayBuffer(6));
  v.setUint8(0, 0x20);
  v.setInt16(1, i16(dx));
  v.setInt16(3, i16(dy));
  v.setUint8(5, mode);
  return v.buffer;
}

export function encodeKey(keycode: number, down: boolean, mods: number, repeat = false): ArrayBuffer {
  const v = new DataView(new ArrayBuffer(5));
  v.setUint8(0, 0x30);
  v.setUint16(1, keycode);
  v.setUint8(3, down ? (repeat ? 2 : 1) : 0);
  v.setUint8(4, mods & 0x1f);
  return v.buffer;
}

/**
 * Text packets for a string (IME commits, typing a paste out): one or
 * more, each at most MAX_TEXT_BYTES and never splitting a character.
 */
export function encodeText(text: string): ArrayBuffer[] {
  const out: ArrayBuffer[] = [];
  const enc = new TextEncoder();
  let chunk = "";
  let size = 0;
  for (const ch of text) {
    const n = enc.encode(ch).length;
    if (size + n > MAX_TEXT_BYTES && chunk) {
      out.push(textPacket(enc.encode(chunk)));
      chunk = "";
      size = 0;
    }
    chunk += ch;
    size += n;
  }
  if (chunk) out.push(textPacket(enc.encode(chunk)));
  return out;
}

function textPacket(bytes: Uint8Array): ArrayBuffer {
  const b = new Uint8Array(bytes.length + 1);
  b[0] = 0x31;
  b.set(bytes, 1);
  return b.buffer;
}

export function encodeReleaseAll(): ArrayBuffer {
  return new Uint8Array([0x3f]).buffer;
}

const INDEX = new Map(KEY_CODES.map((c, i) => [c, i]));

/** A KeyboardEvent.code's keycode on the wire (0: unknown). */
export function keycodeOf(code: string): number {
  return INDEX.get(code) ?? 0;
}

/** The modifier bits of a keyboard or mouse event. */
export function modsOf(e: { shiftKey: boolean; ctrlKey: boolean; altKey: boolean; metaKey: boolean; getModifierState?: (k: string) => boolean }): number {
  return (
    (e.shiftKey ? Mods.Shift : 0) |
    (e.ctrlKey ? Mods.Ctrl : 0) |
    (e.altKey ? Mods.Alt : 0) |
    (e.metaKey ? Mods.Meta : 0) |
    (e.getModifierState?.("CapsLock") ? Mods.Caps : 0)
  );
}
