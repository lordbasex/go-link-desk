// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { describe, expect, it } from "vitest";
import { median } from "./latency";

describe("median", () => {
  it("takes the middle", () => {
    expect(median([30, 10, 20])).toBe(20);
    expect(median([40, 10, 20, 30])).toBe(25);
    expect(median([])).toBeNull();
  });
});
