import { describe, expect, it } from "vitest";
import type { CostQuote } from "../port/wire";
import { cacheRate, costSay } from "./overview";

const quote = (over: Partial<CostQuote>): CostQuote => ({ original: { amount: "12.5", currency: "CNY" }, estimated: false, costComplete: false, ...over } as CostQuote);

describe("the cache rate", () => {
  it("is the share of split prompt that was cached, not an average of rates", () => {
    expect(cacheRate(900, 100)).toBeCloseTo(0.9);
    expect(cacheRate(2300, 700)).toBeCloseTo(2300 / 3000);
  });

  it("leaves prompt the endpoint did not split out of the denominator", () => {
    expect(cacheRate(2300, 700 + 5000, 5000)).toBeCloseTo(2300 / 3000);
  });

  it("says nothing when no request was split", () => {
    expect(cacheRate(0, 5000, 5000)).toBeNull();
    expect(cacheRate(0, 0)).toBeNull();
  });
});

describe("saying a cost as its coverage allows", () => {
  it("gives an exact figure only for a complete cost", () => {
    expect(costSay(quote({ coverage: "complete" }))).toEqual({ kind: "exact", amount: 12.5, currency: "CNY" });
  });

  it("gives a floor, not a figure, for a partly priced one", () => {
    expect(costSay(quote({ coverage: "partial" }))).toEqual({ kind: "atLeast", amount: 12.5, currency: "CNY" });
  });

  it("gives no amount for an unpriced one, and keeps the reason", () => {
    expect(costSay(quote({ coverage: "incomplete", incompleteReason: "no_rate" }))).toEqual({ kind: "unknown", reason: "no_rate" });
  });

  it("says nothing was billed for none, or for no quote at all", () => {
    expect(costSay(quote({ coverage: "none" }))).toEqual({ kind: "none" });
    expect(costSay(undefined)).toEqual({ kind: "none" });
  });
});
