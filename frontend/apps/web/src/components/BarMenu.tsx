// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { CheckIcon } from "./Icons";

/**
 * A button of the session's floating toolbar that opens a menu under it,
 * as the iPhone app's menus: sections, choices with a check, and actions.
 * Esc or a click elsewhere closes it; onOpen tells the toolbar to stay.
 */
export function BarMenu({ label, icon, onOpen, children }: { label: string; icon: ReactNode; onOpen?: (open: boolean) => void; children: (close: () => void) => ReactNode }) {
  const [open, setOpen] = useState(false);
  const id = useId();
  const box = useRef<HTMLDivElement>(null);
  const set = (v: boolean) => {
    setOpen(v);
    onOpen?.(v);
  };
  useEffect(() => {
    if (!open) return;
    const down = (e: PointerEvent) => {
      if (!box.current?.contains(e.target as Node)) set(false);
    };
    const key = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        set(false);
      }
    };
    window.addEventListener("pointerdown", down, true);
    window.addEventListener("keydown", key, true);
    return () => {
      window.removeEventListener("pointerdown", down, true);
      window.removeEventListener("keydown", key, true);
    };
    // set is stable enough: it only closes this menu.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);
  return (
    <div className="bar-menu-box" ref={box}>
      <button type="button" className={`bar-button${open ? " is-on" : ""}`} aria-label={label} title={label} aria-haspopup="menu" aria-expanded={open} aria-controls={open ? id : undefined} onClick={() => set(!open)}>
        {icon}
      </button>
      {open && (
        <div className="bar-menu" role="menu" id={id} aria-label={label}>
          {children(() => set(false))}
        </div>
      )}
    </div>
  );
}

export function MenuSection({ title }: { title: string }) {
  return (
    <div className="bar-menu-section" role="presentation">
      {title}
    </div>
  );
}

export function MenuSeparator() {
  return <div className="bar-menu-separator" role="separator" />;
}

/** One entry: an action, or a choice (checked shows a check). */
export function MenuItem({ children, onSelect, checked, kind = "action", icon }: { children: ReactNode; onSelect: () => void; checked?: boolean; kind?: "action" | "radio" | "check"; icon?: ReactNode }) {
  const role = kind === "radio" ? "menuitemradio" : kind === "check" ? "menuitemcheckbox" : "menuitem";
  return (
    <button type="button" className="bar-menu-item" role={role} aria-checked={kind === "action" ? undefined : !!checked} onClick={onSelect}>
      <span className="bar-menu-mark">{kind === "action" ? icon : checked ? <CheckIcon /> : null}</span>
      <span>{children}</span>
    </button>
  );
}
