// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent as ReactKeyboardEvent,
  type SyntheticEvent,
} from "react";
import { createPortal } from "react-dom";

// The site's own dropdown (atom of the design system), in place of the
// browser's native <select>, whose open list the operating system draws.
// It follows the WAI-ARIA "select-only combobox" pattern: the focus stays
// on the button (role combobox) and aria-activedescendant points at the
// highlighted option of the listbox, which is portaled to the page (or to
// the full screen element) so no popover, drawer or dialog clips it.

export interface SelectOption<V extends string = string> {
  value: V;
  label: string;
  /** Optional second line, in a quieter color. */
  detail?: string;
}

export interface SelectProps<V extends string = string> {
  value: V;
  options: readonly SelectOption<V>[];
  onChange: (value: V) => void;
  /** The button's id (a <label htmlFor> focuses it). */
  id?: string;
  /** The id of the visible <label> that names the control. */
  labelId?: string;
  /** A name for a control without a visible label. */
  ariaLabel?: string;
  disabled?: boolean;
  /** Extra classes for the button (size variants: select-sm, select-lg). */
  className?: string;
}

/** Keys typed within this time build one type-ahead search. */
const TYPEAHEAD_MS = 600;
/** Space kept between the list and the window's edges. */
const EDGE = 8;
const MAX_LIST_HEIGHT = 320;

/** Where the open list goes: a full screen element hides everything else. */
function portalTarget(): Element {
  return document.fullscreenElement ?? document.body;
}

/** Stops a pointer or click event inside the list from reaching the page. */
const keepInside = (e: SyntheticEvent) => e.stopPropagation();

