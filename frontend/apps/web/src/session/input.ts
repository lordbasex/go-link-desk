// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import {
  Mods,
  encodeAbs,
  encodeButton,
  encodeKey,
  encodeRel,
  encodeReleaseAll,
  encodeText,
  encodeWheel,
  keycodeOf,
  modsOf,
  norm,
} from "@go-link-desk/shared";

/** Where the picture is, for turning a pointer position into the host's. */
export interface PictureGeometry {
  /** The picture's rectangle on the page (CSS pixels, client coordinates). */
  rect(): { left: number; top: number; width: number; height: number } | null;
  /** The host's display shown: its index in the host's list. */
  display(): number;
}

export interface InputSink {
  sendPointer(packet: ArrayBuffer): void;
  sendInput(packet: ArrayBuffer): void;
}

export interface InputOptions {
  /** This computer's Ctrl is the host's Cmd (a Windows or Linux viewer on a Mac). */
  ctrlIsCmd: boolean;
}

/** A button packet's display when it carries no position (pointer lock). */
const NO_POSITION = 0xff;

/** Wheel lines in a page, for deltaMode 2. */
const LINES_PER_PAGE = 20;

const MODIFIERS = new Set(["ShiftLeft", "ShiftRight", "ControlLeft", "ControlRight", "AltLeft", "AltRight", "MetaLeft", "MetaRight", "CapsLock", "Fn"]);

const CTRL_TO_META: Record<string, string> = { ControlLeft: "MetaLeft", ControlRight: "MetaRight" };

/**
 * The viewer's mouse and keyboard, sent to the host while it may control.
 * Pointer moves go out at most once a frame on the unreliable channel; a
 * click carries its own position, so it lands right even if moves were
 * lost. Keys go by position (KeyboardEvent.code): the host's layout gives
 * the characters. Everything held is let go when the page loses focus, is
 * hidden or leaves full screen, so no key stays stuck on the host.
 */
export class InputCapture {
  private seq = 0;
  private pending: { x: number; y: number } | null = null;
  private rel = { dx: 0, dy: 0 };
  private wheel = { dx: 0, dy: 0, lines: false };
  private frame = 0;
  private down = new Set<string>(); // codes held here
  private buttons = new Set<number>();
  private off: (() => void)[] = [];
  private locked = false;

  constructor(
    private readonly surface: HTMLElement,
    private readonly keys: HTMLElement,
    private readonly geometry: PictureGeometry,
    private readonly sink: InputSink,
    private opts: InputOptions,
  ) {}

  setOptions(opts: InputOptions): void {
    if (opts.ctrlIsCmd !== this.opts.ctrlIsCmd) this.releaseAll();
    this.opts = opts;
  }

  attach(): void {
    const on = <K extends keyof HTMLElementEventMap>(el: HTMLElement | Document | Window, type: K | string, fn: (e: never) => void, opts?: AddEventListenerOptions) => {
      el.addEventListener(type, fn as EventListener, opts);
      this.off.push(() => el.removeEventListener(type, fn as EventListener, opts));
    };
    on(this.surface, "pointermove", (e: PointerEvent) => this.onMove(e));
    on(this.surface, "pointerdown", (e: PointerEvent) => this.onButton(e, true));
    on(this.surface, "pointerup", (e: PointerEvent) => this.onButton(e, false));
    on(this.surface, "pointercancel", () => this.releaseAll());
    on(this.surface, "wheel", (e: WheelEvent) => this.onWheel(e), { passive: false });
    on(this.surface, "contextmenu", (e: Event) => e.preventDefault());
    on(this.keys, "keydown", (e: KeyboardEvent) => this.onKey(e, true));
    on(this.keys, "keyup", (e: KeyboardEvent) => this.onKey(e, false));
    on(this.keys, "input", () => {
      // Only compositions reach the field; it never keeps anything.
      if (this.keys instanceof HTMLTextAreaElement && !this.composing) this.keys.value = "";
    });
    on(this.keys, "compositionstart", () => (this.composing = true));
    on(this.keys, "compositionend", (e: CompositionEvent) => {
      this.composing = false;
      if (e.data) this.typeText(e.data);
      if (this.keys instanceof HTMLTextAreaElement) this.keys.value = "";
    });
    on(this.keys, "blur", () => this.releaseAll());
    on(window, "blur", () => this.releaseAll());
    on(document, "visibilitychange", () => document.visibilityState === "hidden" && this.releaseAll());
    on(document, "fullscreenchange", () => !document.fullscreenElement && this.releaseAll());
    on(document, "pointerlockchange", () => {
      this.locked = document.pointerLockElement === this.surface;
      if (!this.locked) this.releaseAll();
    });
  }

  detach(): void {
    this.releaseAll();
    this.off.splice(0).forEach((f) => f());
    if (this.frame) cancelAnimationFrame(this.frame);
    this.frame = 0;
  }

  private composing = false;

  /** Lets go of every key and button, here and on the host. */
  releaseAll(): void {
    this.down.clear();
    this.buttons.clear();
    this.sink.sendInput(encodeReleaseAll());
  }

  /** Types text on the host, character by character (a paste typed out). */
  typeText(text: string): void {
    for (const p of encodeText(text)) this.sink.sendInput(p);
  }

  /** Presses keys in order and lets go of them in reverse (a shortcut). */
  sendCombo(codes: string[]): void {
    let mods = 0;
    const bit: Record<string, number> = { ShiftLeft: Mods.Shift, ControlLeft: Mods.Ctrl, AltLeft: Mods.Alt, MetaLeft: Mods.Meta };
    for (const c of codes) {
      this.sink.sendInput(encodeKey(keycodeOf(c), true, mods));
      mods |= bit[c] ?? 0;
    }
    for (const c of [...codes].reverse()) {
      mods &= ~(bit[c] ?? 0);
      this.sink.sendInput(encodeKey(keycodeOf(c), false, mods));
    }
  }

