// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import { incomplete, decimal, priceTextOf, pricesToSend } from "./ModelPrices";

const stored = { alpha: { input: 3, output: 15, cacheHit: 0.3, currency: "USD" } };

describe("what a price row sends", () => {
  it("sends a complete row, with the input price as the cached price when that is blank", () => {
    expect(pricesToSend(["a"], { a: { inp: "1.5", out: "6", hit: "", cur: "CNY", src: "", chk: "" } }, {})).toEqual({ a: { input: 1.5, output: 6, cacheHit: 1.5, currency: "CNY" } });
    expect(pricesToSend(["a"], { a: { inp: "1.5", out: "6", hit: "0.15", cur: "USD", src: "", chk: "" } }, {})).toEqual({ a: { input: 1.5, output: 6, cacheHit: 0.15, currency: "USD" } });
  });

  it("keeps zero as a price: free is not unknown", () => {
    expect(pricesToSend(["a"], { a: { inp: "0", out: "0", hit: "", cur: "CNY", src: "", chk: "" } }, {})).toEqual({ a: { input: 0, output: 0, cacheHit: 0, currency: "CNY" } });
  });

  it("clears a stored price when the row is emptied, and sends nothing for a model that never had one", () => {
    expect(pricesToSend(["alpha"], { alpha: { inp: "", out: "", hit: "", cur: "USD", src: "", chk: "" } }, stored)).toEqual({ alpha: null });
    expect(pricesToSend(["beta"], { beta: { inp: "", out: "", hit: "", cur: "CNY", src: "", chk: "" } }, stored)).toEqual({});
  });

  it("changes nothing for a half-finished row", () => {
    const half = { alpha: { inp: "4", out: "", hit: "", cur: "USD", src: "", chk: "" } };
    expect(incomplete(half.alpha)).toBe(true);
    expect(pricesToSend(["alpha"], half, stored)).toEqual({ alpha: stored.alpha });
    expect(pricesToSend(["beta"], { beta: { inp: "4", out: "", hit: "", cur: "USD", src: "", chk: "" } }, stored)).toEqual({});
  });

  it("reads a stored rate back into the same row, leaving the cached field blank when it equals the input price", () => {
    expect(priceTextOf({ input: 2, output: 8, cacheHit: 2, currency: "CNY" })).toEqual({ inp: "2", out: "8", hit: "", cur: "CNY", src: "", chk: "" });
    expect(priceTextOf({ input: 2, output: 8, cacheHit: 0.2, currency: "USD" })).toEqual({ inp: "2", out: "8", hit: "0.2", cur: "USD", src: "", chk: "" });
  });

  it("types only digits and one point", () => {
    expect(decimal("-1.2.3e5")).toBe("1.235");
    expect(decimal("abc")).toBe("");
  });
});