export function Select<V extends string = string>({
  value,
  options,
  onChange,
  id,
  labelId,
  ariaLabel,
  disabled = false,
  className = "",
}: SelectProps<V>) {
  const autoId = useId();
  const buttonId = id ?? `select-${autoId}`;
  const listId = `${buttonId}-list`;
  const optionId = (i: number) => `${buttonId}-opt-${i}`;
  const buttonRef = useRef<HTMLButtonElement>(null);
  const listRef = useRef<HTMLUListElement>(null);
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const [pos, setPos] = useState<CSSProperties>({ visibility: "hidden" });
  // Games are drawn on black: the video keeps the dark colors in the light
  // theme, and so does a list opened from a control over it.
  const [onStage, setOnStage] = useState(false);
  const typed = useRef({ text: "", at: 0 });

  const selectedIndex = options.findIndex((o) => o.value === value);
  const selected = selectedIndex >= 0 ? options[selectedIndex] : undefined;

  const show = useCallback(
    (index?: number) => {
      if (disabled || options.length === 0) return;
      setActive(index ?? Math.max(0, selectedIndex));
      setOnStage(!!buttonRef.current?.closest(".video-stage, .test-controller-stage"));
      setPos({ visibility: "hidden" });
      setOpen(true);
    },
    [disabled, options.length, selectedIndex],
  );
  const hide = useCallback((refocus = true) => {
    setOpen(false);
    typed.current.text = "";
    if (refocus) buttonRef.current?.focus({ preventScroll: true });
  }, []);
  const pick = (index: number) => {
    const option = options[index];
    if (!option) return;
    hide();
    if (option.value !== value) onChange(option.value);
  };

  // Close when disabled or when the options run out while open.
  useEffect(() => {
    if (open && (disabled || options.length === 0)) hide(false);
  }, [open, disabled, options.length, hide]);
  useEffect(() => {
    if (active >= options.length && options.length > 0) setActive(options.length - 1);
  }, [active, options.length]);

  // Under the button, or above it when there is more room there; never
  // outside the window. Follows the button while the page scrolls.
  useLayoutEffect(() => {
    if (!open) return;
    const place = () => {
      const button = buttonRef.current;
      const list = listRef.current;
      if (!button || !list) return;
      const b = button.getBoundingClientRect();
      const vw = window.innerWidth;
      const vh = window.innerHeight;
      const natural = Math.min(list.scrollHeight, MAX_LIST_HEIGHT);
      const below = vh - b.bottom - EDGE - 4;
      const above = b.top - EDGE - 4;
      const up = below < natural && above > below;
      const maxHeight = Math.max(80, Math.min(MAX_LIST_HEIGHT, up ? above : below));
      const width = Math.min(Math.max(b.width, 180), vw - EDGE * 2);
      const left = Math.min(Math.max(EDGE, b.left), vw - EDGE - width);
      setPos(
        up
          ? { left, minWidth: width, maxWidth: vw - EDGE * 2, bottom: vh - b.top + 4, maxHeight }
          : { left, minWidth: width, maxWidth: vw - EDGE * 2, top: b.bottom + 4, maxHeight },
      );
    };
    place();
    const onScroll = (e: Event) => {
      if (e.target instanceof Node && listRef.current?.contains(e.target)) return;
      place();
    };
    window.addEventListener("resize", place);
    window.addEventListener("scroll", onScroll, true);
    return () => {
      window.removeEventListener("resize", place);
      window.removeEventListener("scroll", onScroll, true);
    };
  }, [open, options.length]);

  // Keep the highlighted option in sight.
  useEffect(() => {
    if (!open) return;
    const el = document.getElementById(`${buttonId}-opt-${active}`);
    el?.scrollIntoView?.({ block: "nearest" });
  }, [open, active, buttonId]);

  // A press anywhere else closes the list (and only the list).
  useEffect(() => {
    if (!open) return;
    const away = (e: Event) => {
      const target = e.target as Node;
      if (buttonRef.current?.contains(target) || listRef.current?.contains(target)) return;
      hide(false);
    };
    document.addEventListener("pointerdown", away, true);
    document.addEventListener("mousedown", away, true);
    return () => {
      document.removeEventListener("pointerdown", away, true);
      document.removeEventListener("mousedown", away, true);
    };
  }, [open, hide]);

  /** The next option whose label starts with what was typed. */
  const search = (key: string): number | undefined => {
    const now = Date.now();
    const t = typed.current;
    t.text = now - t.at > TYPEAHEAD_MS ? key : t.text + key;
    t.at = now;
    const text = t.text.toLocaleLowerCase();
    // Repeating one letter cycles through the options with that letter.
    const cycling = text.length > 1 && [...text].every((c) => c === text[0]);
    const needle = cycling ? text[0]! : text;
    const start = open ? active : Math.max(0, selectedIndex);
    const from = cycling || text.length === 1 ? start + 1 : start;
    for (let n = 0; n < options.length; n++) {
      const i = (from + n) % options.length;
      if (options[i]!.label.toLocaleLowerCase().startsWith(needle)) return i;
    }
    return undefined;
  };

  const onKeyDown = (e: ReactKeyboardEvent<HTMLButtonElement>) => {
    if (disabled) return;
    const last = options.length - 1;
    const printable = e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey;
    const typing = typed.current.text !== "" && Date.now() - typed.current.at <= TYPEAHEAD_MS;
    let handled = true;
    if (!open) {
      switch (e.key) {
        case "Enter":
        case " ":
        case "ArrowDown":
        case "ArrowUp":
          show();
          break;
        case "Home":
          show(0);
          break;
        case "End":
          show(last);
          break;
        default:
          if (printable) {
            const i = search(e.key);
            show(i);
          } else handled = false;
      }
    } else {
      switch (e.key) {
        case "ArrowDown":
          if (e.altKey) break;
          setActive((i) => Math.min(last, i + 1));
          break;
        case "ArrowUp":
          if (e.altKey) pick(active);
          else setActive((i) => Math.max(0, i - 1));
          break;
        case "Home":
          setActive(0);
          break;
        case "End":
          setActive(last);
          break;
        case "PageDown":
          setActive((i) => Math.min(last, i + 10));
          break;
        case "PageUp":
          setActive((i) => Math.max(0, i - 10));
          break;
        case "Enter":
          pick(active);
          break;
        case " ":
          if (typing) {
            const i = search(" ");
            if (i !== undefined) setActive(i);
          } else pick(active);
          break;
        case "Escape":
          hide();
          break;
        case "Tab":
          // Tab moves on as usual; the list just closes.
          hide(false);
          return;
        default:
          if (printable) {
            const i = search(e.key);
            if (i !== undefined) setActive(i);
          } else handled = false;
      }
    }
    if (handled) {
      // Nothing else reacts to these keys: not the game (arrows move a
      // player), not the popover or dialog around (Escape closes them).
      e.preventDefault();
      e.stopPropagation();
    }
  };

  const list = open
    ? createPortal(
        <ul
          ref={listRef}
          id={listId}
          role="listbox"
          aria-labelledby={labelId}
          aria-label={labelId ? undefined : ariaLabel}
          tabIndex={-1}
          className={`select-list${onStage ? " stage-tokens" : ""}`}
          style={pos}
          onPointerDown={keepInside}
          // The focus stays on the button while the list is used.
          onMouseDown={(e) => {
            e.preventDefault();
            e.stopPropagation();
          }}
          onClick={keepInside}
          onTouchStart={keepInside}
        >
          {options.map((o, i) => (
            <li
              key={o.value}
              id={optionId(i)}
              role="option"
              aria-selected={o.value === value}
              className={`select-option${i === active ? " is-active" : ""}`}
              onMouseMove={() => i !== active && setActive(i)}
              onClick={() => pick(i)}
            >
              <svg className="select-check" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <path d="M5 12.5l4.5 4.5L19 7.5" />
              </svg>
              <span className="select-option-text">
                <span className="select-option-label">{o.label}</span>
                {o.detail && <span className="select-option-detail">{o.detail}</span>}
              </span>
            </li>
          ))}
        </ul>,
        portalTarget(),
      )
    : null;

  return (
    <>
      <button
        ref={buttonRef}
        id={buttonId}
        type="button"
        role="combobox"
        className={`select${open ? " is-open" : ""}${className ? ` ${className}` : ""}`}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={open ? listId : undefined}
        aria-labelledby={labelId}
        aria-label={labelId ? undefined : ariaLabel}
        aria-activedescendant={open ? optionId(active) : undefined}
        disabled={disabled}
        onClick={() => (open ? hide() : show())}
        onKeyDown={onKeyDown}
        onBlur={(e) => {
          // Leaving for anything but the list closes it.
          if (open && !(e.relatedTarget instanceof Node && listRef.current?.contains(e.relatedTarget))) hide(false);
        }}
      >
        <span className="select-value">
          {(selected ?? options[0])?.label ?? ""}
        </span>
        <svg className="select-chevron" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
          <path d="m6 9 6 6 6-6" />
        </svg>
      </button>
      {list}
    </>
  );
}
