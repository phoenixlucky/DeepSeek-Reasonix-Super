import { describe, expect, it } from "vitest";
import { clampedShare } from "./meterShare";

describe("the share a meter fills", () => {
  it("is the rounded percentage of the maximum", () => {
    expect(clampedShare(1, 4)).toBe(25);
    expect(clampedShare(49, 64)).toBe(77);
  });

  it("never leaves 0 to 100, whatever the value", () => {
    expect(clampedShare(200, 100)).toBe(100);
    expect(clampedShare(-5, 100)).toBe(0);
  });

  it("is empty when there is no maximum to measure against or the value is not a number", () => {
    expect(clampedShare(5, 0)).toBe(0);
    expect(clampedShare(5, -1)).toBe(0);
    expect(clampedShare(Number.NaN, 10)).toBe(0);
    expect(clampedShare(Number.POSITIVE_INFINITY, 10)).toBe(0);
  });
});
