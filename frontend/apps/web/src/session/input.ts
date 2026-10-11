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
  /** The host's points per CSS pixel of the picture (touchpad moves). */
  pointsPerPixel?(): number;
}

export interface InputSink {
  sendPointer(packet: ArrayBuffer): void;
  sendInput(packet: ArrayBuffer): void;
  /** Puts text on the host's clipboard and pastes it there. */
  sendClipboard(text: string, paste: boolean): boolean;
}

export interface InputOptions {
  /** This computer's Ctrl is the host's Cmd (a Windows or Linux viewer on a Mac). */
  ctrlIsCmd: boolean;
  /**
   * The viewer may share the clipboard: Cmd+V or Ctrl+V pastes what is
   * copied here, not what is copied on the host.
   */
  pasteToHost: boolean;
}

/** A button packet's display when it carries no position (pointer lock). */
const NO_POSITION = 0xff;

/** Wheel lines in a page, for deltaMode 2. */
const LINES_PER_PAGE = 20;

/**
 * Touchpad mode (a finger on a tablet or phone): the finger moves the
 * host's pointer like a laptop's touchpad instead of clicking where it
 * lands, which a finger is too coarse for.
 */
const TOUCH_SPEED = 1.6; // host points per point the finger moves, at the picture's scale
const TAP_MS = 250; // a touch shorter than this, that barely moved, is a tap
const TAP_SLOP = 10; // CSS pixels a tap may wander
const TWO_FINGER_SCROLL = 2; // wheel pixels per CSS pixel two fingers move

