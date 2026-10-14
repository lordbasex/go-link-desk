// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { describe, expect, it } from "vitest";
import { clock, contentRect } from "./LiveSession";

describe("contentRect", () => {
  it("letterboxes a fitted picture", () => {
    // A 16:10 desktop in a 16:9 box: bars left and right.
    expect(contentRect(1600, 900, 1440, 900, "fit")).toEqual({ x: 80, y: 0, w: 1440, h: 900 });
    expect(contentRect(1000, 1000, 2000, 1000, "fit")).toEqual({ x: 0, y: 250, w: 1000, h: 500 });
  });
  it("fills the box when stretched", () => {
    expect(contentRect(1600, 900, 1440, 900, "stretch")).toEqual({ x: 0, y: 0, w: 1600, h: 900 });
  });
});

describe("clock", () => {
  it("shows a recording's length", () => {
    expect(clock(0)).toBe("0:00");
    expect(clock(245_400)).toBe("4:05");
    expect(clock(3_729_000)).toBe("1:02:09");
  });
});
