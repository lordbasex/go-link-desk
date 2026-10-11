// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { keycodeOf } from "@go-link-desk/shared";
import { InputCapture } from "./input";

const bytes = (b: ArrayBuffer) => [...new Uint8Array(b)];

function setup(ctrlIsCmd = false) {
  const surface = document.createElement("div");
  const keys = document.createElement("textarea");
  document.body.append(surface, keys);
  const pointer: ArrayBuffer[] = [];
  const input: ArrayBuffer[] = [];
  const cap = new InputCapture(
    surface,
    keys,
    { rect: () => ({ left: 100, top: 50, width: 1000, height: 500 }), display: () => 1 },
    { sendPointer: (p) => pointer.push(p), sendInput: (p) => input.push(p) },
    { ctrlIsCmd },
  );
  cap.attach();
  return { cap, surface, keys, pointer, input };
}

const key = (el: HTMLElement, type: "keydown" | "keyup", code: string, init: KeyboardEventInit = {}) =>
  el.dispatchEvent(new KeyboardEvent(type, { code, bubbles: true, cancelable: true, ...init }));

beforeEach(() => {
  vi.stubGlobal("requestAnimationFrame", (fn: FrameRequestCallback) => setTimeout(() => fn(0), 0) as unknown as number);
  vi.stubGlobal("cancelAnimationFrame", (id: number) => clearTimeout(id));
});

afterEach(() => {
  document.body.innerHTML = "";
  vi.unstubAllGlobals();
});

describe("InputCapture", () => {
  it("coalesces moves into one a frame, over the picture", async () => {
    const { surface, pointer } = setup();
    const move = (x: number, y: number) => surface.dispatchEvent(new MouseEvent("pointermove", { clientX: x, clientY: y, bubbles: true }) as PointerEvent);
    move(150, 60);
    move(600, 300); // the middle of the picture
    move(5000, 5000); // outside: ignored
    await new Promise((r) => setTimeout(r, 5));
    expect(pointer).toHaveLength(1);
    expect(bytes(pointer[0]!)).toEqual([0x01, 0, 1, 1, 0x80, 0, 0x80, 0]);
  });

  it("ignores a press outside the picture, and lets a release there go where the pointer is", () => {
    const { surface, input } = setup();
    surface.dispatchEvent(new MouseEvent("pointerdown", { button: 0, clientX: 50, clientY: 60, bubbles: true, cancelable: true }) as PointerEvent);
    expect(input).toHaveLength(0);
    // A drag that leaves the picture lets go out there.
    surface.dispatchEvent(new MouseEvent("pointerdown", { button: 0, clientX: 600, clientY: 300, bubbles: true, cancelable: true }) as PointerEvent);
    surface.dispatchEvent(new MouseEvent("pointerup", { button: 0, clientX: 50, clientY: 60, bubbles: true, cancelable: true }) as PointerEvent);
    expect(bytes(input[1]!)).toEqual([0x10, 0, 0, 0xff, 0xff, 0xff, 0xff, 0xff]);
  });

  it("leaves the page's own controls over the picture alone", () => {
    const { surface, input } = setup();
    const button = document.createElement("button");
    surface.append(button);
    button.dispatchEvent(new MouseEvent("pointerdown", { button: 0, clientX: 600, clientY: 300, bubbles: true, cancelable: true }) as PointerEvent);
    button.dispatchEvent(new WheelEvent("wheel", { deltaY: 100, bubbles: true, cancelable: true }));
    expect(input).toHaveLength(0);
  });

  it("sends a click with its own position", () => {
    const { surface, input } = setup();
    surface.dispatchEvent(new MouseEvent("pointerdown", { button: 2, clientX: 100, clientY: 550, bubbles: true, cancelable: true }) as PointerEvent);
    expect(bytes(input[0]!)).toEqual([0x10, 2, 1, 1, 0, 0, 0xff, 0xff]);
  });

  it("sends keys by position, with repeats, and drops releases it never saw pressed", () => {
    const { keys, input } = setup();
    key(keys, "keyup", "KeyA");
    key(keys, "keydown", "KeyA", { shiftKey: true });
    key(keys, "keydown", "KeyA", { shiftKey: true, repeat: true });
    key(keys, "keyup", "KeyA");
    const a = keycodeOf("KeyA");
    expect(input.map(bytes)).toEqual([
      [0x30, 0, a, 1, 1],
      [0x30, 0, a, 2, 1],
      [0x30, 0, a, 0, 0],
    ]);
  });

  it("lets go of the keys pressed with Cmd when Cmd goes (macOS never sends their keyup)", () => {
    const { keys, input } = setup();
    key(keys, "keydown", "MetaLeft", { metaKey: true });
    key(keys, "keydown", "KeyC", { metaKey: true });
    key(keys, "keyup", "MetaLeft");
    const sent = input.map(bytes);
    expect(sent.at(-1)).toEqual([0x30, 0, keycodeOf("KeyC"), 0, 0]);
  });

  it("can send this Ctrl as the host's Cmd", () => {
    const { keys, input } = setup(true);
    key(keys, "keydown", "ControlLeft", { ctrlKey: true });
    expect(bytes(input[0]!)).toEqual([0x30, 0, keycodeOf("MetaLeft"), 1, 8]);
  });

  it("releases everything when the page loses focus", () => {
    const { input } = setup();
    window.dispatchEvent(new Event("blur"));
    expect(bytes(input.at(-1)!)).toEqual([0x3f]);
  });

  it("sends shortcuts and typed text", () => {
    const { cap, input } = setup();
    cap.sendCombo(["MetaLeft", "Tab"]);
    expect(input.map(bytes)).toEqual([
      [0x30, 0, keycodeOf("MetaLeft"), 1, 0],
      [0x30, 0, keycodeOf("Tab"), 1, 8],
      [0x30, 0, keycodeOf("Tab"), 0, 8],
      [0x30, 0, keycodeOf("MetaLeft"), 0, 0],
    ]);
    input.length = 0;
    cap.typeText("hola");
    expect(new TextDecoder().decode(new Uint8Array(input[0]!).slice(1))).toBe("hola");
  });

  it("ignores keys while an IME composes, then types its result", () => {
    const { keys, input } = setup();
    key(keys, "keydown", "KeyA", { isComposing: true });
    keys.dispatchEvent(new CompositionEvent("compositionstart"));
    keys.dispatchEvent(new CompositionEvent("compositionend", { data: "日本" }));
    expect(input).toHaveLength(1);
    expect(new TextDecoder().decode(new Uint8Array(input[0]!).slice(1))).toBe("日本");
  });
});