interface TouchPoint {
  x: number;
  y: number;
  startX: number;
  startY: number;
  at: number;
}

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
  private touches = new Map<number, TouchPoint>();
  private touchMax = 0; // fingers down at once in this gesture
  private touchFrac = { x: 0, y: 0 }; // moves smaller than a point, kept
  /** Called when the input switches between a mouse and a finger. */
  onTouchMode: ((touch: boolean) => void) | null = null;
  private touchMode = false;

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
    on(this.surface, "pointermove", (e: PointerEvent) => (e.pointerType === "touch" ? this.onTouchMove(e) : this.onMove(e)));
    on(this.surface, "pointerdown", (e: PointerEvent) => (e.pointerType === "touch" ? this.onTouchDown(e) : this.onButton(e, true)));
    on(this.surface, "pointerup", (e: PointerEvent) => (e.pointerType === "touch" ? this.onTouchUp(e) : this.onButton(e, false)));
    on(this.surface, "pointercancel", (e: PointerEvent) => {
      if (e.pointerType === "touch") this.touches.delete(e.pointerId);
      else this.releaseAll();
    });
    on(this.surface, "wheel", (e: WheelEvent) => this.onWheel(e), { passive: false });
    on(this.surface, "contextmenu", (e: Event) => e.preventDefault());
    on(this.keys, "keydown", (e: KeyboardEvent) => this.onKey(e, true));
    on(this.keys, "keyup", (e: KeyboardEvent) => this.onKey(e, false));
    on(this.keys, "input", (e: InputEvent) => this.onTextInput(e));
    on(this.keys, "compositionstart", () => (this.composing = true));
    on(this.keys, "compositionend", (e: CompositionEvent) => {
      this.composing = false;
      if (e.data) this.typeText(e.data);
      this.resetField();
    });
    on(this.keys, "paste", (e: ClipboardEvent) => this.onPaste(e));
    on(this.keys, "blur", () => this.releaseAll());
    on(window, "blur", () => this.releaseAll());
    on(document, "visibilitychange", () => document.visibilityState === "hidden" && this.releaseAll());
    on(document, "fullscreenchange", () => !document.fullscreenElement && this.releaseAll());
    on(document, "pointerlockchange", () => {
      this.locked = document.pointerLockElement === this.surface;
      if (!this.locked) this.releaseAll();
    });
    this.resetField();
  }

  /** Opens the touchscreen keyboard (call it from a tap: iOS only opens it then). */
  showKeyboard(): void {
    this.resetField();
    this.keys.focus({ preventScroll: true });
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
    this.touches.clear();
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
    // The screen's own video only: a call's videos are the page's.
    return this.locked || t === this.surface || (t instanceof HTMLVideoElement && t.classList.contains("screen")) || (t instanceof Element && t.classList.contains("remote-cursor"));
  }

  private onMove(e: PointerEvent): void {
    if (!this.mine(e) && !this.buttons.size) return;
    if (e.pointerType === "mouse" || e.pointerType === "pen") this.setTouchMode(false);
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

  /** The viewer's Cmd+V or Ctrl+V: its text goes to the host's clipboard, and the host pastes it. */
  private onPaste(e: ClipboardEvent): void {
    e.preventDefault();
    const text = e.clipboardData?.getData("text/plain") ?? "";
    if (text) this.sink.sendClipboard(text, true);
  }

  /**
   * What reaches the field without a key the page understood: a
   * touchscreen keyboard's typing (iOS and Android send text, not keys),
   * and its Backspace and Return. The field keeps one space so a
   * Backspace always has something to delete, or it would not be told.
   */
  private onTextInput(e: InputEvent): void {
    if (this.composing) return;
    switch (e.inputType) {
      case "insertText":
      case "insertReplacementText":
        if (e.data) this.typeText(e.data);
        break;
      case "insertLineBreak":
      case "insertParagraph":
        this.sendCombo(["Enter"]);
        break;
      case "deleteContentBackward":
        this.sendCombo(["Backspace"]);
        break;
      case "deleteContentForward":
        this.sendCombo(["Delete"]);
        break;
    }
    this.resetField();
  }

  /** Leaves the key field holding one space, the caret after it. */
  resetField(): void {
    if (!(this.keys instanceof HTMLTextAreaElement)) return;
    this.keys.value = " ";
    this.keys.setSelectionRange(1, 1);
  }

  private setTouchMode(touch: boolean): void {
    if (touch === this.touchMode) return;
    this.touchMode = touch;
    this.onTouchMode?.(touch);
  }

  private onTouchDown(e: PointerEvent): void {
    if (!this.mine(e)) return;
    e.preventDefault();
    this.setTouchMode(true);
    if (this.touches.size === 0) this.touchMax = 0;
    this.touches.set(e.pointerId, { x: e.clientX, y: e.clientY, startX: e.clientX, startY: e.clientY, at: performance.now() });
    this.touchMax = Math.max(this.touchMax, this.touches.size);
    try {
      this.surface.setPointerCapture(e.pointerId);
    } catch {
      // not a real pointer (tests)
    }
  }

  private onTouchMove(e: PointerEvent): void {
    const t = this.touches.get(e.pointerId);
    if (!t) return;
    e.preventDefault();
    const dx = e.clientX - t.x;
    const dy = e.clientY - t.y;
    t.x = e.clientX;
    t.y = e.clientY;
    if (this.touches.size >= 2) {
      // Two fingers scroll, as on a touchpad: the content follows them.
      // Each finger reports; half of each is the gesture's move.
      this.wheel.lines = false;
      this.wheel.dx -= (dx / this.touches.size) * TWO_FINGER_SCROLL;
      this.wheel.dy -= (dy / this.touches.size) * TWO_FINGER_SCROLL;
      this.schedule();
      return;
    }
    if (this.touchMax > 1) return; // the second finger lifted: the gesture is over
    const k = (this.geometry.pointsPerPixel?.() ?? 1) * TOUCH_SPEED;
    this.touchFrac.x += dx * k;
    this.touchFrac.y += dy * k;
    const mx = Math.trunc(this.touchFrac.x);
    const my = Math.trunc(this.touchFrac.y);
    this.touchFrac.x -= mx;
    this.touchFrac.y -= my;
    if (mx || my) {
      this.rel.dx += mx;
      this.rel.dy += my;
      this.schedule();
    }
  }

  private onTouchUp(e: PointerEvent): void {
    const t = this.touches.get(e.pointerId);
    if (!t) return;
    e.preventDefault();
    this.touches.delete(e.pointerId);
    if (this.touches.size > 0) return; // the gesture ends with its last finger
    const tap = performance.now() - t.at < TAP_MS && Math.hypot(e.clientX - t.startX, e.clientY - t.startY) < TAP_SLOP;
    if (!tap) return;
    // A tap clicks where the host's pointer is; two fingers right-click.
    const button = this.touchMax >= 2 ? 2 : 0;
    this.flush();
    this.sink.sendInput(encodeButton(button, true, NO_POSITION, 0xffff, 0xffff));
    this.sink.sendInput(encodeButton(button, false, NO_POSITION, 0xffff, 0xffff));
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
    // Paste: let the browser fire its paste event, which carries what is
    // copied here (no permission prompt, in every browser).
    if (down && this.opts.pasteToHost && e.code === "KeyV" && (e.metaKey || e.ctrlKey) && !e.altKey && !e.shiftKey) return;
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