  /** The pointer's position over the picture, 0..1, or null outside it. */
  private position(e: { clientX: number; clientY: number }): { x: number; y: number } | null {
    const r = this.geometry.rect();
    if (!r || r.width <= 0 || r.height <= 0) return null;
    const x = (e.clientX - r.left) / r.width;
    const y = (e.clientY - r.top) / r.height;
    if (x < 0 || x > 1 || y < 0 || y > 1) return null;
    return { x, y };
  }

  private schedule(): void {
    if (!this.frame) this.frame = requestAnimationFrame(() => this.flush());
  }

  /** Sends what piled up this frame: one move, one wheel. */
  private flush(): void {
    this.frame = 0;
    if (this.pending) {
      this.seq = (this.seq + 1) & 0xffff;
      this.sink.sendPointer(encodeAbs(this.seq, this.geometry.display(), norm(this.pending.x), norm(this.pending.y)));
      this.pending = null;
    }
    if (this.rel.dx || this.rel.dy) {
      this.seq = (this.seq + 1) & 0xffff;
      this.sink.sendPointer(encodeRel(this.seq, this.rel.dx, this.rel.dy));
      this.rel = { dx: 0, dy: 0 };
    }
    if (this.wheel.dx || this.wheel.dy) {
      this.sink.sendInput(encodeWheel(this.wheel.dx, this.wheel.dy, this.wheel.lines ? 1 : 0));
      this.wheel = { dx: 0, dy: 0, lines: false };
    }
  }

  /**
   * Whether an event belongs to the picture: on the surface itself or the
   * video, not on the page's own controls over it (a dialog, a menu).
   */
  private mine(e: Event): boolean {
    const t = e.target as Element | null;
    return this.locked || t === this.surface || t instanceof HTMLVideoElement || (t instanceof Element && t.classList.contains("remote-cursor"));
  }

  private onMove(e: PointerEvent): void {
    if (!this.mine(e) && !this.buttons.size) return;
    if (this.locked) {
      this.rel.dx += e.movementX;
      this.rel.dy += e.movementY;
      this.schedule();
      return;
    }
    const p = this.position(e);
    if (!p) return;
    this.pending = p;
    this.schedule();
  }

  private onButton(e: PointerEvent, down: boolean): void {
    if (e.button < 0 || e.button > 4) return;
    if (down && !this.mine(e)) return;
    if (!down && !this.buttons.has(e.button)) return; // pressed elsewhere
    if (down) {
      this.keys.focus({ preventScroll: true });
      try {
        this.surface.setPointerCapture(e.pointerId); // a drag keeps going outside
      } catch {
        // not a real pointer (tests)
      }
    }
    e.preventDefault();
    if (this.locked) {
      this.flush();
      this.track(e.button, down);
      this.sink.sendInput(encodeButton(e.button, down, NO_POSITION, 0xffff, 0xffff));
      return;
    }
    const p = this.position(e);
    if (!p && down) return; // outside the picture (its bars)
    this.flush(); // the move before it goes first
    this.track(e.button, down);
    // A release outside the picture (a drag that left it) lets go where
    // the host's pointer is, without moving it.
    if (!p) this.sink.sendInput(encodeButton(e.button, false, NO_POSITION, 0xffff, 0xffff));
    else this.sink.sendInput(encodeButton(e.button, down, this.geometry.display(), norm(p.x), norm(p.y)));
  }

  private track(button: number, down: boolean): void {
    if (down) this.buttons.add(button);
    else this.buttons.delete(button);
  }

  private onWheel(e: WheelEvent): void {
    if (!this.mine(e)) return;
    e.preventDefault();
    // Pixels stay pixels (a trackpad's smooth scroll); lines and pages go
    // as lines.
    const lines = e.deltaMode !== 0;
    const k = e.deltaMode === 2 ? LINES_PER_PAGE : 1;
    if (this.wheel.lines !== lines && (this.wheel.dx || this.wheel.dy)) this.flush();
    this.wheel.lines = lines;
    this.wheel.dx += e.deltaX * k;
    this.wheel.dy += e.deltaY * k;
    this.schedule();
  }

  private onKey(e: KeyboardEvent, down: boolean): void {
    // An IME at work: its text comes whole with compositionend.
    if (e.isComposing || e.key === "Process" || e.keyCode === 229) return;
    let code = e.code;
    if (this.opts.ctrlIsCmd && CTRL_TO_META[code]) code = CTRL_TO_META[code]!;
    const keycode = keycodeOf(code);
    if (!keycode) return;
    e.preventDefault();
    e.stopPropagation();
    if (!down && !this.down.has(code)) return; // a release this page never saw pressed
    if (down) this.down.add(code);
    else this.down.delete(code);
    let mods = modsOf(e);
    if (this.opts.ctrlIsCmd && mods & Mods.Ctrl) mods = (mods & ~Mods.Ctrl) | Mods.Meta;
    this.sink.sendInput(encodeKey(keycode, down, mods, down && e.repeat));
    // macOS browsers never send the keyup of a key pressed while Cmd is
    // down (Cmd+C's C): when Cmd goes, the keys pressed with it go too.
    if (!down && (code === "MetaLeft" || code === "MetaRight")) {
      for (const held of [...this.down]) {
        if (MODIFIERS.has(held)) continue;
        this.down.delete(held);
        this.sink.sendInput(encodeKey(keycodeOf(held), false, mods));
      }
    }
  }
}
