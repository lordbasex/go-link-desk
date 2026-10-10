// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect, useState } from "react";
import { Select, type SelectOption } from "./Select";

const FRUITS: SelectOption[] = [
  { value: "apple", label: "Apple" },
  { value: "apricot", label: "Apricot", detail: "Stone fruit" },
  { value: "banana", label: "Banana" },
  { value: "blueberry", label: "Blueberry" },
  { value: "cherry", label: "Cherry" },
];

function Harness({ onChange = () => {}, disabled = false }: { onChange?: (v: string) => void; disabled?: boolean }) {
  const [value, setValue] = useState("banana");
  return (
    <>
      <label id="fruit-label" htmlFor="fruit">
        Fruit
      </label>
      <Select
        id="fruit"
        labelId="fruit-label"
        value={value}
        options={FRUITS}
        disabled={disabled}
        onChange={(v) => {
          setValue(v);
          onChange(v);
        }}
      />
      <button type="button">After</button>
    </>
  );
}

/** A popover that, like the room's voice settings, closes on Escape and on a press outside it. */
function InPopover({ onClose }: { onClose: () => void }) {
  useEffect(() => {
    const esc = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    const away = (e: MouseEvent) => !document.getElementById("pop")?.contains(e.target as Node) && onClose();
    window.addEventListener("keydown", esc);
    document.addEventListener("mousedown", away);
    return () => {
      window.removeEventListener("keydown", esc);
      document.removeEventListener("mousedown", away);
    };
  }, [onClose]);
  return (
    <div id="pop" role="dialog" aria-label="Settings">
      <Harness />
    </div>
  );
}

const combo = () => screen.getByRole("combobox", { name: "Fruit" });
const activeOption = () => document.getElementById(combo().getAttribute("aria-activedescendant") ?? "");

afterEach(cleanup);

