import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { initialState as initial, reduce } from "./session";
import type { Item, SessionEvent } from "./session";

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

const ask = (skipInMs?: number): SessionEvent => ({ kind: "ask_request", ask: { id: "a1", questions: [], skipInMs } });
const card = (s: typeof initial) => s.items.find((i: Item): i is Extract<Item, { t: "ask" }> => i.t === "ask")!;

it("stamps the deadline from the wait the kernel announced", () => {
  vi.setSystemTime(1_000_000);
  expect(card(reduce(initial, ask(30_000))).skipAt).toBe(1_030_000);
});

it("leaves a question that is never skipped without a deadline", () => {
  expect(card(reduce(initial, ask())).skipAt).toBeUndefined();
});

it("moves the same card's deadline when the question is announced again, and clears it when taken over", () => {
  vi.setSystemTime(1_000_000);
  let s = reduce(initial, ask(30_000));
  vi.setSystemTime(1_010_000);
  s = reduce(s, ask(20_000));
  expect(s.items.filter((i) => i.t === "ask")).toHaveLength(1);
  expect(card(s).skipAt).toBe(1_030_000);
  s = reduce(s, { kind: "ask_engaged", ask: { id: "a1", questions: [] } });
  expect(s.items.filter((i) => i.t === "ask")).toHaveLength(1);
  expect(card(s).skipAt).toBeUndefined();
});
