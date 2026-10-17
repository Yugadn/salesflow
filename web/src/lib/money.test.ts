import { describe, expect, it } from "vitest";

import { centsToDecimal, formatIDR, isValidPrice, lineTotal, toCents } from "./money";

describe("toCents", () => {
  it.each([
    ["96000", 9600000n],
    ["1333.33", 133333n],
    ["4200.5", 420050n],
    ["0", 0n],
    ["-12.30", -1230n],
  ])("%s -> %s", (input, want) => {
    expect(toCents(input)).toBe(want);
  });

  it("rejects more than two decimals and garbage", () => {
    expect(() => toCents("1.234")).toThrow();
    expect(() => toCents("abc")).toThrow();
  });
});

describe("arithmetic is exact", () => {
  it("1333.33 x 3 is 3999.99, not 3999.9900000000002", () => {
    expect(centsToDecimal(lineTotal("1333.33", 3))).toBe("3999.99");
  });

  it("sums order lines like the backend: 2 x 96000 + 6 x 4200 = 217200", () => {
    expect(centsToDecimal(lineTotal("96000", 2) + lineTotal("4200", 6))).toBe("217200");
  });

  it("0.1 + 0.2 is 0.3", () => {
    expect(centsToDecimal(toCents("0.1") + toCents("0.2"))).toBe("0.3");
  });
});

describe("formatIDR", () => {
  it("uses Indonesian separators", () => {
    // Intl inserts a non-breaking space after "Rp".
    expect(formatIDR("96000").replace(/\s/g, " ")).toBe("Rp 96.000");
    expect(formatIDR("1333.33").replace(/\s/g, " ")).toBe("Rp 1.333,33");
  });
});

describe("isValidPrice", () => {
  it.each([
    ["96000", true],
    ["4200.50", true],
    ["-1", false],
    ["1.234", false],
    ["", false],
  ])("%s -> %s", (input, want) => {
    expect(isValidPrice(input)).toBe(want);
  });
});