describe("Select", () => {
  it("is a named combobox that shows the value and opens a listbox", async () => {
    render(<Harness />);
    const button = combo();
    expect(button).toHaveAttribute("aria-haspopup", "listbox");
    expect(button).toHaveAttribute("aria-expanded", "false");
    expect(button).toHaveTextContent("Banana");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();

    await userEvent.click(button);
    expect(button).toHaveAttribute("aria-expanded", "true");
    const list = screen.getByRole("listbox", { name: "Fruit" });
    expect(button).toHaveAttribute("aria-controls", list.id);
    expect(screen.getAllByRole("option")).toHaveLength(5);
    expect(screen.getByRole("option", { name: "Banana" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("option", { name: "Apple" })).toHaveAttribute("aria-selected", "false");
    expect(screen.getByText("Stone fruit")).toBeInTheDocument();
    // The highlighted option starts at the current value.
    expect(activeOption()).toHaveTextContent("Banana");
  });

  it("moves with the arrows, Home and End and picks with Enter", async () => {
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);
    combo().focus();
    await userEvent.keyboard("{ArrowDown}");
    expect(combo()).toHaveAttribute("aria-expanded", "true");
    await userEvent.keyboard("{ArrowDown}");
    expect(activeOption()).toHaveTextContent("Blueberry");
    await userEvent.keyboard("{End}");
    expect(activeOption()).toHaveTextContent("Cherry");
    await userEvent.keyboard("{ArrowDown}");
    expect(activeOption()).toHaveTextContent("Cherry");
    await userEvent.keyboard("{Home}");
    expect(activeOption()).toHaveTextContent("Apple");
    await userEvent.keyboard("{ArrowUp}{ArrowDown}");
    expect(activeOption()).toHaveTextContent("Apricot");
    await userEvent.keyboard("{Enter}");
    expect(onChange).toHaveBeenCalledWith("apricot");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    expect(combo()).toHaveFocus();
    expect(combo()).toHaveTextContent("Apricot");
  });

  it("opens with Enter, Space and ArrowUp and picks with Space", async () => {
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);
    combo().focus();
    await userEvent.keyboard("{Enter}");
    expect(screen.getByRole("listbox")).toBeInTheDocument();
    await userEvent.keyboard("{Escape}");
    await userEvent.keyboard("{ArrowUp}");
    expect(screen.getByRole("listbox")).toBeInTheDocument();
    await userEvent.keyboard("{Escape}");
    await userEvent.keyboard(" ");
    expect(screen.getByRole("listbox")).toBeInTheDocument();
    await userEvent.keyboard("{ArrowUp} ");
    expect(onChange).toHaveBeenCalledWith("apricot");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("jumps to what is typed", async () => {
    render(<Harness />);
    combo().focus();
    await userEvent.keyboard("{Enter}");
    await userEvent.keyboard("ch");
    expect(activeOption()).toHaveTextContent("Cherry");
    // A pause starts a new search; "apr" goes past Apple to Apricot.
    await new Promise((r) => setTimeout(r, 700));
    await userEvent.keyboard("apr");
    expect(activeOption()).toHaveTextContent("Apricot");
    // Repeating one letter cycles through the options with that letter.
    await new Promise((r) => setTimeout(r, 700));
    await userEvent.keyboard("b");
    expect(activeOption()).toHaveTextContent("Banana");
    await userEvent.keyboard("b");
    expect(activeOption()).toHaveTextContent("Blueberry");
    await userEvent.keyboard("b");
    expect(activeOption()).toHaveTextContent("Banana");
  });

  it("types ahead while closed too, opening on the match", async () => {
    render(<Harness />);
    combo().focus();
    await userEvent.keyboard("c");
    expect(screen.getByRole("listbox")).toBeInTheDocument();
    expect(activeOption()).toHaveTextContent("Cherry");
  });

  it("picks with a click and calls back only on a change", async () => {
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);
    await userEvent.click(combo());
    await userEvent.click(screen.getByRole("option", { name: "Banana" }));
    expect(onChange).not.toHaveBeenCalled();
    await userEvent.click(combo());
    await userEvent.click(screen.getByRole("option", { name: /Apricot/ }));
    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenCalledWith("apricot");
    expect(combo()).toHaveFocus();
  });

  it("closes on Escape without closing the popover around it", async () => {
    const onClose = vi.fn();
    render(<InPopover onClose={onClose} />);
    combo().focus();
    await userEvent.keyboard("{Enter}");
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
    expect(combo()).toHaveFocus();
    // Closed, Escape belongs to the popover again.
    await userEvent.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("picking an option does not count as a press outside the popover", async () => {
    const onClose = vi.fn();
    render(<InPopover onClose={onClose} />);
    await userEvent.click(combo());
    await userEvent.click(screen.getByRole("option", { name: "Cherry" }));
    expect(onClose).not.toHaveBeenCalled();
    expect(combo()).toHaveTextContent("Cherry");
  });

  it("closes on a press outside and on Tab", async () => {
    render(<Harness />);
    await userEvent.click(combo());
    fireEvent.mouseDown(document.body);
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    combo().focus();
    await userEvent.keyboard("{Enter}");
    await userEvent.tab();
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "After" })).toHaveFocus();
  });

  it("does nothing while disabled", async () => {
    const onChange = vi.fn();
    render(<Harness onChange={onChange} disabled />);
    expect(combo()).toBeDisabled();
    await userEvent.click(combo());
    fireEvent.keyDown(combo(), { key: "ArrowDown" });
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    expect(onChange).not.toHaveBeenCalled();
  });

  it("keeps the keys it uses from the rest of the page", async () => {
    const seen = vi.fn();
    window.addEventListener("keydown", seen);
    render(<Harness />);
    combo().focus();
    await userEvent.keyboard("{ArrowDown}{ArrowDown}{Enter}");
    window.removeEventListener("keydown", seen);
    expect(seen).not.toHaveBeenCalled();
  });
});
